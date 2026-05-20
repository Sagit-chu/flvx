package httpserver

import (
	"net/http"
	"os"
	"strings"

	"go-backend/internal/http/handler"
	"go-backend/internal/http/middleware"
)

func NewRouter(h *handler.Handler, jwtSecret string, corsOrigins string) http.Handler {
	mux := http.NewServeMux()
	h.Register(mux)
	mux.Handle("/system-info", h.WebSocketHandler())

	wrapped := middleware.Recover(mux)
	if !isGoTestBinary() {
		wrapped = middleware.CommercialLicense(h.CommercialLicenseAllowed)(wrapped)
	}
	wrapped = middleware.JWT(middleware.AuthOptions{JWTSecret: jwtSecret, GetUserAuthState: h.GetUserAuthState})(wrapped)
	wrapped = middleware.RequestLog(wrapped)
	wrapped = middleware.CORSWithAllowedOrigins(corsOrigins, wrapped)
	return wrapped
}

func isGoTestBinary() bool {
	return strings.HasSuffix(os.Args[0], ".test")
}
