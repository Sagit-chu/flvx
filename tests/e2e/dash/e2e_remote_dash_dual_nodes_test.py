import json
import os
import subprocess
import time
import urllib.request
import re
import sys

PANEL_HOST = "3.38.98.31"
PANEL_URL = f"http://{PANEL_HOST}:6365"
NODE1_HOST = "3.38.98.31"
NODE2_HOST = "20.118.172.127"
SSH_KEY = os.path.expanduser("~/.ssh/id_ed25519")

def post_json(url, payload, token=None, retries=3):
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = token
    req = urllib.request.Request(url, data=json.dumps(payload).encode(), headers=headers, method="POST")
    for i in range(retries):
        try:
            with urllib.request.urlopen(req) as resp:
                return json.loads(resp.read().decode())
        except urllib.error.HTTPError as e:
            if e.code == 502 and i < retries - 1:
                print(f"502 Bad Gateway on {url}, retrying...")
                time.sleep(2)
                continue
            print(f"HTTPError on {url}: {e.code} - {e.read().decode()}")
            raise

def put_json(url, payload, token=None):
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = token
    req = urllib.request.Request(url, data=json.dumps(payload).encode(), headers=headers, method="PUT")
    with urllib.request.urlopen(req) as resp:
        return json.loads(resp.read().decode())

def get_json(url, token=None):
    headers = {}
    if token:
        headers["Authorization"] = token
    req = urllib.request.Request(url, headers=headers, method="GET")
    with urllib.request.urlopen(req) as resp:
        return json.loads(resp.read().decode())

def ssh_cmd(host, cmd, capture=False):
    ssh_cmd_list = [
        "ssh", "-o", "StrictHostKeyChecking=no", "-i", SSH_KEY, f"root@{host}", cmd
    ]
    if capture:
        return subprocess.run(ssh_cmd_list, capture_output=True, text=True)
    return subprocess.run(ssh_cmd_list)

def get_tunnel_id_by_name(name, token):
    tunnels = post_json(f"{PANEL_URL}/api/v1/tunnel/list", {"page":1, "pageSize": 100}, token)
    for t in tunnels.get("data", []):
        if t["name"] == name:
            return t["id"]
    raise Exception(f"Tunnel {name} not found")

def deploy_node(token, name, host, port_range):
    print(f"Creating node {name} on {host}...")
    post_json(f"{PANEL_URL}/api/v1/node/create", {
        "name": name,
        "serverIp": host,
        "tcpListenAddr": "0.0.0.0",
        "port": port_range
    }, token)
    
    nodes_res = post_json(f"{PANEL_URL}/api/v1/node/list", {"page":1, "pageSize": 100}, token)
    node = next((n for n in nodes_res["data"] if n["name"] == name), None)
    if not node:
        raise Exception(f"Node {name} not found after creation.")
    node_id = node["id"]
    
    install_res = post_json(f"{PANEL_URL}/api/v1/node/install", {"id": node_id}, token)
    cmd = install_res["data"]
    cmd = re.sub(r"curl -L [^\s]+ -o \./install\.sh", "curl -L https://raw.githubusercontent.com/Sagit-chu/flvx/main/install.sh -o ./install.sh", cmd)
    cmd = cmd.replace("VERSION=2.1.9", "VERSION=3.0.0-alpha1")
    
    print(f"Deploying agent on {host}...")
    ssh_cmd(host, "systemctl stop flux_agent dash; rm -rf /etc/flux_agent /usr/local/bin/flux_agent /usr/local/bin/dash; rm -f install.sh")
    
    res = ssh_cmd(host, cmd)
    if res.returncode != 0:
        raise Exception(f"Failed to deploy agent on {host}")
        
    ssh_cmd(host, "rm -f /etc/flux_agent/relay-state.yaml /etc/flux_agent/dash.yaml /etc/flux_agent/exit.yaml /etc/flux_agent/exit-state.yaml")
    ssh_cmd(host, "sed -i 's/StandardOutput=null/StandardOutput=journal/' /etc/systemd/system/flux_agent.service && sed -i 's/StandardError=null/StandardError=journal/' /etc/systemd/system/flux_agent.service && systemctl daemon-reload")
    ssh_cmd(host, "systemctl restart flux_agent")
        
    print(f"Waiting for {name} to become online...")
    for _ in range(30):
        nodes = post_json(f"{PANEL_URL}/api/v1/node/list", {"page":1, "pageSize":100}, token)
        node = next((n for n in nodes["data"] if n["id"] == node_id), None)
        if node and node.get("status") == 1:
            print(f"{name} is online.")
            return node_id
        time.sleep(1)
        
    ssh_cmd(host, "journalctl -u flux_agent -n 50")
    raise Exception(f"Node {name} failed to come online.")

