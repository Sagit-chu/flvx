#!/usr/bin/env python3

from __future__ import annotations

import argparse
import json
import shlex
import subprocess
import sys
import textwrap
from dataclasses import dataclass
from pathlib import Path


ROOT_DIR = Path(__file__).resolve().parents[2]
MANIFEST_PATH = ROOT_DIR / "release" / "dash-build-manifest.json"
DEFAULT_BACKEND_COMMAND = "go run ./cmd/paneld"
DEFAULT_INSTALL_CHANNEL = "stable"


REMOTE_WORKER = textwrap.dedent(
    r'''
    #!/usr/bin/env python3

    from __future__ import annotations

    import argparse
    import json
    import os
    import random
    import re
    import shutil
    import signal
    import socket
    import subprocess
    import sys
    import threading
    import textwrap
    import time
    from contextlib import closing
    from pathlib import Path
    from urllib.error import HTTPError, URLError
    from urllib.request import Request, urlopen


    API_TIMEOUT_SEC = 15


    def random_suffix() -> str:
        return f"{int(time.time())}-{random.randint(1000, 9999)}"


    def find_free_port(host: str = "127.0.0.1") -> int:
        with closing(socket.socket(socket.AF_INET, socket.SOCK_STREAM)) as sock:
            sock.bind((host, 0))
            return int(sock.getsockname()[1])


    def wait_for_port(host: str, port: int, timeout: float) -> None:
        deadline = time.time() + timeout
        while time.time() < deadline:
            try:
                with socket.create_connection((host, port), timeout=0.5):
                    return
            except OSError:
                time.sleep(0.2)
        raise RuntimeError(f"timed out waiting for {host}:{port}")


    def wait_for(predicate, timeout: float, interval: float, message: str):
        deadline = time.time() + timeout
        last_error = None
        while time.time() < deadline:
            try:
                result = predicate()
                if result:
                    return result
            except Exception as exc:
                last_error = exc
            time.sleep(interval)
        if last_error is not None:
            raise RuntimeError(f"{message}: {last_error}") from last_error
        raise RuntimeError(message)


    def run_shell(command: str, *, cwd: Path | None = None, env: dict[str, str] | None = None) -> None:
        completed = subprocess.run(
            command,
            shell=True,
            executable="/bin/bash",
            cwd=str(cwd) if cwd else None,
            env=env,
            capture_output=True,
            text=True,
        )
        if completed.returncode != 0:
            raise RuntimeError(
                f"command failed ({completed.returncode}): {command}\nstdout:\n{completed.stdout}\nstderr:\n{completed.stderr}"
            )


    def start_logged_process(command: str, *, cwd: Path, env: dict[str, str], log_path: Path):
        log_path.parent.mkdir(parents=True, exist_ok=True)
        log_handle = log_path.open("w", encoding="utf-8")
        process = subprocess.Popen(
            command,
            shell=True,
            executable="/bin/bash",
            cwd=str(cwd),
            env=env,
            stdout=log_handle,
            stderr=subprocess.STDOUT,
            preexec_fn=os.setsid,
            text=True,
        )
        return process, log_handle


    def terminate_process(process: subprocess.Popen[str] | None) -> None:
        if process is None or process.poll() is not None:
            return
        try:
            os.killpg(os.getpgid(process.pid), signal.SIGTERM)
            process.wait(timeout=5)
        except Exception:
            try:
                os.killpg(os.getpgid(process.pid), signal.SIGKILL)
            except Exception:
                pass


    class JSONClient:
        def __init__(self, base_url: str):
            self.base_url = base_url.rstrip("/")
            self.token = ""

        def request(self, method: str, path: str, payload: dict | None = None, *, auth: bool = True) -> dict:
            body = json.dumps(payload or {}).encode()
            req = Request(self.base_url + path, data=body, method=method)
            req.add_header("Content-Type", "application/json")
            if auth and self.token:
                req.add_header("Authorization", self.token)
            try:
                with urlopen(req, timeout=API_TIMEOUT_SEC) as resp:
                    return json.loads(resp.read().decode())
            except HTTPError as exc:
                detail = exc.read().decode(errors="replace")
                raise RuntimeError(f"{method} {path} failed: {exc.code} {detail}") from exc
            except URLError as exc:
                raise RuntimeError(f"{method} {path} failed: {exc}") from exc

        def login(self, username: str, password: str) -> None:
            payload = self.request(
                "POST",
                "/api/v1/user/login",
                {"username": username, "password": password, "captchaId": ""},
                auth=False,
            )
            token = (payload.get("data") or {}).get("token", "")
            if payload.get("code") != 0 or not token:
                raise RuntimeError(f"login failed: {payload}")
            self.token = token


    def api_ok(payload: dict, context: str) -> dict:
        if payload.get("code") != 0:
            raise RuntimeError(f"{context} failed: {payload}")
        return payload.get("data") or {}


    def list_nodes(client: JSONClient) -> list[dict]:
        data = api_ok(client.request("POST", "/api/v1/node/list", {}), "list nodes")
        if not isinstance(data, list):
            raise RuntimeError(f"unexpected node list payload: {data}")
        return data


    def create_node(client: JSONClient, *, name: str, server_ip: str, listen_ip: str, port_range: str) -> dict:
        api_ok(
            client.request(
                "POST",
                "/api/v1/node/create",
                {
                    "name": name,
                    "serverIp": server_ip,
                    "serverIpV4": server_ip,
                    "port": port_range,
                    "interfaceName": "",
                    "http": 1,
                    "tls": 1,
                    "socks": 1,
                    "tcpListenAddr": listen_ip,
                    "udpListenAddr": listen_ip,
                    "isRemote": 0,
                    "remoteUrl": "",
                    "remoteToken": "",
                },
            ),
            f"create node {name}",
        )

        def poll() -> dict:
            for item in list_nodes(client):
                if item.get("name") == name:
                    return item
            return {}

        return wait_for(poll, 10.0, 0.3, f"node {name} not visible in node/list")


    def update_panel_ip(client: JSONClient, backend_addr: str) -> None:
        api_ok(
            client.request(
                "POST",
                "/api/v1/config/update-single",
                {"name": "ip", "value": backend_addr},
            ),
            "update panel ip config",
        )


    def extract_secret_from_install_command(command: str) -> str:
        parts = command.split()
        for index, part in enumerate(parts):
            if part == "-s" and index + 1 < len(parts):
                return parts[index + 1]
        match = re.search(r"(?:^|\s)-s\s+([^\s]+)", command)
        if match:
            return match.group(1)
        raise RuntimeError(f"could not extract secret from install command: {command}")


    def install_agent_bundle(
        client: JSONClient,
        *,
        node_id: int,
        root: Path,
        channel: str,
        bundle_url: str,
    ) -> str:
        command = api_ok(
            client.request(
                "POST",
                "/api/v1/node/install",
                {"id": node_id, "channel": channel},
            ),
            f"request install command for node {node_id}",
        )
        if not isinstance(command, str) or not command.strip():
            raise RuntimeError(f"node/install returned unexpected payload: {command}")
        secret = extract_secret_from_install_command(command)
        root.mkdir(parents=True, exist_ok=True)
        bundle_archive = root / "agent-bundle.tar.gz"
        run_shell(f"curl -fsSL {shlex_quote(bundle_url)} -o {shlex_quote(str(bundle_archive))}")
        run_shell(f"tar -xf {shlex_quote(str(bundle_archive))} -C {shlex_quote(str(root))} --strip-components=1")
        run_shell(f"chmod +x {shlex_quote(str(root / 'bin' / 'gost'))} {shlex_quote(str(root / 'bin' / 'dash'))} {shlex_quote(str(root / 'bin' / 'flvx-agent-launcher'))}")
        (root / "config").mkdir(parents=True, exist_ok=True)
        (root / "runtime" / "gost").mkdir(parents=True, exist_ok=True)
        (root / "runtime" / "dash").mkdir(parents=True, exist_ok=True)
        (root / "data").mkdir(parents=True, exist_ok=True)
        (root / "logs").mkdir(parents=True, exist_ok=True)
        runtime_env_example = root / "config" / "runtime.env.example"
        runtime_env = root / "config" / "runtime.env"
        if runtime_env_example.exists() and not runtime_env.exists():
            runtime_env.write_text(runtime_env_example.read_text(encoding="utf-8"), encoding="utf-8")
        return secret


    def ensure_fake_system_tools(fake_bin: Path) -> None:
        fake_bin.mkdir(parents=True, exist_ok=True)
        for name in ("systemctl", "tcpkill"):
            script = fake_bin / name
            script.write_text("#!/usr/bin/env bash\nexit 0\n", encoding="utf-8")
            script.chmod(0o755)


    def replace_runtime_engine(runtime_env: Path, engine: str, dash_binary_path: str) -> None:
        lines = runtime_env.read_text(encoding="utf-8").splitlines()
        replaced = False
        dash_path_replaced = False
        config_path_replaced = False
        for index, line in enumerate(lines):
            if line.startswith("RUNTIME_ENGINE="):
                lines[index] = f"RUNTIME_ENGINE={engine}"
                replaced = True
            elif line.startswith("FLVX_AGENT_DASH_BIN="):
                lines[index] = f"FLVX_AGENT_DASH_BIN={dash_binary_path}"
                dash_path_replaced = True
            elif line.startswith("FLVX_AGENT_CONFIG="):
                lines[index] = f"FLVX_AGENT_CONFIG={runtime_env.parent / 'agent.yaml'}"
                config_path_replaced = True
        if not replaced:
            lines.append(f"RUNTIME_ENGINE={engine}")
        if not dash_path_replaced:
            lines.append(f"FLVX_AGENT_DASH_BIN={dash_binary_path}")
        if not config_path_replaced:
            lines.append(f"FLVX_AGENT_CONFIG={runtime_env.parent / 'agent.yaml'}")
        runtime_env.write_text("\n".join(lines) + "\n", encoding="utf-8")


    def write_agent_config(
        root: Path,
        *,
        backend_addr: str,
        secret: str,
        mode: str,
        tunnel_listen: str,
        api_listen: str,
        active_exit_server: str,
        active_exit_token: str,
        dash_binary_path: str,
    ) -> None:
        config_dir = root / "config"
        data_dir = root / "data"
        log_dir = root / "logs"
        config_dir.mkdir(parents=True, exist_ok=True)
        data_dir.mkdir(parents=True, exist_ok=True)
        log_dir.mkdir(parents=True, exist_ok=True)
        agent_yaml = textwrap.dedent(
            f"""
            panel:
              server: {backend_addr}
              secret: {secret}
            node:
              mode: {mode}
              runtime_default: dash
            network:
              tunnel_listen: {tunnel_listen}
              api_listen: {api_listen}
            relay:
              state_file: {data_dir}/relay-state.yaml
              active_exit:
                server: {active_exit_server}
                token: {active_exit_token}
            paths:
              data_dir: {data_dir}
              log_dir: {log_dir}
            """
        ).strip() + "\n"
        (config_dir / "agent.yaml").write_text(agent_yaml, encoding="utf-8")
        replace_runtime_engine(config_dir / "runtime.env", "dash", dash_binary_path)


    def start_agent(root: Path, log_path: Path):
        env = os.environ.copy()
        env["FLVX_AGENT_RUNTIME_ENV"] = str(root / "config" / "runtime.env")
        return start_logged_process(
            f"{shlex_quote(str(root / 'bin' / 'flvx-agent-launcher'))}",
            cwd=root,
            env=env,
            log_path=log_path,
        )


    def switch_runtime_to_dash(client: JSONClient, timeout: float) -> dict:
        api_ok(client.request("PUT", "/api/v1/system/runtime", {"engine": "dash"}), "switch runtime to dash")

        def poll() -> dict:
            data = api_ok(client.request("GET", "/api/v1/system/runtime"), "get runtime settings")
            if data.get("currentEngine") == "dash" and data.get("switchStatus") in {"idle", "completed"}:
                return data
            return {}

        return wait_for(poll, timeout, 0.5, "dash runtime switch did not complete")


    def wait_for_nodes_ready(client: JSONClient, *, node_names: list[str], timeout: float) -> list[dict]:
        wanted = set(node_names)

        def poll() -> list[dict]:
            rows = api_ok(client.request("POST", "/api/v1/node/check-status", {}), "refresh node statuses")
            selected = [row for row in rows if row.get("name") in wanted]
            if len(selected) != len(wanted):
                return []
            if all(row.get("runtimeReady") is True and int(row.get("status") or 0) == 1 for row in selected):
                return selected
            return []

        return wait_for(poll, timeout, 1.0, "nodes did not reach runtimeReady=true and status=1")


    def create_tunnel(client: JSONClient, *, name: str, entry_node_id: int) -> dict:
        api_ok(
            client.request(
                "POST",
                "/api/v1/tunnel/create",
                {
                    "name": name,
                    "type": 1,
                    "status": 1,
                    "flow": 1,
                    "trafficRatio": 1,
                    "inNodeId": [{"nodeId": entry_node_id, "protocol": "tls", "strategy": "round"}],
                },
            ),
            f"create tunnel {name}",
        )

        def poll() -> dict:
            rows = api_ok(client.request("POST", "/api/v1/tunnel/list", {}), "list tunnels")
            for row in rows:
                if row.get("name") == name:
                    return row
            return {}

        return wait_for(poll, 10.0, 0.3, f"tunnel {name} not visible in tunnel/list")


    def create_forward(client: JSONClient, *, name: str, tunnel_id: int, remote_addr: str) -> dict:
        runtime_data = api_ok(
            client.request(
                "POST",
                "/api/v1/forward/create",
                {
                    "name": name,
                    "tunnelId": tunnel_id,
                    "remoteAddr": remote_addr,
                    "inPort": 0,
                    "strategy": "round",
                },
            ),
            f"create forward {name}",
        )
        runtime = runtime_data.get("runtime") or {}
        listen_port = 0
        for child in runtime.get("children") or []:
            if int(child.get("port") or 0) > 0:
                listen_port = int(child["port"])
                break
        if listen_port <= 0:
            raise RuntimeError(f"forward runtime did not report a listen port: {runtime}")

        def poll() -> dict:
            rows = api_ok(client.request("POST", "/api/v1/forward/list", {}), "list forwards")
            for row in rows:
                if row.get("name") == name:
                    row["runtime"] = runtime
                    row["inPort"] = listen_port
                    return row
            return {}

        return wait_for(poll, 10.0, 0.3, f"forward {name} not visible in forward/list")


    class TCPEchoServer(threading.Thread):
        def __init__(self, host: str, port: int):
            super().__init__(daemon=True)
            self.stop_event = threading.Event()
            self.server = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
            self.server.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
            self.server.bind((host, port))
            self.server.listen()
            self.server.settimeout(0.5)

        def run(self) -> None:
            while not self.stop_event.is_set():
                try:
                    conn, _ = self.server.accept()
                except socket.timeout:
                    continue
                threading.Thread(target=self._handle, args=(conn,), daemon=True).start()

        def _handle(self, conn: socket.socket) -> None:
            with conn:
                data = conn.recv(65535)
                if data:
                    conn.sendall(data)

        def close(self) -> None:
            self.stop_event.set()
            self.server.close()


    class UDPEchoServer(threading.Thread):
        def __init__(self, host: str, port: int):
            super().__init__(daemon=True)
            self.stop_event = threading.Event()
            self.server = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
            self.server.bind((host, port))
            self.server.settimeout(0.5)

        def run(self) -> None:
            while not self.stop_event.is_set():
                try:
                    payload, addr = self.server.recvfrom(65535)
                except socket.timeout:
                    continue
                self.server.sendto(payload, addr)

        def close(self) -> None:
            self.stop_event.set()
            self.server.close()


    def verify_tcp_flow(host: str, port: int) -> dict:
        payload = b"dash-cleanroom-tcp"
        with socket.create_connection((host, port), timeout=5) as conn:
            conn.sendall(payload)
            echoed = conn.recv(65535)
        return {"success": echoed == payload, "echo": echoed.decode(errors="replace")}


    def verify_udp_flow(host: str, port: int) -> dict:
        payload = b"dash-cleanroom-udp"
        with closing(socket.socket(socket.AF_INET, socket.SOCK_DGRAM)) as sock:
            sock.settimeout(5)
            sock.sendto(payload, (host, port))
            echoed, _ = sock.recvfrom(65535)
        return {"success": echoed == payload, "echo": echoed.decode(errors="replace")}


    def shlex_quote(value: str) -> str:
        import shlex

        return shlex.quote(value)


    def build_parser() -> argparse.ArgumentParser:
        parser = argparse.ArgumentParser(description="Remote worker for Dash dual-rule clean-room validation")
        parser.add_argument("--workspace", required=True)
        parser.add_argument("--backend-command", required=True)
        parser.add_argument("--backend-timeout", type=float, default=60.0)
        parser.add_argument("--backend-port", type=int, default=0)
        parser.add_argument("--install-channel", default="stable")
        parser.add_argument("--bundle-url", default="")
        parser.add_argument("--target-host", default="127.0.0.1")
        parser.add_argument("--target-port", type=int, default=0)
        parser.add_argument("--dash-binary-path", default="/tmp/dash-agent-build/target/release/dash")
        parser.add_argument("--keep-workspace", action="store_true")
        return parser


    def main() -> int:
        args = build_parser().parse_args()
        workspace = Path(args.workspace)
        backend_root = workspace / "go-backend"
        nodes_root = workspace / "nodes"
        logs_root = workspace / "logs"
        fake_bin = workspace / "fake-systemd"
        backend_db = workspace / "panel.db"

        backend_process = None
        backend_log = None
        entry_process = None
        entry_log = None
        exit_process = None
        exit_log = None
        tcp_server = None
        udp_server = None

        try:
            if not backend_root.exists():
                raise RuntimeError(f"missing uploaded go-backend tree at {backend_root}")

            workspace.mkdir(parents=True, exist_ok=True)
            nodes_root.mkdir(parents=True, exist_ok=True)
            logs_root.mkdir(parents=True, exist_ok=True)
            ensure_fake_system_tools(fake_bin)

            backend_port = args.backend_port or find_free_port()
            backend_addr = f"127.0.0.1:{backend_port}"
            target_host = args.target_host
            target_port = args.target_port or find_free_port(target_host)

            if args.target_port == 0 and target_host == "127.0.0.1":
                tcp_server = TCPEchoServer(target_host, target_port)
                udp_server = UDPEchoServer(target_host, target_port)
                tcp_server.start()
                udp_server.start()

            backend_env = os.environ.copy()
            backend_env.update(
                {
                    "SERVER_ADDR": f":{backend_port}",
                    "DB_PATH": str(backend_db),
                    "JWT_SECRET": f"dash-cleanroom-{random_suffix()}",
                    "DASH_RUNTIME_ENABLED": "1",
                    "DASH_NODE_API_SCHEME": "http",
                    "DASH_NODE_API_PORT": "19090",
                }
            )
            backend_process, backend_log = start_logged_process(
                args.backend_command,
                cwd=backend_root,
                env=backend_env,
                log_path=logs_root / "backend.log",
            )
            wait_for_port("127.0.0.1", backend_port, args.backend_timeout)

            client = JSONClient(f"http://127.0.0.1:{backend_port}")
            wait_for(lambda: client.login("admin_user", "admin_user") or True, args.backend_timeout, 0.5, "backend login did not become ready")
            update_panel_ip(client, backend_addr)

            suffix = random_suffix()
            entry_name = f"dash-cleanroom-entry-{suffix}"
            exit_name = f"dash-cleanroom-exit-{suffix}"
            tunnel_name = f"dash-cleanroom-tunnel-{suffix}"
            forward_name = f"dash-cleanroom-forward-{suffix}"

            entry_node = create_node(
                client,
                name=entry_name,
                server_ip="127.0.0.2",
                listen_ip="127.0.0.2",
                port_range="25000-25999",
            )
            exit_node = create_node(
                client,
                name=exit_name,
                server_ip="127.0.0.3",
                listen_ip="127.0.0.3",
                port_range="26000-26999",
            )

            entry_root = nodes_root / "entry"
            exit_root = nodes_root / "exit"
            entry_secret = install_agent_bundle(
                client,
                node_id=int(entry_node["id"]),
                root=entry_root,
                channel=args.install_channel,
                bundle_url=args.bundle_url,
            )
            exit_secret = install_agent_bundle(
                client,
                node_id=int(exit_node["id"]),
                root=exit_root,
                channel=args.install_channel,
                bundle_url=args.bundle_url,
            )

            write_agent_config(
                entry_root,
                backend_addr=backend_addr,
                secret=entry_secret,
                mode="entry",
                tunnel_listen="127.0.0.2:18080",
                api_listen="127.0.0.2:19090",
                active_exit_server="127.0.0.3:18080",
                active_exit_token=exit_secret,
                dash_binary_path=args.dash_binary_path,
            )
            write_agent_config(
                exit_root,
                backend_addr=backend_addr,
                secret=exit_secret,
                mode="exit",
                tunnel_listen="127.0.0.3:18080",
                api_listen="127.0.0.3:19090",
                active_exit_server="127.0.0.3:18080",
                active_exit_token=exit_secret,
                dash_binary_path=args.dash_binary_path,
            )

            exit_process, exit_log = start_agent(exit_root, logs_root / "exit.log")
            wait_for_port("127.0.0.3", 19090, args.backend_timeout)
            entry_process, entry_log = start_agent(entry_root, logs_root / "entry.log")
            wait_for_port("127.0.0.2", 19090, args.backend_timeout)

            runtime_switch = switch_runtime_to_dash(client, args.backend_timeout)
            node_rows = wait_for_nodes_ready(client, node_names=[entry_name, exit_name], timeout=args.backend_timeout)

            tunnel = create_tunnel(client, name=tunnel_name, entry_node_id=int(entry_node["id"]))
            forward = create_forward(
                client,
                name=forward_name,
                tunnel_id=int(tunnel["id"]),
                remote_addr=f"{target_host}:{target_port}",
            )

            tcp_result = verify_tcp_flow("127.0.0.2", int(forward["inPort"]))
            udp_result = verify_udp_flow("127.0.0.2", int(forward["inPort"]))

            summary = {
                "runtimeEngine": runtime_switch.get("currentEngine"),
                "remote": {
                    "workspace": str(workspace),
                    "backendAddr": backend_addr,
                    "targetHost": target_host,
                    "targetPort": target_port,
                },
                "nodes": [
                    {
                        "id": int(row["id"]),
                        "name": row["name"],
                        "serverIp": row["serverIp"],
                        "status": int(row.get("status") or 0),
                        "runtimeReady": bool(row.get("runtimeReady")),
                    }
                    for row in node_rows
                ],
                "tunnel": {"id": int(tunnel["id"]), "name": tunnel["name"], "type": 1},
                "forward": {
                    "id": int(forward["id"]),
                    "name": forward["name"],
                    "listenHost": "127.0.0.2",
                    "inPort": int(forward["inPort"]),
                    "runtime": forward["runtime"],
                },
                "tcp": tcp_result,
                "udp": udp_result,
                "artifacts": {
                    "entryRoot": str(entry_root),
                    "exitRoot": str(exit_root),
                    "logs": {
                        "backend": str(logs_root / "backend.log"),
                        "entry": str(logs_root / "entry.log"),
                        "exit": str(logs_root / "exit.log"),
                    },
                },
            }
            json.dump(summary, sys.stdout)
            sys.stdout.write("\n")
            return 0
        except Exception as exc:
            print(f"remote cleanroom validation failed: {exc}", file=sys.stderr)
            return 1
        finally:
            if tcp_server is not None:
                tcp_server.close()
            if udp_server is not None:
                udp_server.close()
            terminate_process(entry_process)
            terminate_process(exit_process)
            terminate_process(backend_process)
            if entry_log is not None:
                entry_log.close()
            if exit_log is not None:
                exit_log.close()
            if backend_log is not None:
                backend_log.close()
            if not args.keep_workspace:
                shutil.rmtree(workspace, ignore_errors=True)


    if __name__ == "__main__":
        raise SystemExit(main())
    '''
)


