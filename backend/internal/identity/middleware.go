package identity

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"tirek/backend/internal/organizations"
	platformhttp "tirek/backend/internal/platform/http"
)

type ctxKey struct{ name string }

func (k ctxKey) String() string { return "identity." + k.name }

// claimsKey carries the authenticated identity in the request context.
var claimsKey = ctxKey{"claims"}

// AuthContext is the authenticated identity for a request.
type AuthContext struct {
	UserID    uuid.UUID
	OrgID     uuid.UUID
	SessionID uuid.UUID
	Role      string
}

// ClaimsFromContext returns the authenticated identity, if any.
func ClaimsFromContext(ctx context.Context) (*AuthContext, bool) {
	c, ok := ctx.Value(claimsKey).(*AuthContext)
	return c, ok
}

// Authenticate is a middleware that validates the Bearer access token and
// injects the identity into the request context.
func (s *Service) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			platformhttp.WriteProblem(w, r, http.StatusUnauthorized,
				ErrUnauthenticated.Code, ErrUnauthenticated.Msg, "missing or malformed Authorization header")
			return
		}
		raw := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
		if raw == "" {
			platformhttp.WriteProblem(w, r, http.StatusUnauthorized,
				ErrUnauthenticated.Code, ErrUnauthenticated.Msg, "missing or malformed Authorization header")
			return
		}
		claims, err := s.tokens.ParseAccessToken(raw)
		if err != nil {
			platformhttp.WriteProblem(w, r, http.StatusUnauthorized,
				ErrUnauthenticated.Code, ErrUnauthenticated.Msg, "access token is invalid or expired")
			return
		}
		userID, _ := uuid.Parse(claims.Subject)
		orgID, _ := uuid.Parse(claims.OrgID)
		sessionID, _ := uuid.Parse(claims.SessionID)
		ctx := context.WithValue(r.Context(), claimsKey, &AuthContext{
			UserID: userID, OrgID: orgID, SessionID: sessionID, Role: claims.Role,
		})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequirePermission is a middleware that enforces RBAC for the current role.
// Roles carrying the implicit org.owner permission pass every check.
func RequirePermission(perm string, orgs *organizations.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ac, ok := ClaimsFromContext(r.Context())
			if !ok {
				platformhttp.WriteProblem(w, r, http.StatusUnauthorized,
					ErrUnauthenticated.Code, ErrUnauthenticated.Msg, "authentication required")
				return
			}
			perms, err := orgs.Permissions(r.Context(), ac.OrgID, ac.Role)
			if err != nil {
				platformhttp.WriteProblem(w, r, http.StatusInternalServerError,
					"INTERNAL_ERROR", "Internal server error", "")
				return
			}
			if !perms[perm] && !perms["org.owner"] {
				platformhttp.WriteProblem(w, r, http.StatusForbidden,
					ErrForbidden.Code, ErrForbidden.Msg, "missing permission: "+perm)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireAnyPermission is a middleware that enforces RBAC, passing when the
// current role holds any one of the given permissions (or the implicit
// org.owner wildcard). Used where a finer-grained "write" permission and a
// broader "manage" permission both satisfy the endpoint.
func RequireAnyPermission(perms []string, orgs *organizations.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ac, ok := ClaimsFromContext(r.Context())
			if !ok {
				platformhttp.WriteProblem(w, r, http.StatusUnauthorized,
					ErrUnauthenticated.Code, ErrUnauthenticated.Msg, "authentication required")
				return
			}
			held, err := orgs.Permissions(r.Context(), ac.OrgID, ac.Role)
			if err != nil {
				platformhttp.WriteProblem(w, r, http.StatusInternalServerError,
					"INTERNAL_ERROR", "Internal server error", "")
				return
			}
			if held["org.owner"] {
				next.ServeHTTP(w, r)
				return
			}
			for _, p := range perms {
				if held[p] {
					next.ServeHTTP(w, r)
					return
				}
			}
			platformhttp.WriteProblem(w, r, http.StatusForbidden,
				ErrForbidden.Code, ErrForbidden.Msg, "missing permission: "+strings.Join(perms, " or "))
		})
	}
}
