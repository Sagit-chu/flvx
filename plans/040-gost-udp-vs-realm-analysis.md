# GOST UDP vs Realm UDP Analysis

**Goal:** Compare GOST UDP forwarding through tunnels with Realm's UDP implementation to understand why users report GOST UDP forwarding has problems while Realm works correctly.

## Task Checklist

- [x] Analyze GOST UDP tunnel architecture
- [x] Analyze Realm UDP relay architecture
- [x] Identify architectural differences
- [x] Identify potential issues in GOST implementation
- [ ] Document findings and recommendations

---

## GOST UDP Architecture

### Core Components

1. **UDP Relay** (`x/internal/net/udp/relay.go`)
   - Simple bidirectional packet copying between two `net.PacketConn` interfaces
   - Uses two goroutines: one for each direction
   - Sequential packet processing (no batching)
   - No idle timeout or association tracking

2. **UDP over Tunnel** (`x/handler/relay/bind.go`, `x/handler/socks/v5/udp_tun.go`)
   - Wraps UDP data with SOCKS5-style framing via `UDPTunServerConn()`
   - Adds address headers to each packet
   - Uses smux multiplexing for tunnel connections

3. **SOCKS5 UDP Framing** (`x/internal/util/socks/conn.go`, `x/internal/util/relay/conn.go`)
   - `udpTunConn.ReadFrom()`: Parses SOCKS5 UDP header to extract target address
   - `udpTunConn.WriteTo()`: Wraps data with SOCKS5 UDP header including:
     - RSV (data length, 2 bytes)
     - Frag (0xff for tunnel relay, 1 byte)
     - Address (4/16/1+n bytes for IPv4/IPv6/domain)

4. **Multiplexing** (`x/internal/util/mux/mux.go`)
   - Uses smux v1.5.31 for connection multiplexing
   - Adds stream framing and flow control overhead

### Data Flow (UDP over Tunnel)

```
Client UDP packet
    ↓
[Local GOST] SOCKS5 UDP framing (add ~10-26 bytes)
    ↓
smux stream (add framing, flow control)
    ↓
TCP tunnel to remote
    ↓
[Remote GOST] smux demux
    ↓
SOCKS5 UDP deframing
    ↓
Forward to target
```

---

## Realm UDP Architecture

### Core Components

1. **Association Model** (`realm_core/src/udp/middle.rs`)
   - Per-client socket associations stored in `SockMap`
   - Creates dedicated remote socket per client address
   - Spawns `send_back` task for return path
   - Association timeout for cleanup

2. **Batched I/O** (`realm_core/src/udp/batched.rs`)
   - Uses `recvmmsg/sendmmsg` on Linux
   - Up to 128 packets per batch
   - Significantly higher throughput for high-PPS traffic

3. **No Protocol Overhead**
   - Plain UDP forwarding without adding protocol headers
   - No SOCKS5 framing or mux layer

### Data Flow

```
Client UDP packet
    ↓
[Realm] Direct relay via association socket
    ↓
Target server
```

---

## Key Architectural Differences

| Aspect | GOST | Realm |
|--------|------|-------|
| **UDP over Tunnel** | SOCKS5 framing + smux multiplexing | N/A (direct relay only) |
| **Protocol Overhead** | Extra ~10-26 bytes per packet | None |
| **I/O Model** | Standard Go net.PacketConn | Batched I/O (recvmmsg/sendmmsg) |
| **Connection Tracking** | Via mux session | SockMap with timeout |
| **Association Model** | None (bidirectional copy only) | Per-client socket association |
| **Idle Timeout** | None on UDP relay | Association timeout |
| **Throughput** | Single packet per syscall | Up to 128 packets per syscall |

---

## Identified Potential Issues in GOST

### 1. No Batch I/O Support
- **Impact**: High PPS (packets per second) traffic incurs syscall overhead
- **Realm advantage**: `recvmmsg/sendmmsg` batches up to 128 packets
- **Evidence**: Realm's `batched.rs` implements this; GOST uses standard `ReadFrom/WriteTo`

### 2. Protocol Overhead Per Packet
- **Impact**: Bandwidth waste, extra processing for framing/deframing
- **Overhead**: SOCKS5 UDP header adds ~10 bytes (IPv4) to ~26 bytes (IPv6) per packet
- **Evidence**: `x/internal/util/socks/conn.go:63-78` shows framing overhead

### 3. Mux Layer Overhead
- **Impact**: Latency and throughput degradation
- **smux adds**: Stream framing, flow control, potential backpressure
- **Evidence**: `x/internal/util/mux/mux.go` wraps all tunnel connections

### 4. No Association Tracking
- **Impact**: Cannot properly handle NAT translation for return traffic
- **Realm approach**: `SockMap` tracks client→remote socket mapping
- **Evidence**: GOST's `udp.Relay` just copies packets bidirectionally

### 5. No Idle Timeout on UDP Relay
- **Impact**: Stale connections may persist indefinitely
- **Evidence**: `udp.Relay.Run()` blocks until error or context cancel
- **Contrast**: Realm has association timeout for cleanup

### 6. Sequential Packet Processing
- **Impact**: Cannot pipeline multiple packets
- **Evidence**: `relay.go:44-77` shows sequential `ReadFrom` → `WriteTo` loop
- **Contrast**: Realm's batched I/O handles multiple packets concurrently

### 7. Read Deadline on Underlying TCP
- **Impact**: Could cause unexpected connection termination
- **Default**: 15-second read timeout set on connections
- **Evidence**: `x/handler/relay/metadata.go:readTimeout` defaults to 15s
- **Issue**: UDP relay may not handle deadline properly

---

## Recommendations

1. **Consider Direct UDP Mode**: For scenarios where tunnel is not required, use direct UDP relay (no SOCKS5 framing)

2. **Add Association Tracking**: Implement client→remote socket mapping with timeout

3. **Investigate Batch I/O**: Consider using `recvmmsg/sendmmsg` equivalent in Go (via `x/net` or raw syscalls)

4. **Add Idle Timeout**: Implement timeout-based cleanup for UDP associations

5. **Reduce Framing Overhead**: Consider more compact framing for tunnel UDP

---

## Files Analyzed

### GOST Files
- `go-gost/x/internal/net/udp/relay.go` - Core UDP relay logic
- `go-gost/x/internal/util/mux/mux.go` - smux multiplexing
- `go-gost/x/internal/util/socks/conn.go` - SOCKS5 UDP framing
- `go-gost/x/internal/util/relay/conn.go` - Relay UDP framing
- `go-gost/x/handler/relay/bind.go` - Relay BIND handler
- `go-gost/x/handler/socks/v5/udp_tun.go` - SOCKS5 UDP tunnel handler
- `go-gost/x/handler/tunnel/bind.go` - Tunnel BIND handler
- `go-gost/x/connector/tunnel/bind.go` - Tunnel connector BIND
- `go-gost/x/connector/tunnel/conn.go` - Tunnel connection types
- `go-gost/x/connector/tunnel/listener.go` - Tunnel bind listener

### Realm Files (fetched from GitHub)
- `realm_core/src/udp/mod.rs` - UDP relay entry point
- `realm_core/src/udp/middle.rs` - Association and relay logic with SockMap
- `realm_core/src/udp/socket.rs` - UDP socket binding and association
- `realm_core/src/udp/batched.rs` - Batched I/O implementation