package middleware

import (
	"net/http"
	"strings"

	"go-backend/internal/http/response"
)

type CommercialLicenseChecker func() (bool, string)

func CommercialLicense(checker CommercialLicenseChecker) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if checker == nil || !requiresCommercialLicense(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
			ok, reason := checker()
			if ok {
				next.ServeHTTP(w, r)
				return
			}
			if strings.TrimSpace(reason) == "" {
				reason = "商业授权未激活"
			}
			response.WriteJSON(w, response.Err(451, reason))
		})
	}
}

func requiresCommercialLicense(path string) bool {
	if !strings.HasPrefix(path, "/api/v1/") {
		return false
	}
	if commercialLicenseAllowPath(path) {
		return false
	}
	switch {
	case strings.HasPrefix(path, "/api/v1/node/"):
		return true
	case strings.HasPrefix(path, "/api/v1/tunnel/"):
		return true
	case strings.HasPrefix(path, "/api/v1/forward/"):
		return true
	case strings.HasPrefix(path, "/api/v1/speed-limit/"):
		return true
	case strings.HasPrefix(path, "/api/v1/group/"):
		return true
	case strings.HasPrefix(path, "/api/v1/admin/commerce/"):
		return true
	case strings.HasPrefix(path, "/api/v1/commerce/order/"):
		return true
	case strings.HasPrefix(path, "/api/v1/commerce/subscription/"):
		return true
	case strings.HasPrefix(path, "/api/v1/commerce/wallet/"):
		return true
	case strings.HasPrefix(path, "/api/v1/commerce/ticket/"):
		return true
	case strings.HasPrefix(path, "/api/v1/commerce/notification/"):
		return true
	case strings.HasPrefix(path, "/api/v1/monitor/"):
		return true
	case strings.HasPrefix(path, "/api/v1/federation/"):
		return true
	case strings.HasPrefix(path, "/api/v1/backup/"):
		return true
	case strings.HasPrefix(path, "/api/v1/api/v1/backup/"):
		return true
	default:
		return false
	}
}

func commercialLicenseAllowPath(path string) bool {
	switch {
	case path == "/api/v1/user/login":
		return true
	case path == "/api/v1/user/updatePassword":
		return true
	case strings.HasPrefix(path, "/api/v1/license/local/"):
		return true
	case strings.HasPrefix(path, "/api/v1/captcha/"):
		return true
	case path == "/api/v1/public/config/get":
		return true
	case path == "/api/v1/config/get":
		return true
	case path == "/api/v1/config/list":
		return true
	case path == "/api/v1/system/version":
		return true
	case path == "/api/v1/system/storage":
		return true
	case path == "/api/v1/commerce/public/settings":
		return true
	case path == "/api/v1/commerce/plans/public":
		return true
	case path == "/api/v1/commerce/legal":
		return true
	case path == "/api/v1/payment/epay/notify":
		return true
	case path == "/api/v1/payment/usdt/notify":
		return true
	default:
		return false
	}
}