@dataclass(frozen=True)
class DashManifest:
    repo: str
    ref: str
    binary_name: str


@dataclass(frozen=True)
class SSHConnection:
    host: str
    user: str
    port: int
    identity_file: str

    @property
    def target(self) -> str:
        if self.user:
            return f"{self.user}@{self.host}"
        return self.host


def load_manifest() -> DashManifest:
    payload = json.loads(MANIFEST_PATH.read_text(encoding="utf-8"))
    return DashManifest(
        repo=str(payload["repo"]),
        ref=str(payload["ref"]),
        binary_name=str(payload["binary_name"]),
    )


def ssh_base_command(conn: SSHConnection) -> list[str]:
    command = ["ssh", "-p", str(conn.port), "-o", "BatchMode=yes"]
    if conn.identity_file:
        command.extend(["-i", conn.identity_file])
    command.append(conn.target)
    return command


def run_checked(
    command: list[str], *, input_text: str | None = None
) -> subprocess.CompletedProcess[str]:
    completed = subprocess.run(
        command, input=input_text, capture_output=True, text=True
    )
    if completed.returncode != 0:
        quoted = " ".join(shlex.quote(part) for part in command)
        raise RuntimeError(
            f"command failed ({completed.returncode}): {quoted}\nstdout:\n{completed.stdout}\nstderr:\n{completed.stderr}"
        )
    return completed


