package identity

import (
	"errors"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog"

	"tirek/backend/internal/platform/config"
	platformhttp "tirek/backend/internal/platform/http"
)

// refreshCookiePath scopes the refresh cookie to the auth endpoints.
const refreshCookiePath = "/api/v1/auth"

// Handler exposes the identity module over HTTP.
type Handler struct {
	svc        *Service
	authConfig config.AuthConfig
	cookieName string
	log        zerolog.Logger
}

func NewHandler(svc *Service, authConfig config.AuthConfig, cookieName string, log zerolog.Logger) *Handler {
	return &Handler{svc: svc, authConfig: authConfig, cookieName: cookieName, log: log}
}

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	FullName string `json:"full_name"`
	Phone    string `json:"phone,omitempty"`
	OrgName  string `json:"org_name"`
	OrgType  string `json:"org_type"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	OrgID    string `json:"org_id,omitempty"`
}

// Register handles POST /api/v1/auth/register.
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := platformhttp.DecodeJSON(r, &req); err != nil {
		platformhttp.WriteProblem(w, r, http.StatusBadRequest, "MALFORMED_REQUEST", "Malformed request body", err.Error())
		return
	}
	v := &platformhttp.Validator{}
	v.Email("email", req.Email)
	v.Required("full_name", req.FullName, 200)
	v.Required("org_name", req.OrgName, 200)
	v.OneOf("org_type", req.OrgType, "buyer", "supplier")
	v.MaxLen("phone", req.Phone, 32)
	v.Required("password", req.Password, 128)
	if len(req.Password) < 12 {
		v.Add("password", "must be at least 12 characters")
	}
	if err := v.Error(); err != nil {
		platformhttp.WriteValidationErrors(w, r, err.(platformhttp.ValidationErrors))
		return
	}

	sess, err := h.svc.Register(r.Context(), RegisterInput(req), clientIP(r), r.UserAgent())
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.setRefreshCookie(w, sess.RefreshToken, sess.RefreshExpiresAt)
	platformhttp.WriteJSON(w, http.StatusCreated, authResponseFrom(sess))
}

// Login handles POST /api/v1/auth/login.
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := platformhttp.DecodeJSON(r, &req); err != nil {
		platformhttp.WriteProblem(w, r, http.StatusBadRequest, "MALFORMED_REQUEST", "Malformed request body", err.Error())
		return
	}
	v := &platformhttp.Validator{}
	v.Email("email", req.Email)
	v.Required("password", req.Password, 128)
	if req.OrgID != "" {
		v.UUID("org_id", req.OrgID)
	}
	if err := v.Error(); err != nil {
		platformhttp.WriteValidationErrors(w, r, err.(platformhttp.ValidationErrors))
		return
	}

	in := LoginInput{Email: req.Email, Password: req.Password}
	if req.OrgID != "" {
		id, err := parseUUID(req.OrgID)
		if err != nil {
			platformhttp.WriteValidationErrors(w, r, platformhttp.ValidationErrors{{Field: "org_id", Message: "must be a valid UUID"}})
			return
		}
		in.OrgID = &id
	}

	sess, err := h.svc.Login(r.Context(), in, clientIP(r), r.UserAgent())
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.setRefreshCookie(w, sess.RefreshToken, sess.RefreshExpiresAt)
	platformhttp.WriteJSON(w, http.StatusOK, authResponseFrom(sess))
}

// Refresh handles POST /api/v1/auth/refresh.
func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	raw := h.readRefreshCookie(r)
	if raw == "" {
		platformhttp.WriteProblem(w, r, http.StatusUnauthorized,
			ErrInvalidRefreshToken.Code, ErrInvalidRefreshToken.Msg, "refresh cookie missing")
		return
	}
	sess, err := h.svc.Refresh(r.Context(), raw, clientIP(r), r.UserAgent())
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.setRefreshCookie(w, sess.RefreshToken, sess.RefreshExpiresAt)
	platformhttp.WriteJSON(w, http.StatusOK, authResponseFrom(sess))
}

// Logout handles POST /api/v1/auth/logout.
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	raw := h.readRefreshCookie(r)
	if err := h.svc.Logout(r.Context(), raw, clientIP(r), r.UserAgent()); err != nil {
		h.writeError(w, r, err)
		return
	}
	h.clearRefreshCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

// Me handles GET /api/v1/me.
func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	ac, ok := ClaimsFromContext(r.Context())
	if !ok {
		platformhttp.WriteProblem(w, r, http.StatusUnauthorized,
			ErrUnauthenticated.Code, ErrUnauthenticated.Msg, "authentication required")
		return
	}
	me, err := h.svc.Me(r.Context(), ac.UserID, ac.OrgID)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	platformhttp.WriteJSON(w, http.StatusOK, meResponse{
		User:        userResponseFrom(me.User),
		Org:         orgResponseFrom(me.Org),
		Role:        me.Role,
		Permissions: me.Permissions,
	})
}

func (h *Handler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	var e *Error
	if errors.As(err, &e) {
		platformhttp.WriteProblem(w, r, e.HTTPStatus, e.Code, e.Msg, e.Msg)
		return
	}
	if errors.Is(err, pgx.ErrNoRows) {
		platformhttp.WriteProblem(w, r, http.StatusNotFound, "NOT_FOUND", "Resource not found", "")
		return
	}
	h.log.Error().Err(err).Msg("identity request failed")
	platformhttp.WriteProblem(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error", "")
}

func (h *Handler) setRefreshCookie(w http.ResponseWriter, raw string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     h.cookieName,
		Value:    raw,
		Path:     refreshCookiePath,
		Domain:   h.authConfig.CookieDomain,
		HttpOnly: true,
		Secure:   h.authConfig.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		Expires:  expiresAt,
		MaxAge:   int(time.Until(expiresAt).Seconds()),
	})
}

func (h *Handler) clearRefreshCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     h.cookieName,
		Value:    "",
		Path:     refreshCookiePath,
		Domain:   h.authConfig.CookieDomain,
		HttpOnly: true,
		Secure:   h.authConfig.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

func (h *Handler) readRefreshCookie(r *http.Request) string {
	c, err := r.Cookie(h.cookieName)
	if err != nil {
		return ""
	}
	return c.Value
}

func clientIP(r *http.Request) *netip.Addr {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	host = strings.Trim(host, "[]")
	if host == "" {
		return nil
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return nil
	}
	return &addr
}

func parseUUID(s string) (uuid.UUID, error) { return uuid.Parse(s) }

type authResponse struct {
	AccessToken string       `json:"access_token"`
	TokenType   string       `json:"token_type"`
	ExpiresIn   int          `json:"expires_in"`
	User        userResponse `json:"user"`
	Org         orgResponse  `json:"org"`
	Role        string       `json:"role"`
}

type userResponse struct {
	UserID   string `json:"user_id"`
	Email    string `json:"email"`
	FullName string `json:"full_name"`
	Phone    string `json:"phone,omitempty"`
	Status   string `json:"status"`
}

type orgResponse struct {
	OrgID           string `json:"org_id"`
	Name            string `json:"name"`
	Type            string `json:"type"`
	Country         string `json:"country"`
	DefaultCurrency string `json:"default_currency"`
	Status          string `json:"status"`
}

type meResponse struct {
	User        userResponse `json:"user"`
	Org         orgResponse  `json:"org"`
	Role        string       `json:"role"`
	Permissions []string     `json:"permissions"`
}

func authResponseFrom(s *Session) authResponse {
	return authResponse{
		AccessToken: s.AccessToken,
		TokenType:   "Bearer",
		ExpiresIn:   int(time.Until(s.AccessExpiresAt).Seconds()),
		User:        userResponseFrom(s.User),
		Org:         orgResponseFrom(s.Org),
		Role:        s.Role,
	}
}

func userResponseFrom(u User) userResponse {
	return userResponse{
		UserID: u.UserID.String(), Email: u.Email, FullName: u.FullName,
		Phone: u.Phone, Status: u.Status,
	}
}

func orgResponseFrom(o Organization) orgResponse {
	return orgResponse{
		OrgID: o.OrgID.String(), Name: o.Name, Type: o.Type,
		Country: o.Country, DefaultCurrency: o.DefaultCurrency, Status: o.Status,
	}
}
