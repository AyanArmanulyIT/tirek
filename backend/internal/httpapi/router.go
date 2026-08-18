// Package httpapi wires the modular-monolith HTTP server: chi router, shared
// middleware, and the module handlers.
package httpapi

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"tirek/backend/internal/identity"
	"tirek/backend/internal/organizations"
	"tirek/backend/internal/platform/config"
	platformhttp "tirek/backend/internal/platform/http"
)

// Server holds the long-lived dependencies for the HTTP API.
type Server struct {
	cfg      config.Config
	pool     *pgxpool.Pool
	log      zerolog.Logger
	identity *identity.Service
	handler  http.Handler
}

// New builds the API server and its handler.
func New(cfg config.Config, pool *pgxpool.Pool, log zerolog.Logger) *Server {
	orgs := organizations.NewService(pool, log)
	tokens := identity.TokenManager{
		AccessSecret:  []byte(cfg.Auth.AccessSecret),
		RefreshSecret: []byte(cfg.Auth.RefreshSecret),
		AccessTTL:     cfg.Auth.AccessTTL,
		RefreshTTL:    cfg.Auth.RefreshTTL,
	}
	svc := identity.NewService(pool, tokens, orgs, log)
	identityHandler := identity.NewHandler(svc, cfg.Auth, cfg.Auth.CookieName, log)

	s := &Server{cfg: cfg, pool: pool, log: log, identity: svc}
	s.handler = s.buildRouter(identityHandler)
	return s
}

// Handler returns the fully-wired http.Handler.
func (s *Server) Handler() http.Handler { return s.handler }

// Identity returns the identity service (used by tests).
func (s *Server) Identity() *identity.Service { return s.identity }

func (s *Server) buildRouter(ih *identity.Handler) http.Handler {
	r := chi.NewRouter()
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(platformhttp.CORSMiddleware(s.cfg.API.WebOrigin))
	r.Use(chimiddleware.Recoverer)
	r.Use(s.loggerMiddleware)

	r.Get("/v1/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	r.Get("/v1/ready", func(w http.ResponseWriter, req *http.Request) {
		if err := s.pool.Ping(req.Context()); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready"))
	})

	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/auth/register", ih.Register)
		r.Post("/auth/login", ih.Login)
		r.Post("/auth/refresh", ih.Refresh)
		r.Post("/auth/logout", ih.Logout)

		r.Group(func(r chi.Router) {
			r.Use(s.identity.Authenticate)
			r.Get("/me", ih.Me)
		})
	})

	return r
}

// loggerMiddleware emits one structured log line per request with the
// correlation request_id (see docs/architecture/api.md §9).
func (s *Server) loggerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := chimiddleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		s.log.Info().
			Str("request_id", chimiddleware.GetReqID(r.Context())).
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Int("status", ww.Status()).
			Dur("duration", time.Since(start)).
			Str("remote", r.RemoteAddr).
			Msg("http_request")
	})
}