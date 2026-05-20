package middleware

import (
	"net/http"
	"strings"
)

func CORS(next http.Handler) http.Handler {
	return CORSWithAllowedOrigins("", next)
}

func CORSWithAllowedOrigins(allowedOrigins string, next http.Handler) http.Handler {
	allowed := parseAllowedOrigins(allowedOrigins)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		if len(allowed) == 0 {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		} else if _, ok := allowed[origin]; ok {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Add("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Headers", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, PUT, OPTIONS")
		w.Header().Set("Access-Control-Expose-Headers", "Authorization")
		if r.Method == http.MethodOptions {
			if len(allowed) > 0 && origin != "" {
				if _, ok := allowed[origin]; !ok {
					w.WriteHeader(http.StatusForbidden)
					return
				}
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func parseAllowedOrigins(raw string) map[string]struct{} {
	origins := map[string]struct{}{}
	for _, item := range strings.Split(raw, ",") {
		origin := strings.TrimSpace(item)
		if origin == "" || origin == "*" {
			continue
		}
		origins[origin] = struct{}{}
	}
	return origins
}
