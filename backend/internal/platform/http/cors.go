package http

import (
	"net/http"
	"strconv"
	"strings"
)

// CORSMiddleware allows the browser to call the API cross-origin from the
// configured web origins (WEB_ORIGIN, comma-separated) while carrying
// credentials (refresh cookie + Authorization header). Requests with any other
// Origin get no CORS headers, so the browser blocks them. Preflight OPTIONS
// requests are answered directly without reaching the route handlers.
//
// See docs/architecture/security.md §5: CORS restricted to WEB_ORIGIN.
func CORSMiddleware(allowedOrigins string) func(http.Handler) http.Handler {
	allow := make(map[string]bool)
	for _, o := range strings.Split(allowedOrigins, ",") {
		if o = strings.TrimSpace(o); o != "" {
			allow[o] = true
		}
	}

	const (
		allowMethods = "GET, POST, PUT, PATCH, DELETE, OPTIONS"
		allowHeaders = "Content-Type, Authorization, X-Requested-With"
		maxAge       = 600 // seconds
	)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			allowed := origin != "" && allow[origin]

			if allowed {
				h := w.Header()
				h.Set("Access-Control-Allow-Origin", origin)
				h.Set("Vary", "Origin")
				h.Set("Access-Control-Allow-Credentials", strconv.FormatBool(true))
				h.Set("Access-Control-Allow-Methods", allowMethods)
				h.Set("Access-Control-Allow-Headers", allowHeaders)
				h.Set("Access-Control-Max-Age", strconv.Itoa(maxAge))
			}

			// CORS preflight: answer without invoking the route chain.
			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