def run_ssh(
    conn: SSHConnection, remote_command: str, *, input_text: str | None = None
) -> subprocess.CompletedProcess[str]:
    return run_checked(ssh_base_command(conn) + [remote_command], input_text=input_text)


def create_remote_workspace(conn: SSHConnection, requested: str) -> str:
    if requested:
        remote_command = (
            f"mkdir -p {shlex.quote(requested)} && printf '%s' {shlex.quote(requested)}"
        )
        return run_ssh(conn, remote_command).stdout.strip()
    remote_command = "workspace=$(mktemp -d /tmp/flvx-dash-cleanroom.XXXXXX) && printf '%s' \"$workspace\""
    return run_ssh(conn, remote_command).stdout.strip()


def upload_backend_tree(conn: SSHConnection, remote_workspace: str) -> None:
    run_ssh(conn, f"mkdir -p {shlex.quote(remote_workspace)}")
    rsync_command = [
        "rsync",
        "-az",
        "--delete",
        "-e",
        ssh_transport_command(conn),
        f"{str(ROOT_DIR / 'go-backend')}/",
        f"{conn.target}:{remote_workspace}/go-backend/",
    ]
    run_checked(rsync_command)


def ssh_transport_command(conn: SSHConnection) -> str:
    parts = ["ssh", "-p", str(conn.port), "-o", "BatchMode=yes"]
    if conn.identity_file:
        parts.extend(["-i", conn.identity_file])
    return " ".join(shlex.quote(part) for part in parts)


