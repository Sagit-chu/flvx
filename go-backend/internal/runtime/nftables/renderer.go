package nftables

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"sort"
	"strings"
)

func RenderTable(plan NodePlan) string {
	var b strings.Builder
	b.WriteString("table inet flvx {\n")
	b.WriteString("  chain prerouting {\n")
	b.WriteString("    type nat hook prerouting priority dstnat; policy accept;\n")
	for _, rule := range sortedRules(plan.Rules) {
		family := nftAddressFamily(rule.TargetHost)
		dnatFamily := ""
		if family != "" {
			dnatFamily = family + " "
		}
		for _, protocol := range normalizedProtocols(rule.Protocols) {
			b.WriteString(fmt.Sprintf("    %s dport %d counter dnat %sto %s comment %q\n",
				protocol,
				rule.InPort,
				dnatFamily,
				formatDNATTarget(rule.TargetHost, rule.TargetPort),
				counterComment(rule.ForwardID, CounterDirectionDNAT, protocol),
			))
		}
	}
	b.WriteString("  }\n\n")
	b.WriteString("  chain postrouting {\n")
	b.WriteString("    type nat hook postrouting priority srcnat; policy accept;\n")
	if len(plan.Rules) > 0 {
		b.WriteString("    masquerade comment \"flvx masquerade\"\n")
	}
	b.WriteString("  }\n\n")
	b.WriteString("  chain forward {\n")
	b.WriteString("    type filter hook forward priority filter; policy accept;\n")
	for _, rule := range sortedRules(plan.Rules) {
		family := nftAddressFamily(rule.TargetHost)
		if family == "" {
			continue
		}
		targetHost := strings.Trim(strings.TrimSpace(rule.TargetHost), "[]")
		for _, protocol := range normalizedProtocols(rule.Protocols) {
			b.WriteString(fmt.Sprintf("    %s daddr %s %s dport %d counter comment %q\n",
				family,
				targetHost,
				protocol,
				rule.TargetPort,
				counterComment(rule.ForwardID, CounterDirectionToTarget, protocol),
			))
			b.WriteString(fmt.Sprintf("    %s saddr %s %s sport %d counter comment %q\n",
				family,
				targetHost,
				protocol,
				rule.TargetPort,
				counterComment(rule.ForwardID, CounterDirectionFromTarget, protocol),
			))
		}
	}
	b.WriteString("  }\n")
	b.WriteString("}\n")
	return b.String()
}

func counterComment(forwardID int64, direction, protocol string) string {
	return fmt.Sprintf("flvx forward:%d %s %s", forwardID, direction, protocol)
}

func RuleHash(rule Rule) string {
	protocols := normalizedProtocols(rule.Protocols)
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d|%d|%s|%d|%s",
		rule.ForwardID,
		rule.InPort,
		strings.TrimSpace(rule.TargetHost),
		rule.TargetPort,
		strings.Join(protocols, ","),
	)))
	return hex.EncodeToString(sum[:])
}

func PlanHashes(plan NodePlan) map[int64]string {
	hashes := make(map[int64]string, len(plan.Rules))
	for _, rule := range plan.Rules {
		hashes[rule.ForwardID] = RuleHash(rule)
	}
	return hashes
}

func sortedRules(rules []Rule) []Rule {
	out := append([]Rule(nil), rules...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].InPort == out[j].InPort {
			return out[i].ForwardID < out[j].ForwardID
		}
		return out[i].InPort < out[j].InPort
	})
	return out
}

func normalizedProtocols(protocols []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, 2)
	for _, protocol := range protocols {
		p := strings.ToLower(strings.TrimSpace(protocol))
		if p != "tcp" && p != "udp" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	if len(out) == 0 {
		return []string{"tcp", "udp"}
	}
	sort.Strings(out)
	return out
}

func formatDNATTarget(host string, port int) string {
	trimmed := strings.Trim(strings.TrimSpace(host), "[]")
	if ip := net.ParseIP(trimmed); ip != nil && ip.To4() == nil {
		return fmt.Sprintf("[%s]:%d", trimmed, port)
	}
	return fmt.Sprintf("%s:%d", trimmed, port)
}

func nftAddressFamily(host string) string {
	trimmed := strings.Trim(strings.TrimSpace(host), "[]")
	ip := net.ParseIP(trimmed)
	if ip == nil {
		return ""
	}
	if ip.To4() == nil {
		return "ip6"
	}
	return "ip"
}
