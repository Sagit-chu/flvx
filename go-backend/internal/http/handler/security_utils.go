package handler

import (
	"fmt"
	"net"
	"strings"
)

// DisableSafeRemoteAddrCheckForTesting allows bypassing the safety check during integration tests.
var DisableSafeRemoteAddrCheckForTesting = false

// IsSafeRemoteAddr checks if a given address is safe to connect to (prevents SSRF/Open Proxy).
// It resolves domains to IPs to prevent DNS rebinding attacks pointing to internal networks.
// Supports multiple addresses separated by commas or newlines (one per line).
func IsSafeRemoteAddr(addr string) error {
	if DisableSafeRemoteAddrCheckForTesting {
		return nil
	}

	for _, part := range splitRemoteParts(addr) {
		if err := checkSingleRemoteAddr(part); err != nil {
			return err
		}
	}
	return nil
}

// splitRemoteParts splits a multi-address string by commas and newlines.
func splitRemoteParts(addr string) []string {
	addr = strings.ReplaceAll(addr, "\n", ",")
	addr = strings.ReplaceAll(addr, "\r", ",")
	parts := strings.Split(addr, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

// checkSingleRemoteAddr validates a single address.
func checkSingleRemoteAddr(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		if strings.Contains(err.Error(), "missing port in address") {
			host = addr
		} else {
			return fmt.Errorf("invalid address format: %v", err)
		}
	}

	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("could not resolve address %q: %v", addr, err)
	}

	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsPrivate() {
			return fmt.Errorf("address %q resolves to internal IP: %s", addr, ip.String())
		}
	}

	return nil
}

// IsValidNodeAddress ensures the address is strictly a host or host:port.
// It explicitly denies schemes (http://, https://), paths (/...), and query params (?).
func IsValidNodeAddress(addr string) error {
	addr = strings.TrimSpace(addr)
	if strings.Contains(addr, "://") {
		return fmt.Errorf("address must not contain scheme (e.g. http://)")
	}
	if strings.ContainsAny(addr, "/?") {
		return fmt.Errorf("address must not contain path or query parameters")
	}

	_, _, err := net.SplitHostPort(addr)
	if err != nil {
		if !strings.Contains(err.Error(), "missing port in address") {
			return fmt.Errorf("invalid address format")
		}
	}
	return nil
}