def upload_install_script(conn: SSHConnection, remote_workspace: str) -> None:
    install_path = ROOT_DIR / "install.sh"
    if not install_path.exists():
        raise RuntimeError(f"missing local install.sh at {install_path}")
    remote_install = f"{remote_workspace}/install.sh"
    remote_command = (
        f"cat > {shlex.quote(remote_install)} && chmod +x {shlex.quote(remote_install)}"
    )
    run_ssh(conn, remote_command, input_text=install_path.read_text(encoding="utf-8"))


def write_remote_worker(conn: SSHConnection, remote_script_path: str) -> None:
    remote_command = f"cat > {shlex.quote(remote_script_path)} && chmod +x {shlex.quote(remote_script_path)}"
    run_ssh(conn, remote_command, input_text=REMOTE_WORKER)


def remove_remote_workspace(conn: SSHConnection, remote_workspace: str) -> None:
    run_ssh(conn, f"rm -rf {shlex.quote(remote_workspace)}")


def render_dry_run(args: argparse.Namespace, manifest: DashManifest) -> dict:
    return {
        "mode": "dry-run",
        "remote": {
            "host": args.remote_host,
            "user": args.remote_user,
            "sshPort": args.ssh_port,
            "python": args.remote_python,
        },
        "release": {
            "repo": manifest.repo,
            "ref": manifest.ref,
            "binary": manifest.binary_name,
            "installChannel": args.install_channel,
        },
        "plan": {
            "dryRunNoTouch": True,
            "backend": {
                "mode": "remote-linux",
                "command": args.backend_command,
                "isolatedPort": args.backend_port == 0,
                "timeoutSec": args.backend_timeout,
            },
            "nodes": ["entry", "exit"],
            "trafficChecks": ["tcp", "udp"],
            "steps": [
                "create_remote_workspace",
                "upload_backend",
                "upload_install_script",
                "start_backend",
                "create_entry_exit_nodes",
                "install_dash_bundle",
                "start_dash_nodes",
                "switch_runtime_and_wait_ready",
                "create_tunnel_and_forward",
                "verify_tcp_udp",
                "print_json_summary",
            ],
        },
    }

    def resolve_bundle_url(repo: str, version: str, arch: str) -> str:
        resolved_version = version.strip() or "latest"
        if resolved_version == "latest":
            req = Request(f"https://api.github.com/repos/{repo}/releases/latest")
            with urlopen(req, timeout=10) as resp:
                payload = json.loads(resp.read().decode())
            resolved_version = str(payload.get("tag_name") or "").strip()
            if not resolved_version:
                raise RuntimeError(f"could not resolve latest release for {repo}")
        return f"https://github.com/{repo}/releases/download/{resolved_version}/flvx-agent-bundle-{arch}.tar.gz"


