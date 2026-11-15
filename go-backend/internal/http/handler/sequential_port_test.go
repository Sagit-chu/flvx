package handler

import "testing"

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

func TestSequentialPortConfigReadsRepositoryAndDefaultsOff(t *testing.T) {
	_, r := setupConfigAccessTestRouter(t)
	h := &Handler{repo: r}
	if h.isSequentialPortEnabled() {
		t.Fatal("missing config must preserve random allocation")
	}
	for _, tc := range []struct {
		value string
		want  bool
	}{
		{"true", true}, {" TRUE ", true}, {"false", false}, {"invalid", false},
	} {
		seedConfigValue(t, r, sequentialPortConfigKey, tc.value)
		if got := h.isSequentialPortEnabled(); got != tc.want {
			t.Fatalf("config %q: got %v, want %v", tc.value, got, tc.want)
		}
	}
}
