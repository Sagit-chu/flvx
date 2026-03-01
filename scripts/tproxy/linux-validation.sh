#!/usr/bin/env bash

set -euo pipefail

PHASE=""
PORT="53053"
IFACE="eth0"
CAPTURE_SECONDS="20"
TARGET_PORT="53053"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --phase)
      PHASE="${2:-}"
      shift 2
      ;;
    --port)
      PORT="${2:-}"
      shift 2
      ;;
    --iface)
      IFACE="${2:-}"
      shift 2
      ;;
    --capture-seconds)
      CAPTURE_SECONDS="${2:-}"
      shift 2
      ;;
    --target-port)
      TARGET_PORT="${2:-}"
      shift 2
      ;;
    *)
      echo "Unknown arg: $1" >&2
      exit 1
      ;;
  esac
done

if [[ -z "$PHASE" ]]; then
  echo "Usage: bash scripts/tproxy/linux-validation.sh --phase p6-2|p6-3|p6-4 [options]" >&2
  exit 1
fi

run_p6_2() {
  echo "[P6-2] Check TPROXY rule idempotency and state"
  echo "- Port: $PORT"

  echo "\niptables rules for chain FLVX_TPROXY:"
  iptables -t mangle -S FLVX_TPROXY || true

  echo "\nip rules for fwmark 0xb:" 
  ip rule show | grep "fwmark 0xb" || true

  echo "\nipv4 route table 111:"
  ip route show table 111 || true

  echo "\nstate file:"
  if [[ -f /etc/flux_agent/tproxy_policy_state.json ]]; then
    cat /etc/flux_agent/tproxy_policy_state.json
  else
    echo "missing /etc/flux_agent/tproxy_policy_state.json"
  fi

  echo "\n[P6-2] Done"
}

run_p6_3() {
  echo "[P6-3] Capture UDP traffic for source-preserving validation"
  echo "- Interface: $IFACE"
  echo "- Capture seconds: $CAPTURE_SECONDS"
  echo "- Target port: $TARGET_PORT"

  tcpdump -ni "$IFACE" "udp port $TARGET_PORT" -vv -tt -G "$CAPTURE_SECONDS" -W 1 -w /tmp/flvx_tproxy_p6_3.pcap || true

  echo "capture file: /tmp/flvx_tproxy_p6_3.pcap"
  echo "next: compare client src tuple with target-side packet src tuple"
  echo "[P6-3] Done"
}

run_p6_4() {
  echo "[P6-4] Failure drill helper outputs"
  echo "- Port: $PORT"

  echo "\nA) capability check baseline"
  command -v ip >/dev/null && echo "ip: ok" || echo "ip: missing"
  command -v iptables >/dev/null && echo "iptables: ok" || echo "iptables: missing"

  echo "\nB) current policy route entries"
  ip rule show | grep "fwmark" || true
  ip route show table 111 || true

  echo "\nC) conflicting rule inspection"
  iptables -t mangle -S FLVX_TPROXY || true

  echo "\nD) loop guard should be validated from API create/update rejection"
  echo "[P6-4] Done"
}

case "$PHASE" in
  p6-2)
    run_p6_2
    ;;
  p6-3)
    run_p6_3
    ;;
  p6-4)
    run_p6_4
    ;;
  *)
    echo "Unsupported phase: $PHASE" >&2
    exit 1
    ;;
esac
