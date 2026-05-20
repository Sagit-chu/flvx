package handler

import (
	"net"
	"net/http"
	"strings"
	"time"
)

type rateLimitBucket struct {
	Count     int
	ResetTime time.Time
}

func (h *Handler) allowRateLimitedRequest(key string, limit int, window time.Duration) bool {
	if h == nil || limit <= 0 || window <= 0 {
		return true
	}
	now := time.Now()
	h.rateLimitMu.Lock()
	defer h.rateLimitMu.Unlock()
	if h.rateLimits == nil {
		h.rateLimits = make(map[string]rateLimitBucket)
	}
	for itemKey, bucket := range h.rateLimits {
		if now.After(bucket.ResetTime) {
			delete(h.rateLimits, itemKey)
		}
	}
	bucket := h.rateLimits[key]
	if bucket.ResetTime.IsZero() || now.After(bucket.ResetTime) {
		h.rateLimits[key] = rateLimitBucket{Count: 1, ResetTime: now.Add(window)}
		return true
	}
	if bucket.Count >= limit {
		return false
	}
	bucket.Count++
	h.rateLimits[key] = bucket
	return true
}

func clientIPFromRequest(r *http.Request) string {
	if r == nil {
		return "unknown"
	}
	for _, header := range []string{"CF-Connecting-IP", "X-Real-IP", "X-Forwarded-For"} {
		value := strings.TrimSpace(r.Header.Get(header))
		if value == "" {
			continue
		}
		if header == "X-Forwarded-For" {
			value = strings.TrimSpace(strings.Split(value, ",")[0])
		}
		if ip := net.ParseIP(value); ip != nil {
			return ip.String()
		}
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil {
		if ip := net.ParseIP(host); ip != nil {
			return ip.String()
		}
	}
	if ip := net.ParseIP(strings.TrimSpace(r.RemoteAddr)); ip != nil {
		return ip.String()
	}
	return "unknown"
}
