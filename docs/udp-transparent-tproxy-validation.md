# UDP Transparent TPROXY Validation Guide

This guide covers Linux validation for transparent UDP forwarding (`udpMode=transparent`) and maps to plan items P6-2, P6-3, and P6-4.

## Scope

- Verify TPROXY policy rule idempotency and cleanup
- Verify restart recovery behavior from state file
- Verify source-preserving UDP path with packet capture
- Verify common failure drills and expected outcomes

## Prerequisites

- Linux node with root privileges
- `iptables`, `ip`, `tcpdump`, and `ss` installed
- Agent version with `EnsureTProxyPolicy` / `DeleteTProxyPolicy`
- Panel and node connectivity healthy

## Quick Commands

Run automated checks from repository root:

```bash
bash scripts/tproxy/linux-validation.sh --phase p6-2 --port 53053
bash scripts/tproxy/linux-validation.sh --phase p6-3 --iface eth0 --capture-seconds 20 --target-port 53053
bash scripts/tproxy/linux-validation.sh --phase p6-4 --port 53053
```

## P6-2 Linux Integration Validation

### 1) Idempotent ensure

1. Create or update one forward to `udpMode=transparent` on a fixed port.
2. Trigger redeploy twice.
3. Validate no duplicate rules:

```bash
iptables -t mangle -S FLVX_TPROXY | grep -- "--dport <PORT>"
ip rule show | grep "fwmark 0xb"
ip route show table 111
```

Expected:

- Exactly one TPROXY rule for target port
- One effective policy rule path for mark/table
- Local route exists for table 111

### 2) Delete cleanup

1. Delete the forward.
2. Validate rule removed:

```bash
iptables -t mangle -S FLVX_TPROXY | grep -- "--dport <PORT>"
```

Expected:

- No remaining rule for deleted port

### 3) Restart recovery

1. Ensure at least one transparent forward exists.
2. Confirm state file present:

```bash
cat /etc/flux_agent/tproxy_policy_state.json
```

3. Restart agent service.
4. Validate rule restored for each entry in state file.

## P6-3 E2E Source-Preserving Verification

### Topology

- Client -> Entry Node (transparent UDP listener) -> Target UDP server

### Procedure

1. Start packet capture on entry node ingress interface.
2. Start packet capture on target server interface.
3. Send UDP traffic from client to forwarding entry address/port.
4. Compare source IP/port seen at target with client tuple.

Capture examples:

```bash
tcpdump -ni <IFACE> udp port <PORT>
tcpdump -ni any udp and host <TARGET_IP> and port <TARGET_PORT>
```

Success criteria:

- Target-side packet source equals client source tuple
- No source NAT behavior observed on forwarding node

## P6-4 Failure Drill Matrix

### A) Missing capability / unsupported platform

- Simulate non-Linux or remove prerequisites (`ip`/`iptables`).
- Expected: deployment fails with clear capability error; no partial rules applied.

### B) Missing policy route

- Remove `ip rule` or table route manually.
- Expected: diagnose payload includes TPROXY error fields; runtime degradation visible.

### C) Rule conflict

- Pre-insert conflicting mark/table or chain entries.
- Expected: ensure path reports failure details and rollback path executes.

### D) Loop misconfiguration

- Set target equal to node local address with same listen port.
- Expected: create/update rejected by configuration guardrail.

## Evidence Checklist

- Command output snapshots for `iptables`, `ip rule`, `ip route`
- `forward/diagnose` response containing `tproxy` object
- Packet capture snippets proving source preservation
- Failure drill logs for at least one scenario in each category
