package suppliers

import (
	"errors"
	"net/http"
	"net/netip"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"tirek/backend/internal/identity"
	platformhttp "tirek/backend/internal/platform/http"
)

// Handler exposes the suppliers module over HTTP.
type Handler struct {
	svc *Service
	log zerolog.Logger
}

func NewHandler(svc *Service, log zerolog.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

const (
	defaultLimit = 50
	maxLimit     = 200
)

func parsePagination(r *http.Request) (limit, offset int) {
	limit = defaultLimit
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 {
		limit = v
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	if v, err := strconv.Atoi(r.URL.Query().Get("offset")); err == nil && v >= 0 {
		offset = v
	}
	return limit, offset
}

func ctxFrom(r *http.Request) (Ctx, bool) {
	ac, ok := identity.ClaimsFromContext(r.Context())
	if !ok {
		return Ctx{}, false
	}
	return Ctx{
		UserID:    ac.UserID,
		OrgID:     ac.OrgID,
		Role:      ac.Role,
		IP:        clientIP(r),
		UserAgent: r.UserAgent(),
	}, true
}

func clientIP(r *http.Request) *netip.Addr {
	ap, err := netip.ParseAddrPort(r.RemoteAddr)
	if err != nil {
		return nil
	}
	addr := ap.Addr()
	return &addr
}

type supplierRequest struct {
	Name            string `json:"name"`
	LegalName       string `json:"legal_name"`
	Status          string `json:"status"`
	Country         string `json:"country"`
	DefaultCurrency string `json:"default_currency"`
	Phone           string `json:"phone"`
	Email           string `json:"email"`
}

func (req *supplierRequest) validate() error {
	v := &platformhttp.Validator{}
	v.Required("name", req.Name, 200)
	v.MaxLen("legal_name", req.LegalName, 200)
	if req.Status == "" {
		req.Status = "active"
	}
	v.OneOf("status", req.Status, "active", "suspended", "closed")
	if req.Country == "" {
		req.Country = "KZ"
	}
	v.UpperAlpha("country", req.Country, 2)
	if req.DefaultCurrency == "" {
		req.DefaultCurrency = "KZT"
	}
	v.UpperAlpha("default_currency", req.DefaultCurrency, 3)
	v.MaxLen("phone", req.Phone, 32)
	if req.Email != "" {
		v.Email("email", req.Email)
	}
	return v.Error()
}

// List handles GET /api/v1/suppliers.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	c, ok := ctxFrom(r)
	if !ok {
		identityUnauthenticated(w, r)
		return
	}
	limit, offset := parsePagination(r)
	items, total, err := h.svc.List(r.Context(), c, limit, offset)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	platformhttp.WriteJSON(w, http.StatusOK, map[string]any{
		"data":   items,
		"limit":  limit,
		"offset": offset,
		"total":  total,
	})
}

// Create handles POST /api/v1/suppliers.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	c, ok := ctxFrom(r)
	if !ok {
		identityUnauthenticated(w, r)
		return
	}
	var req supplierRequest
	if err := platformhttp.DecodeJSON(r, &req); err != nil {
		platformhttp.WriteProblem(w, r, http.StatusBadRequest, "MALFORMED_REQUEST", "Malformed request body", err.Error())
		return
	}
	if err := req.validate(); err != nil {
		platformhttp.WriteValidationErrors(w, r, err.(platformhttp.ValidationErrors))
		return
	}
	out, err := h.svc.Create(r.Context(), c, CreateInput(req))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	platformhttp.WriteJSON(w, http.StatusCreated, out)
}

// Get handles GET /api/v1/suppliers/{supplierId}.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	c, ok := ctxFrom(r)
	if !ok {
		identityUnauthenticated(w, r)
		return
	}
	id, err := pathUUID(r, "supplierId")
	if err != nil {
		platformhttp.WriteProblem(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Validation failed", err.Error())
		return
	}
	out, err := h.svc.Get(r.Context(), c, id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	platformhttp.WriteJSON(w, http.StatusOK, out)
}

// Update handles PATCH /api/v1/suppliers/{supplierId}.
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	c, ok := ctxFrom(r)
	if !ok {
		identityUnauthenticated(w, r)
		return
	}
	id, err := pathUUID(r, "supplierId")
	if err != nil {
		platformhttp.WriteProblem(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Validation failed", err.Error())
		return
	}
	var req supplierRequest
	if err := platformhttp.DecodeJSON(r, &req); err != nil {
		platformhttp.WriteProblem(w, r, http.StatusBadRequest, "MALFORMED_REQUEST", "Malformed request body", err.Error())
		return
	}
	if err := req.validate(); err != nil {
		platformhttp.WriteValidationErrors(w, r, err.(platformhttp.ValidationErrors))
		return
	}
	out, err := h.svc.Update(r.Context(), c, id, UpdateInput(req))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	platformhttp.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	var e *Error
	if errors.As(err, &e) {
		platformhttp.WriteProblem(w, r, e.HTTPStatus, e.Code, e.Msg, e.Msg)
		return
	}
	h.log.Error().Err(err).Msg("suppliers request failed")
	platformhttp.WriteProblem(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error", "")
}

func identityUnauthenticated(w http.ResponseWriter, r *http.Request) {
	platformhttp.WriteProblem(w, r, http.StatusUnauthorized,
		identity.ErrUnauthenticated.Code, identity.ErrUnauthenticated.Msg, "authentication required")
}

func pathUUID(r *http.Request, name string) (uuid.UUID, error) {
	raw := chi.URLParam(r, name)
	if raw == "" {
		return uuid.Nil, errors.New("missing path parameter")
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, errors.New("invalid path parameter: must be a UUID")
	}
	return id, nil
}
