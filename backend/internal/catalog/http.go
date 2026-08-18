package catalog

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

// Handler exposes the catalog module over HTTP.
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

// productRequest is the full-update payload for create and update.
type productRequest struct {
	Name        string  `json:"name"`
	SKU         string  `json:"sku"`
	Description string  `json:"description"`
	Unit        string  `json:"unit"`
	Status      string  `json:"status"`
	CategoryID  *string `json:"category_id"`
	ImageURL    string  `json:"image_url"`
	MinOrderQty *int32  `json:"min_order_qty"`
	VatRateBps  *int32  `json:"vat_rate_bps"`
	PriceMinor  int64   `json:"price_minor"`
	Currency    string  `json:"currency"`
}

func (req *productRequest) validate() error {
	v := &platformhttp.Validator{}
	v.Required("name", req.Name, 200)
	v.MaxLen("sku", req.SKU, 100)
	v.MaxLen("description", req.Description, 1000)
	if req.Unit == "" {
		req.Unit = "piece"
	}
	v.OneOf("unit", req.Unit, "piece", "kg", "g", "l", "ml", "pack", "box")
	if req.Status == "" {
		req.Status = StatusActive
	}
	v.OneOf("status", req.Status, StatusDraft, StatusActive, StatusArchived)
	if req.CategoryID != nil && *req.CategoryID != "" {
		v.UUID("category_id", *req.CategoryID)
	}
	v.MaxLen("image_url", req.ImageURL, 2048)
	if req.MinOrderQty == nil || *req.MinOrderQty <= 0 {
		one := int32(1)
		req.MinOrderQty = &one
	}
	if req.VatRateBps == nil || *req.VatRateBps < 0 {
		vat := int32(1200)
		req.VatRateBps = &vat
	}
	if req.Currency == "" {
		req.Currency = "KZT"
	}
	v.UpperAlpha("currency", req.Currency, 3)
	if req.PriceMinor < 0 {
		v.Add("price_minor", "must be non-negative")
	}
	return v.Error()
}

func (req *productRequest) toInput() ProductInput {
	in := ProductInput{
		Name: req.Name, SKU: req.SKU, Description: req.Description,
		Unit: req.Unit, Status: req.Status, ImageURL: req.ImageURL,
		MinOrderQty: *req.MinOrderQty, VatRateBps: *req.VatRateBps,
		PriceMinor: req.PriceMinor, Currency: req.Currency,
	}
	if req.CategoryID != nil && *req.CategoryID != "" {
		if id, err := uuid.Parse(*req.CategoryID); err == nil {
			in.CategoryID = &id
		}
	}
	return in
}

type categoryRequest struct {
	Name string `json:"name"`
}

func (req *categoryRequest) validate() error {
	v := &platformhttp.Validator{}
	v.Required("name", req.Name, 100)
	return v.Error()
}

// List handles GET /api/v1/products.
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

// Create handles POST /api/v1/products.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	c, ok := ctxFrom(r)
	if !ok {
		identityUnauthenticated(w, r)
		return
	}
	var req productRequest
	if err := platformhttp.DecodeJSON(r, &req); err != nil {
		platformhttp.WriteProblem(w, r, http.StatusBadRequest, "MALFORMED_REQUEST", "Malformed request body", err.Error())
		return
	}
	if err := req.validate(); err != nil {
		platformhttp.WriteValidationErrors(w, r, err.(platformhttp.ValidationErrors))
		return
	}
	out, err := h.svc.Create(r.Context(), c, req.toInput())
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	platformhttp.WriteJSON(w, http.StatusCreated, out)
}

// Get handles GET /api/v1/products/{productId}.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	c, ok := ctxFrom(r)
	if !ok {
		identityUnauthenticated(w, r)
		return
	}
	id, err := pathUUID(r, "productId")
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

// Update handles PATCH /api/v1/products/{productId}.
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	c, ok := ctxFrom(r)
	if !ok {
		identityUnauthenticated(w, r)
		return
	}
	id, err := pathUUID(r, "productId")
	if err != nil {
		platformhttp.WriteProblem(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Validation failed", err.Error())
		return
	}
	var req productRequest
	if err := platformhttp.DecodeJSON(r, &req); err != nil {
		platformhttp.WriteProblem(w, r, http.StatusBadRequest, "MALFORMED_REQUEST", "Malformed request body", err.Error())
		return
	}
	if err := req.validate(); err != nil {
		platformhttp.WriteValidationErrors(w, r, err.(platformhttp.ValidationErrors))
		return
	}
	out, err := h.svc.Update(r.Context(), c, id, req.toInput())
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	platformhttp.WriteJSON(w, http.StatusOK, out)
}

// ListCategories handles GET /api/v1/products/categories.
func (h *Handler) ListCategories(w http.ResponseWriter, r *http.Request) {
	c, ok := ctxFrom(r)
	if !ok {
		identityUnauthenticated(w, r)
		return
	}
	limit, offset := parsePagination(r)
	items, total, err := h.svc.ListCategories(r.Context(), c, limit, offset)
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

// CreateCategory handles POST /api/v1/products/categories.
func (h *Handler) CreateCategory(w http.ResponseWriter, r *http.Request) {
	c, ok := ctxFrom(r)
	if !ok {
		identityUnauthenticated(w, r)
		return
	}
	var req categoryRequest
	if err := platformhttp.DecodeJSON(r, &req); err != nil {
		platformhttp.WriteProblem(w, r, http.StatusBadRequest, "MALFORMED_REQUEST", "Malformed request body", err.Error())
		return
	}
	if err := req.validate(); err != nil {
		platformhttp.WriteValidationErrors(w, r, err.(platformhttp.ValidationErrors))
		return
	}
	out, err := h.svc.CreateCategory(r.Context(), c, CategoryInput(req))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	platformhttp.WriteJSON(w, http.StatusCreated, out)
}

// Browse handles GET /api/v1/catalog/products (buyer marketplace).
func (h *Handler) Browse(w http.ResponseWriter, r *http.Request) {
	c, ok := ctxFrom(r)
	if !ok {
		identityUnauthenticated(w, r)
		return
	}
	limit, offset := parsePagination(r)
	q := r.URL.Query().Get("q")
	var supplierID, categoryID *uuid.UUID
	for _, f := range []struct {
		param string
		ptr   **uuid.UUID
	}{{"supplier_id", &supplierID}, {"category_id", &categoryID}} {
		raw := r.URL.Query().Get(f.param)
		if raw == "" {
			continue
		}
		id, err := uuid.Parse(raw)
		if err != nil {
			platformhttp.WriteProblem(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Validation failed", f.param+" must be a valid UUID")
			return
		}
		*f.ptr = &id
	}
	items, total, err := h.svc.Browse(r.Context(), c, q, supplierID, categoryID, limit, offset)
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

// BrowseCategories handles GET /api/v1/catalog/categories (buyer marketplace
// filter options).
func (h *Handler) BrowseCategories(w http.ResponseWriter, r *http.Request) {
	c, ok := ctxFrom(r)
	if !ok {
		identityUnauthenticated(w, r)
		return
	}
	items, total, err := h.svc.BrowseCategories(r.Context(), c)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	platformhttp.WriteJSON(w, http.StatusOK, map[string]any{
		"data":   items,
		"limit":  len(items),
		"offset": 0,
		"total":  total,
	})
}

func (h *Handler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	var e *Error
	if errors.As(err, &e) {
		platformhttp.WriteProblem(w, r, e.HTTPStatus, e.Code, e.Msg, e.Msg)
		return
	}
	h.log.Error().Err(err).Msg("catalog request failed")
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
