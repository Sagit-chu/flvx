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
func IsSafeRemoteAddr(addr string) error {
	if DisableSafeRemoteAddrCheckForTesting {
		return nil
	}

	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		// If there is no port, try to treat the whole string as host
		if strings.Contains(err.Error(), "missing port in address") {
			host = addr
		} else {
			return fmt.Errorf("invalid address format: %v", err)
		}
	}

	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("could not resolve address: %v", err)
	}

	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
			return fmt.Errorf("address resolves to internal or reserved IP: %s", ip.String())
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
	
	// Try parsing as host:port
	_, _, err := net.SplitHostPort(addr)
	if err != nil {
		if strings.Contains(err.Error(), "missing port in address") {
			// It's just a host, which is fine
		} else {
			return fmt.Errorf("invalid address format")
		}
	}
	return nil
}
