package handler

import (
	"testing"
)

// ---------------------------------------------------------------------------
// nodeSupportsV4 / nodeSupportsV6
// ---------------------------------------------------------------------------

func TestNodeSupportsV4_Nil(t *testing.T) {
	if nodeSupportsV4(nil) {
		t.Fatal("nil node must not support v4")
	}
}

func TestNodeSupportsV6_Nil(t *testing.T) {
	if nodeSupportsV6(nil) {
		t.Fatal("nil node must not support v6")
	}
}

func TestNodeSupportsV4_ExplicitV4(t *testing.T) {
	n := &nodeRecord{ServerIPv4: "10.0.0.1"}
	if !nodeSupportsV4(n) {
		t.Fatal("explicit server_ip_v4 must support v4")
	}
}

func TestNodeSupportsV6_ExplicitV6(t *testing.T) {
	n := &nodeRecord{ServerIPv6: "2001:db8::1"}
	if !nodeSupportsV6(n) {
		t.Fatal("explicit server_ip_v6 must support v6")
	}
}

func TestNodeSupportsV4_OnlyV6Set(t *testing.T) {
	n := &nodeRecord{ServerIPv6: "2001:db8::1"}
	if nodeSupportsV4(n) {
		t.Fatal("node with only v6 should not support v4")
	}
}

func TestNodeSupportsV6_OnlyV4Set(t *testing.T) {
	n := &nodeRecord{ServerIPv4: "10.0.0.1"}
	if nodeSupportsV6(n) {
		t.Fatal("node with only v4 should not support v6")
	}
}

func TestNodeSupportsV4_DualStack(t *testing.T) {
	n := &nodeRecord{ServerIPv4: "10.0.0.1", ServerIPv6: "2001:db8::1"}
	if !nodeSupportsV4(n) {
		t.Fatal("dual-stack node must support v4")
	}
}

func TestNodeSupportsV6_DualStack(t *testing.T) {
	n := &nodeRecord{ServerIPv4: "10.0.0.1", ServerIPv6: "2001:db8::1"}
	if !nodeSupportsV6(n) {
		t.Fatal("dual-stack node must support v6")
	}
}

func TestNodeSupportsV4_LegacyV4Only(t *testing.T) {
	n := &nodeRecord{ServerIP: "192.168.1.1"}
	if !nodeSupportsV4(n) {
		t.Fatal("legacy v4 ip in server_ip must support v4")
	}
	if nodeSupportsV6(n) {
		t.Fatal("legacy v4 ip in server_ip must not support v6")
	}
}

func TestNodeSupportsV6_LegacyV6Only(t *testing.T) {
	n := &nodeRecord{ServerIP: "2001:db8::1"}
	if !nodeSupportsV6(n) {
		t.Fatal("legacy v6 ip in server_ip must support v6")
	}
	if nodeSupportsV4(n) {
		t.Fatal("legacy v6 ip in server_ip must not support v4")
	}
}

func TestNodeSupportsV4_EmptyNode(t *testing.T) {
	n := &nodeRecord{}
	if nodeSupportsV4(n) {
		t.Fatal("empty node must not support v4")
	}
	if nodeSupportsV6(n) {
		t.Fatal("empty node must not support v6")
	}
}

func TestNodeSupportsV4_LegacyBracketed(t *testing.T) {
	n := &nodeRecord{ServerIP: "[::1]"}
	if nodeSupportsV4(n) {
		t.Fatal("bracketed ipv6 must not support v4")
	}
	if !nodeSupportsV6(n) {
		t.Fatal("bracketed ipv6 must support v6")
	}
}

// ---------------------------------------------------------------------------
// pickNodeAddressV4 / pickNodeAddressV6
// ---------------------------------------------------------------------------

func TestPickNodeAddressV4_Nil(t *testing.T) {
	if pickNodeAddressV4(nil) != "" {
		t.Fatal("nil node must return empty")
	}
}

func TestPickNodeAddressV6_Nil(t *testing.T) {
	if pickNodeAddressV6(nil) != "" {
		t.Fatal("nil node must return empty")
	}
}

func TestPickNodeAddressV4_PreferExplicit(t *testing.T) {
	n := &nodeRecord{ServerIPv4: "10.0.0.1", ServerIP: "192.168.0.1"}
	got := pickNodeAddressV4(n)
	if got != "10.0.0.1" {
		t.Fatalf("expected explicit v4 10.0.0.1, got %q", got)
	}
}

