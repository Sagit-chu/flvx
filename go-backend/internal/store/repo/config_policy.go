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
	"is_commercial":       {},
	"hide_footer_brand":   {},
}

var sensitiveConfigKeys = map[string]struct{}{
	"jwt_secret":            {},
	"license_key":           {},
	"license_expiry":        {},
	"license_machine_id":    {},
	"machine_fingerprint":   {},
	"cloudflare_secret_key": {},
}

var systemManagedConfigKeys = map[string]struct{}{
	"license_key":         {},
	"license_expiry":      {},
	"license_machine_id":  {},
	"is_commercial":       {},
	"machine_fingerprint": {},
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

func IsSystemManagedConfigKey(name string) bool {
	_, ok := systemManagedConfigKeys[normalizeConfigKey(name)]
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

func FilterBackupConfigs(in map[string]string) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for name, value := range in {
		if IsSensitiveConfigKey(name) || IsSystemManagedConfigKey(name) {
			continue
		}
		out[name] = value
	}
	return out
}

func normalizeConfigKey(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