def main():
    print("Testing remote E2E Dash flow (Dual Nodes)...")

    login_res = post_json(f"{PANEL_URL}/api/v1/user/login", {"username": "flvx", "password": "flvxflvx"})
    token = login_res["data"]["token"]
    print("Logged in successfully.")

    post_json(f"{PANEL_URL}/api/v1/config/update-single", {"name": "ip", "value": f"{PANEL_HOST}:6365"}, token)
    
    nodes_res = post_json(f"{PANEL_URL}/api/v1/node/list", {"page":1, "pageSize": 100}, token)
    for n in nodes_res["data"]:
        post_json(f"{PANEL_URL}/api/v1/node/delete", {"id": n["id"]}, token)
        
    tunnels_res = post_json(f"{PANEL_URL}/api/v1/tunnel/list", {"page":1, "pageSize": 100}, token)
    for t in tunnels_res["data"]:
        post_json(f"{PANEL_URL}/api/v1/tunnel/delete", {"id": t["id"]}, token)
        
    forwards_res = post_json(f"{PANEL_URL}/api/v1/forward/list", {"page":1, "pageSize": 100}, token)
    for f in forwards_res.get("data", []):
        post_json(f"{PANEL_URL}/api/v1/forward/delete", {"ids": [f["id"]]}, token)

    put_json(f"{PANEL_URL}/api/v1/system/runtime", {"engine": "dash"}, token)
    print("Switched runtime engine to dash")

    for _ in range(10):
        settings = get_json(f"{PANEL_URL}/api/v1/system/runtime", token)
        if settings["data"]["currentEngine"] == "dash":
            break
        time.sleep(1)

    node1_id = deploy_node(token, "node1-panel", NODE1_HOST, "20000-20100")
    node2_id = deploy_node(token, "node2-remote", NODE2_HOST, "20000-20100")
    
    print("\n--- Test 1: Forward Type (Type 1) ---")
    t1_name = f"type1-forward-{int(time.time())}"
    tunnel1_res = post_json(f"{PANEL_URL}/api/v1/tunnel/create", {
        "name": t1_name,
        "type": 1,
        "flow": 1,
        "inNodeId": [{"nodeId": node1_id, "protocol": "tcp"}],
        "status": 1
    }, token)
    tunnel1_id = get_tunnel_id_by_name(t1_name, token)
    
    forward1_res = post_json(f"{PANEL_URL}/api/v1/forward/create", {
        "name": "fwd1",
        "tunnelId": tunnel1_id,
        "nodeId": node1_id,
        "inIp": "0.0.0.0",
        "inPort": 20001,
        "remoteAddr": f"{NODE2_HOST}:20081",
        "protocol": "tcp",
        "strategy": "round"
    }, token)
    
    ssh_cmd(NODE2_HOST, "python3 -m http.server 20081 >/dev/null 2>&1 &")
    time.sleep(5)
    
    print("Verifying Forward Type 1 traffic...")
    curl1_res = ssh_cmd(NODE1_HOST, "curl -s -m 10 http://127.0.0.1:20001", capture=True)
    ssh_cmd(NODE2_HOST, "pkill -f 'python3 -m http.server 20081' || true")
    
    if curl1_res.returncode == 0 and "Directory listing" in curl1_res.stdout:
        print("✅ Type 1 (Forward) success!")
    else:
        print("❌ Type 1 (Forward) failed!")
        print(curl1_res.stderr, curl1_res.stdout)
        sys.exit(1)

    print("\n--- Test 2: Tunnel Type (Type 2) ---")
    t2_name = f"type2-tunnel-{int(time.time())}"
    tunnel2_res = post_json(f"{PANEL_URL}/api/v1/tunnel/create", {
        "name": t2_name,
        "type": 2,
        "protocol": "tls",
        "flow": 1,
        "inNodeId": [{"nodeId": node1_id, "protocol": "tls"}],
        "outNodeId": [{"nodeId": node2_id, "protocol": "tls"}],
        "chainNodes": [],
        "status": 1
    }, token)
    tunnel2_id = get_tunnel_id_by_name(t2_name, token)
    
    forward2_res = post_json(f"{PANEL_URL}/api/v1/forward/create", {
        "name": "fwd2",
        "tunnelId": tunnel2_id,
        "nodeId": node1_id,
        "inIp": "0.0.0.0",
        "inPort": 20002,
        "remoteAddr": "127.0.0.1:20082",
        "protocol": "tcp",
        "strategy": "round"
    }, token)
    if forward2_res.get("code") != 0:
        print(f"Forward 2 create failed: {forward2_res}")
        sys.exit(1)
    
    ssh_cmd(NODE2_HOST, "python3 -m http.server 20082 >/dev/null 2>&1 &")
    time.sleep(5)
    
    print("Verifying Tunnel Type 2 traffic...")
    curl2_res = ssh_cmd(NODE1_HOST, "curl -s -m 10 http://127.0.0.1:20002", capture=True)
    ssh_cmd(NODE2_HOST, "pkill -f 'python3 -m http.server 20082' || true")
    
    if curl2_res.returncode == 0 and "Directory listing" in curl2_res.stdout:
        print("✅ Type 2 (Tunnel) success!")
    else:
        print("❌ Type 2 (Tunnel) failed!")
        print(curl2_res.stderr, curl2_res.stdout)
        sys.exit(1)

    print("\n🎉 All E2E Tests completed successfully.")

if __name__ == "__main__":
    main()
