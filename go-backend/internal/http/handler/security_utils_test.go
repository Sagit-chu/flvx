package handler

import "testing"

func TestIssue515IsValidNodeAddressAcceptsBareIPv6(t *testing.T) {
	for _, addr := range []string{
		"2001:db8::1",
		"::1",
		"fe80::1%eth0",
	} {
		t.Run(addr, func(t *testing.T) {
			if err := IsValidNodeAddress(addr); err != nil {
				t.Fatalf("expected bare IPv6 address %q to be accepted: %v", addr, err)
			}
		})
	}
}

func TestIsValidNodeAddressKeepsExistingAddressForms(t *testing.T) {
	for _, addr := range []string{
		"203.0.113.10",
		"node.example.com",
		"node.example.com:6365",
		"[2001:db8::1]:6365",
	} {
		t.Run(addr, func(t *testing.T) {
			if err := IsValidNodeAddress(addr); err != nil {
				t.Fatalf("expected node address %q to be accepted: %v", addr, err)
			}
		})
	}
}

func TestIsValidNodeAddressRejectsURLComponents(t *testing.T) {
	for _, addr := range []string{
		"https://node.example.com",
		"node.example.com/path",
		"node.example.com?transport=tcp",
	} {
		t.Run(addr, func(t *testing.T) {
			if err := IsValidNodeAddress(addr); err == nil {
				t.Fatalf("expected node address %q to be rejected", addr)
			}
		})
	}
}
