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

	"tirek/backend/internal/catalog"
	"tirek/backend/internal/identity"
	"tirek/backend/internal/organizations"
	"tirek/backend/internal/platform/config"
	platformhttp "tirek/backend/internal/platform/http"
	"tirek/backend/internal/procurement"
	"tirek/backend/internal/restaurants"
	"tirek/backend/internal/suppliers"
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

	restaurantsSvc := restaurants.NewService(pool, log)
	restaurantsHandler := restaurants.NewHandler(restaurantsSvc, log)
	suppliersSvc := suppliers.NewService(pool, log)
	suppliersHandler := suppliers.NewHandler(suppliersSvc, log)
	catalogSvc := catalog.NewService(pool, log)
	catalogHandler := catalog.NewHandler(catalogSvc, log)
	procurementSvc := procurement.NewService(pool, log)
	procurementHandler := procurement.NewHandler(procurementSvc, log)

	s := &Server{cfg: cfg, pool: pool, log: log, identity: svc}
	s.handler = s.buildRouter(identityHandler, restaurantsHandler, suppliersHandler, catalogHandler, procurementHandler, orgs)
	return s
}

// Handler returns the fully-wired http.Handler.
func (s *Server) Handler() http.Handler { return s.handler }

// Identity returns the identity service (used by tests).
func (s *Server) Identity() *identity.Service { return s.identity }

func (s *Server) buildRouter(ih *identity.Handler, rh *restaurants.Handler, sh *suppliers.Handler, ch *catalog.Handler, ph *procurement.Handler, orgs *organizations.Service) http.Handler {
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

			// Restaurants (buyer organizations)
			r.With(identity.RequireAnyPermission([]string{"restaurants.read", "restaurants.manage"}, orgs)).
				Get("/restaurants", rh.List)
			r.With(identity.RequireAnyPermission([]string{"restaurants.write", "restaurants.manage"}, orgs)).
				Post("/restaurants", rh.Create)
			r.With(identity.RequireAnyPermission([]string{"restaurants.read", "restaurants.manage"}, orgs)).
				Get("/restaurants/{restaurantId}", rh.Get)
			r.With(identity.RequireAnyPermission([]string{"restaurants.write", "restaurants.manage"}, orgs)).
				Patch("/restaurants/{restaurantId}", rh.Update)
			r.With(identity.RequireAnyPermission([]string{"restaurants.read", "restaurants.manage"}, orgs)).
				Get("/restaurants/{restaurantId}/locations", rh.ListLocations)
			r.With(identity.RequireAnyPermission([]string{"restaurants.write", "restaurants.manage"}, orgs)).
				Post("/restaurants/{restaurantId}/locations", rh.CreateLocation)
			r.With(identity.RequireAnyPermission([]string{"restaurants.write", "restaurants.manage"}, orgs)).
				Patch("/restaurants/{restaurantId}/locations/{locationId}", rh.UpdateLocation)

			// Suppliers (supplier organizations)
			r.With(identity.RequireAnyPermission([]string{"suppliers.read", "suppliers.manage"}, orgs)).
				Get("/suppliers", sh.List)
			r.With(identity.RequireAnyPermission([]string{"suppliers.write", "suppliers.manage"}, orgs)).
				Post("/suppliers", sh.Create)
			r.With(identity.RequireAnyPermission([]string{"suppliers.read", "suppliers.manage"}, orgs)).
				Get("/suppliers/{supplierId}", sh.Get)
			r.With(identity.RequireAnyPermission([]string{"suppliers.write", "suppliers.manage"}, orgs)).
				Patch("/suppliers/{supplierId}", sh.Update)

			// Catalog: product management (supplier organizations).
			// NOTE: categories are registered before /products/{productId} so
			// chi routes them to the category handlers.
			r.With(identity.RequireAnyPermission([]string{"catalog.read", "catalog.manage"}, orgs)).
				Get("/products/categories", ch.ListCategories)
			r.With(identity.RequireAnyPermission([]string{"catalog.manage"}, orgs)).
				Post("/products/categories", ch.CreateCategory)
			r.With(identity.RequireAnyPermission([]string{"catalog.read", "catalog.manage"}, orgs)).
				Get("/products", ch.List)
			r.With(identity.RequireAnyPermission([]string{"catalog.manage"}, orgs)).
				Post("/products", ch.Create)
			r.With(identity.RequireAnyPermission([]string{"catalog.read", "catalog.manage"}, orgs)).
				Get("/products/{productId}", ch.Get)
			r.With(identity.RequireAnyPermission([]string{"catalog.manage"}, orgs)).
				Patch("/products/{productId}", ch.Update)

			// Catalog: marketplace browsing (buyer organizations).
			r.With(identity.RequireAnyPermission([]string{"catalog.read", "catalog.manage"}, orgs)).
				Get("/catalog/products", ch.Browse)
			r.With(identity.RequireAnyPermission([]string{"catalog.read", "catalog.manage"}, orgs)).
				Get("/catalog/categories", ch.BrowseCategories)

			// Procurement: buyer cart + purchase requests. Write access covers
			// cart management, submission and cancellation; read access covers
			// listing/viewing. Org-type restrictions (buyer-only cart/requests,
			// supplier-only incoming) are enforced in the service layer.
			r.With(identity.RequireAnyPermission([]string{"procurement.write", "procurement.manage"}, orgs)).
				Get("/procurement/cart", ph.GetCart)
			r.With(identity.RequireAnyPermission([]string{"procurement.write", "procurement.manage"}, orgs)).
				Post("/procurement/cart/items", ph.AddItem)
			r.With(identity.RequireAnyPermission([]string{"procurement.write", "procurement.manage"}, orgs)).
				Patch("/procurement/cart/items/{itemId}", ph.UpdateItem)
			r.With(identity.RequireAnyPermission([]string{"procurement.write", "procurement.manage"}, orgs)).
				Delete("/procurement/cart/items/{itemId}", ph.RemoveItem)
			r.With(identity.RequireAnyPermission([]string{"procurement.write", "procurement.manage"}, orgs)).
				Post("/procurement/submit", ph.Submit)
			r.With(identity.RequireAnyPermission([]string{"procurement.read", "procurement.manage"}, orgs)).
				Get("/procurement/requests", ph.ListRequests)
			r.With(identity.RequireAnyPermission([]string{"procurement.read", "procurement.manage"}, orgs)).
				Get("/procurement/requests/{requestId}", ph.GetRequest)
			r.With(identity.RequireAnyPermission([]string{"procurement.write", "procurement.manage"}, orgs)).
				Patch("/procurement/requests/{requestId}/cancel", ph.CancelRequest)
			r.With(identity.RequireAnyPermission([]string{"procurement.read", "procurement.manage"}, orgs)).
				Get("/procurement/incoming", ph.ListIncoming)
			r.With(identity.RequireAnyPermission([]string{"procurement.read", "procurement.manage"}, orgs)).
				Get("/procurement/incoming/{requestId}", ph.GetIncoming)
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
