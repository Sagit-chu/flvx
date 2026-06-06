package nftables

import (
	"strings"
	"testing"
)

func TestRenderTableIncludesDNATAndMasquerade(t *testing.T) {
	script := RenderTable(NodePlan{
		NodeID: 10,
		Rules: []Rule{
			{
				ForwardID:  42,
				InPort:     24000,
				TargetHost: "198.51.100.20",
				TargetPort: 443,
				Protocols:  []string{"tcp", "udp"},
			},
		},
	})

	expectedParts := []string{
		"table inet flvx",
		"type nat hook prerouting priority dstnat; policy accept;",
		"type nat hook postrouting priority srcnat; policy accept;",
		"tcp dport 24000 counter dnat ip to 198.51.100.20:443 comment \"flvx forward:42 dnat tcp\"",
		"udp dport 24000 counter dnat ip to 198.51.100.20:443 comment \"flvx forward:42 dnat udp\"",
		"masquerade comment \"flvx masquerade\"",
	}
	for _, part := range expectedParts {
		if !strings.Contains(script, part) {
			t.Fatalf("script missing %q:\n%s", part, script)
		}
	}
}

func TestRenderTableBracketsIPv6Target(t *testing.T) {
	script := RenderTable(NodePlan{
		NodeID: 10,
		Rules: []Rule{
			{ForwardID: 42, InPort: 24000, TargetHost: "2001:db8::1", TargetPort: 443, Protocols: []string{"tcp"}},
		},
	})
	if !strings.Contains(script, "dnat ip6 to [2001:db8::1]:443") {
		t.Fatalf("expected bracketed IPv6 dnat target, got:\n%s", script)
	}
}

func TestRenderTableIncludesForwardAccountingCounters(t *testing.T) {
	plan := NodePlan{
		NodeID: 7,
		Rules: []Rule{{
			ForwardID:  42,
			InPort:     12345,
			TargetHost: "198.51.100.20",
			TargetPort: 443,
			Protocols:  []string{"tcp", "udp"},
		}},
	}

	got := RenderTable(plan)
	wantLines := []string{
		`tcp dport 12345 counter dnat ip to 198.51.100.20:443 comment "flvx forward:42 dnat tcp"`,
		`udp dport 12345 counter dnat ip to 198.51.100.20:443 comment "flvx forward:42 dnat udp"`,
		`ct original proto-dst 12345 ip daddr 198.51.100.20 tcp dport 443 counter comment "flvx forward:42 to-target tcp"`,
		`ct original proto-dst 12345 ip saddr 198.51.100.20 tcp sport 443 counter comment "flvx forward:42 from-target tcp"`,
		`ct original proto-dst 12345 ip daddr 198.51.100.20 udp dport 443 counter comment "flvx forward:42 to-target udp"`,
		`ct original proto-dst 12345 ip saddr 198.51.100.20 udp sport 443 counter comment "flvx forward:42 from-target udp"`,
	}
	for _, want := range wantLines {
		if !strings.Contains(got, want) {
			t.Fatalf("RenderTable() missing %q\n%s", want, got)
		}
	}
}

func TestRenderTableIncludesIPv6ForwardAccountingCounters(t *testing.T) {
	plan := NodePlan{
		NodeID: 7,
		Rules: []Rule{{
			ForwardID:  43,
			InPort:     12346,
			TargetHost: "2001:db8::20",
			TargetPort: 8443,
			Protocols:  []string{"tcp"},
		}},
	}

	got := RenderTable(plan)
	wantLines := []string{
		`tcp dport 12346 counter dnat ip6 to [2001:db8::20]:8443 comment "flvx forward:43 dnat tcp"`,
		`ct original proto-dst 12346 ip6 daddr 2001:db8::20 tcp dport 8443 counter comment "flvx forward:43 to-target tcp"`,
		`ct original proto-dst 12346 ip6 saddr 2001:db8::20 tcp sport 8443 counter comment "flvx forward:43 from-target tcp"`,
	}
	for _, want := range wantLines {
		if !strings.Contains(got, want) {
			t.Fatalf("RenderTable() missing %q\n%s", want, got)
		}
	}
}

func TestRenderTableAccountingCountersIncludeOriginalPort(t *testing.T) {
	plan := NodePlan{
		NodeID: 7,
		Rules: []Rule{
			{ForwardID: 42, InPort: 12345, TargetHost: "198.51.100.20", TargetPort: 443, Protocols: []string{"tcp"}},
			{ForwardID: 43, InPort: 12346, TargetHost: "198.51.100.20", TargetPort: 443, Protocols: []string{"tcp"}},
		},
	}

	got := RenderTable(plan)
	wantLines := []string{
		`ct original proto-dst 12345 ip daddr 198.51.100.20 tcp dport 443 counter comment "flvx forward:42 to-target tcp"`,
		`ct original proto-dst 12346 ip daddr 198.51.100.20 tcp dport 443 counter comment "flvx forward:43 to-target tcp"`,
	}
	for _, want := range wantLines {
		if !strings.Contains(got, want) {
			t.Fatalf("RenderTable() missing %q\n%s", want, got)
		}
	}
}

func TestRenderTablePreservesHostnameDNATAndSkipsAccountingCounters(t *testing.T) {
	plan := NodePlan{
		NodeID: 7,
		Rules: []Rule{{
			ForwardID:  44,
			InPort:     12347,
			TargetHost: "example.com",
			TargetPort: 9443,
			Protocols:  []string{"tcp"},
		}},
	}

	got := RenderTable(plan)
	want := `tcp dport 12347 counter dnat to example.com:9443 comment "flvx forward:44 dnat tcp"`
	if !strings.Contains(got, want) {
		t.Fatalf("RenderTable() missing %q\n%s", want, got)
	}
	unwantedLines := []string{
		`dnat ip to example.com`,
		`ip daddr example.com`,
		`ip saddr example.com`,
	}
	for _, unwanted := range unwantedLines {
		if strings.Contains(got, unwanted) {
			t.Fatalf("RenderTable() unexpectedly contains %q\n%s", unwanted, got)
		}
	}
}

func TestRuleHashIsStable(t *testing.T) {
	rule := Rule{ForwardID: 42, InPort: 24000, TargetHost: "198.51.100.20", TargetPort: 443, Protocols: []string{"tcp", "udp"}}
	if RuleHash(rule) != RuleHash(rule) {
		t.Fatalf("expected stable rule hash")
	}
	if RuleHash(rule) == RuleHash(Rule{ForwardID: 42, InPort: 24001, TargetHost: "198.51.100.20", TargetPort: 443, Protocols: []string{"tcp", "udp"}}) {
		t.Fatalf("expected hash to change when port changes")
	}
}

func TestRuleHashIgnoresBindIPWhenRenderingDoesNotUseIt(t *testing.T) {
	base := Rule{
		ForwardID:  42,
		InPort:     24000,
		TargetHost: "198.51.100.20",
		TargetPort: 443,
		Protocols:  []string{"tcp", "udp"},
	}
	withBind := base
	withBind.BindIP = "192.0.2.10"

	if RuleHash(base) != RuleHash(withBind) {
		t.Fatalf("expected bind IP to be ignored by hash when it is not rendered")
	}
}
