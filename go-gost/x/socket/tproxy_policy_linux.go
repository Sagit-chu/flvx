//go:build linux

package socket

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
)

const (
	defaultTPROXYMark  = 11
	defaultTPROXYTable = 111
	tproxyChainName    = "FLVX_TPROXY"
	tproxyStateFile    = "/etc/flux_agent/tproxy_policy_state.json"
)

type tproxyPolicyRequest struct {
	Port         int   `json:"port"`
	Mark         int   `json:"mark"`
	Table        int   `json:"table"`
	ExcludePorts []int `json:"excludePorts,omitempty"`
}

type tproxyPolicyState struct {
	Entries []tproxyPolicyRequest `json:"entries"`
}

var tproxyStateMu sync.Mutex

func (r *tproxyPolicyRequest) normalize() error {
	if r == nil {
		return errors.New("策略路由参数不能为空")
	}
	if r.Port <= 0 || r.Port > 65535 {
		return fmt.Errorf("invalid port: %d", r.Port)
	}
	if r.Mark <= 0 {
		r.Mark = defaultTPROXYMark
	}
	if r.Table <= 0 {
		r.Table = defaultTPROXYTable
	}
	if len(r.ExcludePorts) > 0 {
		set := make(map[int]struct{}, len(r.ExcludePorts))
		ports := make([]int, 0, len(r.ExcludePorts))
		for _, p := range r.ExcludePorts {
			if p <= 0 || p > 65535 {
				continue
			}
			if _, ok := set[p]; ok {
				continue
			}
			set[p] = struct{}{}
			ports = append(ports, p)
		}
		sort.Ints(ports)
		r.ExcludePorts = ports
	}
	return nil
}

func probeTProxyCapability() (map[string]interface{}, error) {
	hasIPTables := commandExists("iptables")
	hasIP6Tables := commandExists("ip6tables")
	hasIP := commandExists("ip")

	capability := map[string]interface{}{
		"supported":    hasIP && hasIPTables,
		"platform":     "linux",
		"hasIP":        hasIP,
		"hasIPTables":  hasIPTables,
		"hasIP6Tables": hasIP6Tables,
	}

	state, _ := loadTProxyState()
	capability["stateEntries"] = len(state.Entries)

	if hasIP && hasIPTables {
		return capability, nil
	}

	return capability, errors.New("tproxy prerequisites missing: require ip and iptables")
}

func ensureTProxyPolicy(req tproxyPolicyRequest) error {
	if _, err := probeTProxyCapability(); err != nil {
		return err
	}

	mark := strconv.Itoa(req.Mark)
	table := strconv.Itoa(req.Table)
	port := strconv.Itoa(req.Port)

	// IPv4 chain and jump rule
	_ = runCmd("iptables", "-t", "mangle", "-N", tproxyChainName)
	if err := ensureRule("iptables", "-t", "mangle", "PREROUTING", []string{"-j", tproxyChainName}); err != nil {
		return fmt.Errorf("iptables ensure prerouting jump: %w", err)
	}
	if err := ensureRule("iptables", "-t", "mangle", tproxyChainName, []string{"-m", "mark", "--mark", mark, "-j", "RETURN"}); err != nil {
		return fmt.Errorf("iptables ensure mark bypass rule: %w", err)
	}
	if err := ensureRule("iptables", "-t", "mangle", tproxyChainName, []string{"-d", "127.0.0.0/8", "-j", "RETURN"}); err != nil {
		return fmt.Errorf("iptables ensure loopback bypass rule: %w", err)
	}
	if err := ensureRule("iptables", "-t", "mangle", tproxyChainName, []string{"-d", "169.254.0.0/16", "-j", "RETURN"}); err != nil {
		return fmt.Errorf("iptables ensure link-local bypass rule: %w", err)
	}
	for _, p := range req.ExcludePorts {
		if err := ensureRule("iptables", "-t", "mangle", tproxyChainName, []string{"-p", "udp", "--dport", strconv.Itoa(p), "-j", "RETURN"}); err != nil {
			return fmt.Errorf("iptables ensure exclude port %d: %w", p, err)
		}
	}
	if err := ensureRule("iptables", "-t", "mangle", tproxyChainName, []string{"-p", "udp", "--dport", port, "-j", "TPROXY", "--on-port", port, "--tproxy-mark", mark + "/0xffffffff"}); err != nil {
		return fmt.Errorf("iptables ensure tproxy rule: %w", err)
	}

	// IPv6 best effort
	if commandExists("ip6tables") {
		_ = runCmd("ip6tables", "-t", "mangle", "-N", tproxyChainName)
		_ = ensureRule("ip6tables", "-t", "mangle", "PREROUTING", []string{"-j", tproxyChainName})
		_ = ensureRule("ip6tables", "-t", "mangle", tproxyChainName, []string{"-m", "mark", "--mark", mark, "-j", "RETURN"})
		_ = ensureRule("ip6tables", "-t", "mangle", tproxyChainName, []string{"-d", "::1/128", "-j", "RETURN"})
		_ = ensureRule("ip6tables", "-t", "mangle", tproxyChainName, []string{"-d", "fe80::/10", "-j", "RETURN"})
		for _, p := range req.ExcludePorts {
			_ = ensureRule("ip6tables", "-t", "mangle", tproxyChainName, []string{"-p", "udp", "--dport", strconv.Itoa(p), "-j", "RETURN"})
		}
		_ = ensureRule("ip6tables", "-t", "mangle", tproxyChainName, []string{"-p", "udp", "--dport", port, "-j", "TPROXY", "--on-port", port, "--tproxy-mark", mark + "/0xffffffff"})
	}

	// policy routing
	_ = runCmd("ip", "rule", "add", "fwmark", mark, "lookup", table)
	_ = runCmd("ip", "route", "replace", "local", "0.0.0.0/0", "dev", "lo", "table", table)
	if commandExists("ip6tables") {
		_ = runCmd("ip", "-6", "rule", "add", "fwmark", mark, "lookup", table)
		_ = runCmd("ip", "-6", "route", "replace", "local", "::/0", "dev", "lo", "table", table)
	}

	if err := upsertTProxyState(req); err != nil {
		return fmt.Errorf("保存tproxy状态失败: %w", err)
	}

	return nil
}

