# Preserve UDP reply source addresses on Linux

The Linux UDP listener preserves the destination address of each incoming
datagram so that replies use the correct local source. This change was developed
against FLVX `3.0.1-rc4`, commit
`d9303032cad09b33cd0b7c91704fd671fbfdd078`.

## Problem

A wildcard UDP socket bound to `0.0.0.0:port` or `[::]:port` can receive datagrams
addressed to several local IP addresses. Previously, the listener retained only
the client address. A reply sent through `WriteTo` could therefore use a source
IP chosen by the kernel instead of the IP the client contacted. DNS clients, for
example, can reject such replies as coming from an unexpected source.

The UDP session pool was also keyed only by the client address and port. When
one client socket contacted multiple local addresses on the same listener port,
these requests could share a session despite having different destinations.

## Changes

- Linux wildcard UDP sockets enable IPv4/IPv6 packet information. `recvmsg`
  preserves the datagram's local destination and interface; `sendmsg` uses the
  corresponding source address and interface when replying.
- IPv4 keeps the original destination (`ipi_addr`) separate from the local
  reply address (`ipi_spec_dst`), so broadcast requests receive unicast replies
  rather than attempting to send with a broadcast source address.
- The destination metadata travels through existing metrics, statistics,
  admission and traffic-limiter wrappers. The exposed remote address remains
  the real client address.
- Sessions are keyed by both the local destination endpoint and the remote
  client endpoint. An accepted session reports its actual local destination.
- Explicitly bound UDP sockets retain their existing behavior. Non-Linux
  platforms retain their existing implementation. No configuration keys or
  panel API changes are required.

The fix affects the shared UDP listener, including services dynamically created
after startup. It does not configure listening addresses, NAT, routing or
firewalls. Multi-homed hosts still need correct source policy routing for their
uplinks; this patch does not fix SSH/TCP routing or packet loss before ingress.

## Build

Use Go 1.25.4 or a compatible newer toolchain. From the repository root:

```sh
cd go-gost
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath \
  -ldflags '-s -w' -o flux_agent .
```

Use the agent with the normal panel integration and forwarding configuration.
Selecting a wildcard listener enables destination preservation automatically
on Linux. The patch does not select a listener address or change the upstream
connection's outbound interface.

## Regression tests

The Linux destination tests cover IPv4 wildcard and dual-stack wildcard sockets,
explicit IPv4/IPv6 bindings, one client source port contacting multiple local
destinations, repeated requests, session expiry, and active metrics, statistics,
admission and traffic-limit wrappers. A separate IPv6 case binds the client to
`::1` and checks the exact remote IP and port for every destination. Broadcast
tests exercise both IPv4 and dual-stack listeners, retaining the broadcast
destination while checking that replies use a local unicast address.

Run the standard suites on Linux:

```sh
(cd go-gost && go test ./...)
(cd go-gost/x && go test ./...)
```

To exercise multiple IPv6 addresses without modifying the host's live network,
compile the tests first and run them inside a disposable network namespace.
The following commands require Linux, root access for `unshare`, and `iproute2`:

```sh
cd go-gost/x
go test -c -race -o /tmp/flvx-udp-destination.test ./listener/udp
sudo unshare --net -- sh -eu -c '
  sysctl -qw net.ipv6.conf.all.disable_ipv6=0
  ip link set lo up
  ip -6 addr add fd00:1::1/128 dev lo nodad
  ip -6 addr add fd00:2::1/128 dev lo nodad
  ip -6 addr add fd00:3::1/128 dev lo nodad
  FLUX_TEST_IPV6_ADDRS=fd00:1::1,fd00:2::1,fd00:3::1 \
    exec /tmp/flvx-udp-destination.test -test.v -test.timeout=60s -test.count=5
'
```

The addresses above are isolated test addresses, not deployment settings.
The `UDP Destination Regression` GitHub Actions workflow runs these namespace
tests, the race detector, both agent module suites, and a Linux agent build.
