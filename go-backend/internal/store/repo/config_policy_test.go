package repo

import "testing"

func TestConfigPolicy(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want ConfigAccessPolicy
	}{
		{name: "app_name is public", key: "app_name", want: ConfigAccessPublic},
		{name: "app_logo is public", key: "app_logo", want: ConfigAccessPublic},
		{name: "app_favicon is public", key: "app_favicon", want: ConfigAccessPublic},
		{name: "app_bg_image is public", key: "app_bg_image", want: ConfigAccessPublic},
		{name: "cloudflare_site_key is public", key: "cloudflare_site_key", want: ConfigAccessPublic},
		{name: "is_commercial is public", key: "is_commercial", want: ConfigAccessPublic},
		{name: "hide_footer_brand is public", key: "hide_footer_brand", want: ConfigAccessPublic},
		{name: "jwt_secret is sensitive", key: "jwt_secret", want: ConfigAccessSensitive},
		{name: "license_key is sensitive", key: "license_key", want: ConfigAccessSensitive},
		{name: "cloudflare_secret_key is sensitive", key: "cloudflare_secret_key", want: ConfigAccessSensitive},
		{name: "trimmed public key is public", key: "  APP_NAME  ", want: ConfigAccessPublic},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PolicyForConfig(tt.key); got != tt.want {
				t.Fatalf("PolicyForConfig(%q) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}

func TestConfigPolicyHelpers(t *testing.T) {
	publicKeys := []string{"app_name", "app_logo", "app_favicon", "app_bg_image", "cloudflare_site_key", "is_commercial", "hide_footer_brand"}
	for _, key := range publicKeys {
		if !IsPublicConfigKey(key) {
			t.Fatalf("expected %q to be public", key)
		}
	}

	sensitiveKeys := []string{"jwt_secret", "license_key", "license_expiry", "license_machine_id", "machine_fingerprint", "cloudflare_secret_key"}
	for _, key := range sensitiveKeys {
		if !IsSensitiveConfigKey(key) {
			t.Fatalf("expected %q to be sensitive", key)
		}
	}

	input := map[string]string{
		"app_name":              "FLVX",
		"license_key":           "secret-license",
		"cloudflare_secret_key": "secret-cloudflare",
		"jwt_secret":            "secret-jwt",
		"cloudflare_site_key":   "site-key",
	}
	filtered := FilterSensitiveConfigs(input)
	if len(filtered) != 2 {
		t.Fatalf("expected 2 public configs, got %d", len(filtered))
	}
	if filtered["app_name"] != "FLVX" || filtered["cloudflare_site_key"] != "site-key" {
		t.Fatalf("unexpected filtered configs: %+v", filtered)
	}
	if _, ok := filtered["jwt_secret"]; ok {
		t.Fatal("expected jwt_secret to be filtered out")
	}
	if _, ok := filtered["license_key"]; ok {
		t.Fatal("expected license_key to be filtered out")
	}
	if _, ok := filtered["cloudflare_secret_key"]; ok {
		t.Fatal("expected cloudflare_secret_key to be filtered out")
	}
}

func TestFilterBackupConfigsOmitsLicenseState(t *testing.T) {
	filtered := FilterBackupConfigs(map[string]string{
		"app_name":            "Brand",
		"license_key":         "secret-license",
		"license_expiry":      "never",
		"license_machine_id":  "machine-id",
		"is_commercial":       "true",
		"machine_fingerprint": "fingerprint",
	})
	if len(filtered) != 1 || filtered["app_name"] != "Brand" {
		t.Fatalf("unexpected backup configs: %+v", filtered)
	}
}

func TestSystemManagedConfigKeys(t *testing.T) {
	for _, key := range []string{"license_key", "license_expiry", "license_machine_id", "is_commercial", "machine_fingerprint"} {
		if !IsSystemManagedConfigKey(key) {
			t.Fatalf("expected %s to be system managed", key)
		}
	}
	for _, key := range []string{"jwt_secret", "cloudflare_secret_key", "app_name"} {
		if IsSystemManagedConfigKey(key) {
			t.Fatalf("did not expect %s to be system managed", key)
		}
	}
}
