package repo

import "strings"

type ConfigAccessPolicy string

const (
	ConfigAccessPublic    ConfigAccessPolicy = "public"
	ConfigAccessSensitive ConfigAccessPolicy = "sensitive"
)

var publicConfigKeys = map[string]struct{}{
	"app_name":            {},
	"app_logo":            {},
	"app_favicon":         {},
	"app_bg_image":        {},
	"cloudflare_site_key": {},
}

var sensitiveConfigKeys = map[string]struct{}{
	"jwt_secret":            {},
	"license_key":           {},
	"license_certificate":   {},
	"license_instance_id":   {},
	"license_public_key":    {},
	"license_center_url":    {},
	"license_last_error":    {},
	"license_updated_at":    {},
	"cloudflare_secret_key": {},
	"epay_key":              {},
	"usdt_secret_key":       {},
}

func PolicyForConfig(name string) ConfigAccessPolicy {
	if IsPublicConfigKey(name) {
		return ConfigAccessPublic
	}
	if IsSensitiveConfigKey(name) {
		return ConfigAccessSensitive
	}
	return ConfigAccessSensitive
}

func IsPublicConfigKey(name string) bool {
	_, ok := publicConfigKeys[normalizeConfigKey(name)]
	return ok
}

func IsSensitiveConfigKey(name string) bool {
	_, ok := sensitiveConfigKeys[normalizeConfigKey(name)]
	return ok
}

func FilterSensitiveConfigs(in map[string]string) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for name, value := range in {
		if IsSensitiveConfigKey(name) {
			continue
		}
		out[name] = value
	}
	return out
}

func normalizeConfigKey(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