func TestPickNodeAddressV4_FallbackLegacy(t *testing.T) {
	n := &nodeRecord{ServerIP: "192.168.0.1"}
	got := pickNodeAddressV4(n)
	if got != "192.168.0.1" {
		t.Fatalf("expected legacy 192.168.0.1, got %q", got)
	}
}

func TestPickNodeAddressV6_PreferExplicit(t *testing.T) {
	n := &nodeRecord{ServerIPv6: "2001:db8::1", ServerIP: "::1"}
	got := pickNodeAddressV6(n)
	if got != "2001:db8::1" {
		t.Fatalf("expected explicit v6 2001:db8::1, got %q", got)
	}
}

func TestPickNodeAddressV6_FallbackLegacy(t *testing.T) {
	n := &nodeRecord{ServerIP: "::1"}
	got := pickNodeAddressV6(n)
	if got != "::1" {
		t.Fatalf("expected legacy ::1, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// selectTunnelDialHost — core IP preference selection logic
// ---------------------------------------------------------------------------

func dualStackNode(name, v4, v6 string) *nodeRecord {
	return &nodeRecord{
		Name:       name,
		ServerIPv4: v4,
		ServerIPv6: v6,
	}
}

func v4OnlyNode(name, v4 string) *nodeRecord {
	return &nodeRecord{
		Name:       name,
		ServerIPv4: v4,
	}
}

func v6OnlyNode(name, v6 string) *nodeRecord {
	return &nodeRecord{
		Name:       name,
		ServerIPv6: v6,
	}
}

func TestSelectTunnelDialHost_NilNodes(t *testing.T) {
	_, err := selectTunnelDialHost(nil, nil, "")
	if err == nil {
		t.Fatal("expected error for nil nodes")
	}
	_, err = selectTunnelDialHost(dualStackNode("a", "1.1.1.1", "::1"), nil, "")
	if err == nil {
		t.Fatal("expected error for nil toNode")
	}
	_, err = selectTunnelDialHost(nil, dualStackNode("b", "1.1.1.1", "::1"), "")
	if err == nil {
		t.Fatal("expected error for nil fromNode")
	}
}

func TestSelectTunnelDialHost_DualStack_DefaultPreference(t *testing.T) {
	from := dualStackNode("from", "10.0.0.1", "2001:db8::1")
	to := dualStackNode("to", "10.0.0.2", "2001:db8::2")

	host, err := selectTunnelDialHost(from, to, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Default prefers v4 when both available
	if host != "10.0.0.2" {
		t.Fatalf("default preference should pick v4, got %q", host)
	}
}

func TestSelectTunnelDialHost_DualStack_PreferV4(t *testing.T) {
	from := dualStackNode("from", "10.0.0.1", "2001:db8::1")
	to := dualStackNode("to", "10.0.0.2", "2001:db8::2")

	host, err := selectTunnelDialHost(from, to, "v4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if host != "10.0.0.2" {
		t.Fatalf("v4 preference should pick v4 address, got %q", host)
	}
}

func TestSelectTunnelDialHost_DualStack_PreferV6(t *testing.T) {
	from := dualStackNode("from", "10.0.0.1", "2001:db8::1")
	to := dualStackNode("to", "10.0.0.2", "2001:db8::2")

	host, err := selectTunnelDialHost(from, to, "v6")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if host != "2001:db8::2" {
		t.Fatalf("v6 preference should pick v6 address, got %q", host)
	}
}

func TestSelectTunnelDialHost_V4Only_PreferV6Fallback(t *testing.T) {
	from := v4OnlyNode("from", "10.0.0.1")
	to := v4OnlyNode("to", "10.0.0.2")

	// User prefers v6, but both nodes are v4-only — should fallback to v4
	host, err := selectTunnelDialHost(from, to, "v6")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if host != "10.0.0.2" {
		t.Fatalf("v6 preference on v4-only nodes should fallback to v4, got %q", host)
	}
}

func TestSelectTunnelDialHost_V6Only_PreferV4Fallback(t *testing.T) {
	from := v6OnlyNode("from", "2001:db8::1")
	to := v6OnlyNode("to", "2001:db8::2")

	// User prefers v4, but both nodes are v6-only — should fallback to v6
	host, err := selectTunnelDialHost(from, to, "v4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if host != "2001:db8::2" {
		t.Fatalf("v4 preference on v6-only nodes should fallback to v6, got %q", host)
	}
}

func TestSelectTunnelDialHost_Incompatible(t *testing.T) {
	from := v4OnlyNode("from", "10.0.0.1")
	to := v6OnlyNode("to", "2001:db8::2")

	_, err := selectTunnelDialHost(from, to, "")
	if err == nil {
		t.Fatal("expected error for incompatible nodes (v4-only -> v6-only)")
	}
}

func TestSelectTunnelDialHost_Incompatible_Reverse(t *testing.T) {
	from := v6OnlyNode("from", "2001:db8::1")
	to := v4OnlyNode("to", "10.0.0.2")

	_, err := selectTunnelDialHost(from, to, "")
	if err == nil {
		t.Fatal("expected error for incompatible nodes (v6-only -> v4-only)")
	}
}

func TestSelectTunnelDialHost_WhitespacePreference(t *testing.T) {
	from := dualStackNode("from", "10.0.0.1", "2001:db8::1")
	to := dualStackNode("to", "10.0.0.2", "2001:db8::2")

	// Whitespace should be trimmed, treated as "v6"
	host, err := selectTunnelDialHost(from, to, "  v6  ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if host != "2001:db8::2" {
		t.Fatalf("trimmed v6 preference should pick v6 address, got %q", host)
	}
}

func TestSelectTunnelDialHost_MixedStack_FromDualToV4(t *testing.T) {
	from := dualStackNode("from", "10.0.0.1", "2001:db8::1")
	to := v4OnlyNode("to", "10.0.0.2")

	// v6 preferred, but target only has v4 — should succeed with v4
	host, err := selectTunnelDialHost(from, to, "v6")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if host != "10.0.0.2" {
		t.Fatalf("should fallback to v4 when target is v4-only, got %q", host)
	}
}

func TestSelectTunnelDialHost_MixedStack_FromDualToV6(t *testing.T) {
	from := dualStackNode("from", "10.0.0.1", "2001:db8::1")
	to := v6OnlyNode("to", "2001:db8::2")

	// v4 preferred, but target only has v6 — should succeed with v6
	host, err := selectTunnelDialHost(from, to, "v4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if host != "2001:db8::2" {
		t.Fatalf("should fallback to v6 when target is v6-only, got %q", host)
	}
}

func TestSelectTunnelDialHost_MixedStack_FromV4ToDual(t *testing.T) {
	from := v4OnlyNode("from", "10.0.0.1")
	to := dualStackNode("to", "10.0.0.2", "2001:db8::2")

	// v6 preferred, but from only has v4 — should use v4 (from can only reach v4 of target)
	host, err := selectTunnelDialHost(from, to, "v6")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if host != "10.0.0.2" {
		t.Fatalf("should use v4 when from is v4-only, got %q", host)
	}
}

func TestSelectTunnelDialHost_MixedStack_FromV6ToDual(t *testing.T) {
	from := v6OnlyNode("from", "2001:db8::1")
	to := dualStackNode("to", "10.0.0.2", "2001:db8::2")

	// v4 preferred, but from only has v6 — should use v6
	host, err := selectTunnelDialHost(from, to, "v4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if host != "2001:db8::2" {
		t.Fatalf("should use v6 when from is v6-only, got %q", host)
	}
}

// ---------------------------------------------------------------------------
// nodeDisplayName
// ---------------------------------------------------------------------------

func TestNodeDisplayName_Nil(t *testing.T) {
	got := nodeDisplayName(nil)
	if got != "node" {
		t.Fatalf("nil node display name should be 'node', got %q", got)
	}
}

func TestNodeDisplayName_Named(t *testing.T) {
	n := &nodeRecord{ID: 42, Name: "hk-node"}
	got := nodeDisplayName(n)
	if got != "hk-node" {
		t.Fatalf("expected 'hk-node', got %q", got)
	}
}

func TestNodeDisplayName_Unnamed(t *testing.T) {
	n := &nodeRecord{ID: 42}
	got := nodeDisplayName(n)
	if got != "node_42" {
		t.Fatalf("expected 'node_42', got %q", got)
	}
}