func deleteTProxyPolicy(req tproxyPolicyRequest) error {
	if !commandExists("iptables") {
		return errors.New("iptables not found")
	}

	mark := strconv.Itoa(req.Mark)
	table := strconv.Itoa(req.Table)
	port := strconv.Itoa(req.Port)

	_ = runCmd("iptables", "-t", "mangle", "-D", tproxyChainName, "-p", "udp", "--dport", port, "-j", "TPROXY", "--on-port", port, "--tproxy-mark", mark+"/0xffffffff")
	_ = runCmd("ip6tables", "-t", "mangle", "-D", tproxyChainName, "-p", "udp", "--dport", port, "-j", "TPROXY", "--on-port", port, "--tproxy-mark", mark+"/0xffffffff")
	_ = runCmd("ip", "rule", "del", "fwmark", mark, "lookup", table)
	_ = runCmd("ip", "-6", "rule", "del", "fwmark", mark, "lookup", table)

	if err := removeTProxyState(req); err != nil {
		return fmt.Errorf("更新tproxy状态失败: %w", err)
	}

	return nil
}

func restoreTProxyPolicies() error {
	state, err := loadTProxyState()
	if err != nil {
		return err
	}
	for _, req := range state.Entries {
		if err := req.normalize(); err != nil {
			continue
		}
		if err := ensureTProxyPolicy(req); err != nil {
			return err
		}
	}
	return nil
}

func getTProxyPolicyStateSummary() (map[string]interface{}, error) {
	state, err := loadTProxyState()
	if err != nil {
		return nil, err
	}
	entries := make([]map[string]interface{}, 0, len(state.Entries))
	for _, item := range state.Entries {
		entries = append(entries, map[string]interface{}{
			"port":         item.Port,
			"mark":         item.Mark,
			"table":        item.Table,
			"excludePorts": item.ExcludePorts,
		})
	}
	return map[string]interface{}{
		"stateFile": tproxyStateFile,
		"entries":   entries,
	}, nil
}

func ensureRule(bin string, tableArg string, table string, chain string, rule []string) error {
	checkArgs := []string{tableArg, table, "-C", chain}
	checkArgs = append(checkArgs, rule...)
	if err := runCmd(bin, checkArgs...); err == nil {
		return nil
	}
	addArgs := []string{tableArg, table, "-A", chain}
	addArgs = append(addArgs, rule...)
	if err := runCmd(bin, addArgs...); err != nil {
		if !isRuleExistsErr(err) {
			return err
		}
	}
	return nil
}

func upsertTProxyState(req tproxyPolicyRequest) error {
	tproxyStateMu.Lock()
	defer tproxyStateMu.Unlock()

	state, err := loadTProxyStateNoLock()
	if err != nil {
		return err
	}
	updated := false
	for i := range state.Entries {
		if state.Entries[i].Port != req.Port {
			continue
		}
		state.Entries[i] = req
		updated = true
		break
	}
	if !updated {
		state.Entries = append(state.Entries, req)
	}
	sort.Slice(state.Entries, func(i, j int) bool { return state.Entries[i].Port < state.Entries[j].Port })
	return saveTProxyStateNoLock(state)
}

func removeTProxyState(req tproxyPolicyRequest) error {
	tproxyStateMu.Lock()
	defer tproxyStateMu.Unlock()

	state, err := loadTProxyStateNoLock()
	if err != nil {
		return err
	}
	out := make([]tproxyPolicyRequest, 0, len(state.Entries))
	for _, item := range state.Entries {
		if item.Port == req.Port {
			continue
		}
		out = append(out, item)
	}
	state.Entries = out
	return saveTProxyStateNoLock(state)
}

func loadTProxyState() (tproxyPolicyState, error) {
	tproxyStateMu.Lock()
	defer tproxyStateMu.Unlock()
	return loadTProxyStateNoLock()
}

func loadTProxyStateNoLock() (tproxyPolicyState, error) {
	state := tproxyPolicyState{Entries: []tproxyPolicyRequest{}}
	b, err := os.ReadFile(tproxyStateFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return state, nil
		}
		return state, err
	}
	if err := json.Unmarshal(b, &state); err != nil {
		return tproxyPolicyState{Entries: []tproxyPolicyRequest{}}, nil
	}
	if state.Entries == nil {
		state.Entries = []tproxyPolicyRequest{}
	}
	return state, nil
}

func saveTProxyStateNoLock(state tproxyPolicyState) error {
	if state.Entries == nil {
		state.Entries = []tproxyPolicyRequest{}
	}
	if err := os.MkdirAll(filepath.Dir(tproxyStateFile), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(tproxyStateFile, b, 0o644)
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func runCmd(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %s", name, strings.Join(args, " "), strings.TrimSpace(string(out)))
	}
	return nil
}

func isRuleExistsErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "exists") || strings.Contains(msg, "already")
}