def run_remote_validation(args: argparse.Namespace, manifest: DashManifest) -> dict:
    conn = SSHConnection(
        host=args.remote_host,
        user=args.remote_user,
        port=args.ssh_port,
        identity_file=args.ssh_identity_file,
    )
    remote_workspace = create_remote_workspace(conn, args.remote_workdir)
    remote_script_path = f"{remote_workspace}/dash_cleanroom_worker.py"
    try:
        upload_backend_tree(conn, remote_workspace)
        upload_install_script(conn, remote_workspace)
        write_remote_worker(conn, remote_script_path)

        bundle_url = args.bundle_url.strip() or resolve_bundle_url(
            "Sagit-chu/flvx", args.install_channel, "amd64"
        )

        remote_parts = [
            shlex.quote(args.remote_python),
            shlex.quote(remote_script_path),
            "--workspace",
            shlex.quote(remote_workspace),
            "--backend-command",
            shlex.quote(args.backend_command),
            "--backend-timeout",
            shlex.quote(str(args.backend_timeout)),
            "--backend-port",
            shlex.quote(str(args.backend_port)),
            "--install-channel",
            shlex.quote(args.install_channel),
            "--target-host",
            shlex.quote(args.target_host),
            "--target-port",
            shlex.quote(str(args.target_port)),
            "--bundle-url",
            shlex.quote(bundle_url),
            "--dash-binary-path",
            shlex.quote("/tmp/dash-agent-build/target/release/dash"),
        ]
        if args.keep_remote_workdir:
            remote_parts.append("--keep-workspace")

        result = run_ssh(conn, " ".join(remote_parts))
        return json.loads(result.stdout)
    finally:
        if not args.keep_remote_workdir:
            try:
                remove_remote_workspace(conn, remote_workspace)
            except Exception:
                pass


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        description="Real-machine Linux clean-room validator for Dash dual-rule forwarding"
    )
    parser.add_argument(
        "--remote-host", required=True, help="SSH host for the remote Linux machine"
    )
    parser.add_argument(
        "--remote-user", default="", help="Optional SSH user for the remote host"
    )
    parser.add_argument(
        "--ssh-port", type=int, default=22, help="SSH port for the remote host"
    )
    parser.add_argument(
        "--ssh-identity-file",
        default="",
        help="Optional SSH identity file passed to ssh -i",
    )
    parser.add_argument(
        "--remote-python",
        default="python3",
        help="Python interpreter to use on the remote Linux host",
    )
    parser.add_argument(
        "--remote-workdir",
        default="",
        help="Optional pre-created remote workspace; defaults to mktemp on the host",
    )
    parser.add_argument(
        "--backend-command",
        default=DEFAULT_BACKEND_COMMAND,
        help="Backend command to run inside the uploaded go-backend tree on the remote host",
    )
    parser.add_argument(
        "--backend-timeout",
        type=float,
        default=60.0,
        help="Seconds to wait for backend startup and node readiness",
    )
    parser.add_argument(
        "--backend-port",
        type=int,
        default=0,
        help="Optional fixed backend port on the remote host; default allocates a free isolated port",
    )
    parser.add_argument(
        "--install-channel",
        default=DEFAULT_INSTALL_CHANNEL,
        help="Release channel passed to /api/v1/node/install",
    )
    parser.add_argument(
        "--bundle-url",
        default="",
        help="Explicit agent bundle URL for remote install; defaults to the published flvx release asset",
    )
    parser.add_argument(
        "--target-host",
        default="127.0.0.1",
        help="Remote traffic target host used by the logical forward",
    )
    parser.add_argument(
        "--target-port",
        type=int,
        default=0,
        help="Existing remote target port; default starts remote TCP/UDP echo listeners",
    )
    parser.add_argument(
        "--keep-remote-workdir",
        action="store_true",
        help="Keep the remote clean-room workspace after execution",
    )
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="Print the remote execution plan JSON without touching any host",
    )
    return parser


def main() -> int:
    manifest = load_manifest()
    args = build_parser().parse_args()
    try:
        payload = (
            render_dry_run(args, manifest)
            if args.dry_run
            else run_remote_validation(args, manifest)
        )
    except Exception as exc:
        print(f"cleanroom validation failed: {exc}", file=sys.stderr)
        return 1
    json.dump(payload, sys.stdout)
    sys.stdout.write("\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
