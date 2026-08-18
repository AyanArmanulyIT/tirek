package restaurants

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

// Handler exposes the restaurants module over HTTP.
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

// parsePagination reads limit/offset query params with sane bounds.
func parsePagination(r *http.Request) (limit, offset int) {
	limit = defaultLimit
	offset = 0
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

// ctxFrom builds the service context from the authenticated request.
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

type createRestaurantRequest struct {
	Name            string `json:"name"`
	LegalName       string `json:"legal_name"`
	Status          string `json:"status"`
	Country         string `json:"country"`
	DefaultCurrency string `json:"default_currency"`
	Phone           string `json:"phone"`
	Email           string `json:"email"`
}

func (req *createRestaurantRequest) validate() error {
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

type updateRestaurantRequest struct {
	Name            string `json:"name"`
	LegalName       string `json:"legal_name"`
	Status          string `json:"status"`
	Country         string `json:"country"`
	DefaultCurrency string `json:"default_currency"`
	Phone           string `json:"phone"`
	Email           string `json:"email"`
}

func (req *updateRestaurantRequest) validate() error {
	v := &platformhttp.Validator{}
	v.Required("name", req.Name, 200)
	v.MaxLen("legal_name", req.LegalName, 200)
	v.OneOf("status", req.Status, "active", "suspended", "closed")
	v.UpperAlpha("country", req.Country, 2)
	v.UpperAlpha("default_currency", req.DefaultCurrency, 3)
	v.MaxLen("phone", req.Phone, 32)
	if req.Email != "" {
		v.Email("email", req.Email)
	}
	return v.Error()
}

// List handles GET /api/v1/restaurants.
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

// Create handles POST /api/v1/restaurants.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	c, ok := ctxFrom(r)
	if !ok {
		identityUnauthenticated(w, r)
		return
	}
	var req createRestaurantRequest
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

// Get handles GET /api/v1/restaurants/{restaurantId}.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	c, ok := ctxFrom(r)
	if !ok {
		identityUnauthenticated(w, r)
		return
	}
	id, err := pathUUID(r, "restaurantId")
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

// Update handles PATCH /api/v1/restaurants/{restaurantId}.
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	c, ok := ctxFrom(r)
	if !ok {
		identityUnauthenticated(w, r)
		return
	}
	id, err := pathUUID(r, "restaurantId")
	if err != nil {
		platformhttp.WriteProblem(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Validation failed", err.Error())
		return
	}
	var req updateRestaurantRequest
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

type createLocationRequest struct {
	Name    string `json:"name"`
	Address string `json:"address"`
	City    string `json:"city"`
	Country string `json:"country"`
	Status  string `json:"status"`
	Phone   string `json:"phone"`
	Email   string `json:"email"`
}

func (req *createLocationRequest) validate() error {
	v := &platformhttp.Validator{}
	v.Required("name", req.Name, 200)
	v.Required("address", req.Address, 500)
	v.Required("city", req.City, 200)
	if req.Status == "" {
		req.Status = "active"
	}
	v.OneOf("status", req.Status, "active", "inactive")
	if req.Country == "" {
		req.Country = "KZ"
	}
	v.UpperAlpha("country", req.Country, 2)
	v.MaxLen("phone", req.Phone, 32)
	if req.Email != "" {
		v.Email("email", req.Email)
	}
	return v.Error()
}

// CreateLocation handles POST /api/v1/restaurants/{restaurantId}/locations.
func (h *Handler) CreateLocation(w http.ResponseWriter, r *http.Request) {
	c, ok := ctxFrom(r)
	if !ok {
		identityUnauthenticated(w, r)
		return
	}
	restaurantID, err := pathUUID(r, "restaurantId")
	if err != nil {
		platformhttp.WriteProblem(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Validation failed", err.Error())
		return
	}
	var req createLocationRequest
	if err := platformhttp.DecodeJSON(r, &req); err != nil {
		platformhttp.WriteProblem(w, r, http.StatusBadRequest, "MALFORMED_REQUEST", "Malformed request body", err.Error())
		return
	}
	if err := req.validate(); err != nil {
		platformhttp.WriteValidationErrors(w, r, err.(platformhttp.ValidationErrors))
		return
	}
	out, err := h.svc.CreateLocation(r.Context(), c, restaurantID, CreateLocationInput(req))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	platformhttp.WriteJSON(w, http.StatusCreated, out)
}

// ListLocations handles GET /api/v1/restaurants/{restaurantId}/locations.
func (h *Handler) ListLocations(w http.ResponseWriter, r *http.Request) {
	c, ok := ctxFrom(r)
	if !ok {
		identityUnauthenticated(w, r)
		return
	}
	restaurantID, err := pathUUID(r, "restaurantId")
	if err != nil {
		platformhttp.WriteProblem(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Validation failed", err.Error())
		return
	}
	limit, offset := parsePagination(r)
	items, total, err := h.svc.ListLocations(r.Context(), c, restaurantID, limit, offset)
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

// UpdateLocation handles PATCH /api/v1/restaurants/{restaurantId}/locations/{locationId}.
func (h *Handler) UpdateLocation(w http.ResponseWriter, r *http.Request) {
	c, ok := ctxFrom(r)
	if !ok {
		identityUnauthenticated(w, r)
		return
	}
	locationID, err := pathUUID(r, "locationId")
	if err != nil {
		platformhttp.WriteProblem(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Validation failed", err.Error())
		return
	}
	var req createLocationRequest
	if err := platformhttp.DecodeJSON(r, &req); err != nil {
		platformhttp.WriteProblem(w, r, http.StatusBadRequest, "MALFORMED_REQUEST", "Malformed request body", err.Error())
		return
	}
	if err := req.validate(); err != nil {
		platformhttp.WriteValidationErrors(w, r, err.(platformhttp.ValidationErrors))
		return
	}
	out, err := h.svc.UpdateLocation(r.Context(), c, locationID, UpdateLocationInput(req))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	platformhttp.WriteJSON(w, http.StatusOK, out)
}

// writeError maps domain errors to RFC 7807 responses.
func (h *Handler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	var e *Error
	if errors.As(err, &e) {
		platformhttp.WriteProblem(w, r, e.HTTPStatus, e.Code, e.Msg, e.Msg)
		return
	}
	h.log.Error().Err(err).Msg("restaurants request failed")
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
