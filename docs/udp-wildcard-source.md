# Preserve UDP reply source addresses on Linux

The Linux UDP listener preserves the destination address of each incoming
datagram so that replies use the correct local source. This change was developed
against FLVX `3.0.1-rc4`, commit
`d9303032cad09b33cd0b7c91704fd671fbfdd078`.
It also includes the `3.0.1-rc5` runtime update at
`c794586cc7773e5f967783895d3071538442bbf7`.

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
- IPv6 multicast destinations remain part of session identity, but replies
  leave the source unspecified and retain the received interface index so the
  kernel can select a valid local unicast source.
- The destination metadata travels through existing metrics, statistics,
  admission and traffic-limiter wrappers. The exposed remote address remains
  the real client address.
- Sessions are keyed by the local destination endpoint, remote client endpoint
  and received interface index. Identical IPv4 broadcast or multicast endpoints
  on different interfaces therefore retain separate reply control information.
  An accepted session reports its actual local destination.
  Link-local IPv6 unicast and link-local or interface-local multicast
  destinations retain their interface zone, keeping identical multicast groups
  on different interfaces separate even when the client uses the same
  non-link-local IP and source port.
  IPv4 destinations, including link-local unicast and multicast, never receive
  an IPv6 zone; their interface isolation uses the separate index in the key.
- Explicitly bound UDP sockets retain their existing behavior. Non-Linux
  platforms retain their existing implementation. No configuration keys or
  panel API changes are required.

The fix affects the shared UDP listener, including services dynamically created
after startup. It does not configure listening addresses, NAT, routing or
firewalls. Multi-homed hosts still need correct source policy routing for their
uplinks; this patch does not fix SSH/TCP routing or packet loss before ingress.

## Build

Use Go 1.26.10 or a compatible newer toolchain. From the repository root:

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
An IPv6 multicast regression sends across an isolated veth pair and checks the
multicast session destination and the unicast reply, including interface zones.
A two-interface case uses one fixed ULA client endpoint to contact the same
link-local or interface-local multicast group on both interfaces and checks
distinct sessions and their individual reply-interface metadata.
IPv4 and dual-stack cases likewise use a single fixed client endpoint to send
to one multicast group or the limited broadcast address on two interfaces,
checking separate sessions and their reply-interface indices.
Link-local IPv4 unicast and multicast regressions also check that accepted
local endpoints have no IPv6 zone, on both IPv4 and dual-stack listeners.

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
go test -c -race -o /tmp/flvx-udp-packet.test ./internal/net/udp
sudo unshare --net -- sh -eu -c '
  sysctl -qw net.ipv6.conf.all.disable_ipv6=0
  ip link set lo up
  ip addr add 198.18.0.1/32 dev lo
  ip addr add 169.254.1.2/32 dev lo
  ip -6 addr add fd00:1::1/128 dev lo nodad
  ip -6 addr add fd00:2::1/128 dev lo nodad
  ip -6 addr add fd00:3::1/128 dev lo nodad
  for suffix in "" 2; do
    ip link add "mc-tx$suffix" type veth peer name "mc-rx$suffix"
    ip link set "mc-tx$suffix" addrgenmode none up
    ip link set "mc-rx$suffix" addrgenmode none up
    ip -6 addr add fe80::1/64 dev "mc-tx$suffix" nodad
    ip -6 addr add fe80::2/64 dev "mc-rx$suffix" nodad
  done
  ip addr add 198.18.1.1/24 dev mc-tx
  ip addr add 198.18.2.1/24 dev mc-tx2
  export FLUX_TEST_IPV6_ADDRS=fd00:1::1,fd00:2::1,fd00:3::1
  export FLUX_TEST_IPV4_LINK_LOCAL=169.254.1.2
  export FLUX_TEST_MULTICAST_RX=mc-rx FLUX_TEST_MULTICAST_TX=mc-tx
  export FLUX_TEST_MULTICAST_RX2=mc-rx2 FLUX_TEST_MULTICAST_TX2=mc-tx2
  /tmp/flvx-udp-destination.test -test.v -test.timeout=60s -test.count=5
  exec /tmp/flvx-udp-packet.test -test.v -test.timeout=60s -test.count=5
'
```

The addresses above are isolated test addresses, not deployment settings.
The `UDP Destination Regression` GitHub Actions workflow runs these namespace
tests, the race detector, both agent module suites, and a Linux agent build.
