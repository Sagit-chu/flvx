package handler

import "testing"

func TestLowestPortFillsGapsInOrder(t *testing.T) {
	nodePorts, err := parsePorts("100-1000")
	if err != nil {
		t.Fatalf("parsePorts: %v", err)
	}

	used := map[int]bool{100: true, 101: true, 103: true}
	pick := func() int {
		var available []int
		for _, p := range nodePorts {
			if !used[p] {
				available = append(available, p)
			}
		}
		port := lowestPort(available)
		used[port] = true
		return port
	}

	for _, want := range []int{102, 104, 105} {
		if got := pick(); got != want {
			t.Fatalf("expected port %d, got %d", want, got)
		}
	}
}

func TestLowestPortIgnoresRangeOrderAndEmpty(t *testing.T) {
	if got := lowestPort(nil); got != 0 {
		t.Fatalf("expected 0 for empty input, got %d", got)
	}
	if got := lowestPort([]int{300, 120, 200}); got != 120 {
		t.Fatalf("expected 120, got %d", got)
	}
}

func TestNormalizeSequentialPortConfigValue(t *testing.T) {
	for in, want := range map[string]string{"true": "true", " FALSE ": "false"} {
		got, err := normalizeAndValidateConfigValue(sequentialPortConfigKey, in)
		if err != nil || got != want {
			t.Fatalf("value %q: got %q err=%v, want %q", in, got, err, want)
		}
	}
	if _, err := normalizeAndValidateConfigValue(sequentialPortConfigKey, "maybe"); err == nil {
		t.Fatal("expected invalid value to be rejected")
	}
}
