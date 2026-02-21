package handler

import "testing"

func TestReleaseChannelFromTag(t *testing.T) {
	tests := []struct {
		name    string
		tag     string
		expects string
	}{
		{name: "stable semantic version", tag: "2.1.4", expects: releaseChannelStable},
		{name: "v prefix should be dev", tag: "v2.1.4", expects: releaseChannelDev},
		{name: "rc release", tag: "2.1.4-rc2", expects: releaseChannelDev},
		{name: "beta release", tag: "2.1.4-beta.1", expects: releaseChannelDev},
		{name: "alpha release", tag: "2.1.4-alpha", expects: releaseChannelDev},
		{name: "non numeric tag", tag: "nightly", expects: releaseChannelDev},
		{name: "empty tag", tag: "", expects: releaseChannelDev},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := releaseChannelFromTag(tc.tag); got != tc.expects {
				t.Fatalf("releaseChannelFromTag(%q) = %q, want %q", tc.tag, got, tc.expects)
			}
		})
	}
}

func TestNormalizeReleaseChannel(t *testing.T) {
	tests := []struct {
		input   string
		expects string
	}{
		{input: "", expects: releaseChannelStable},
		{input: "stable", expects: releaseChannelStable},
		{input: "dev", expects: releaseChannelDev},
		{input: "DEV", expects: releaseChannelDev},
		{input: "preview", expects: releaseChannelStable},
	}

	for _, tc := range tests {
		if got := normalizeReleaseChannel(tc.input); got != tc.expects {
			t.Fatalf("normalizeReleaseChannel(%q) = %q, want %q", tc.input, got, tc.expects)
		}
	}
}
