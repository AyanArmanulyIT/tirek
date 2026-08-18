package procurement

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

// Handler exposes the procurement module over HTTP.
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

type addItemRequest struct {
	ProductID string `json:"product_id"`
	Quantity  *int   `json:"quantity"`
}

func (req *addItemRequest) validate() error {
	v := &platformhttp.Validator{}
	v.UUID("product_id", req.ProductID)
	if req.Quantity == nil {
		one := 1
		req.Quantity = &one
	}
	if *req.Quantity < 1 {
		v.Add("quantity", "must be a positive integer")
	}
	return v.Error()
}

type updateItemRequest struct {
	Quantity *int `json:"quantity"`
}

func (req *updateItemRequest) validate() error {
	v := &platformhttp.Validator{}
	if req.Quantity == nil {
		v.Add("quantity", "required")
	} else if *req.Quantity < 1 {
		v.Add("quantity", "must be a positive integer")
	}
	return v.Error()
}

// GetCart handles GET /api/v1/procurement/cart.
func (h *Handler) GetCart(w http.ResponseWriter, r *http.Request) {
	c, ok := ctxFrom(r)
	if !ok {
		identityUnauthenticated(w, r)
		return
	}
	out, err := h.svc.GetCart(r.Context(), c)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	platformhttp.WriteJSON(w, http.StatusOK, out)
}

// AddItem handles POST /api/v1/procurement/cart/items.
func (h *Handler) AddItem(w http.ResponseWriter, r *http.Request) {
	c, ok := ctxFrom(r)
	if !ok {
		identityUnauthenticated(w, r)
		return
	}
	var req addItemRequest
	if err := platformhttp.DecodeJSON(r, &req); err != nil {
		platformhttp.WriteProblem(w, r, http.StatusBadRequest, "MALFORMED_REQUEST", "Malformed request body", err.Error())
		return
	}
	if err := req.validate(); err != nil {
		platformhttp.WriteValidationErrors(w, r, err.(platformhttp.ValidationErrors))
		return
	}
	productID, _ := uuid.Parse(req.ProductID)
	out, err := h.svc.AddItem(r.Context(), c, productID, *req.Quantity)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	platformhttp.WriteJSON(w, http.StatusOK, out)
}

// UpdateItem handles PATCH /api/v1/procurement/cart/items/{itemId}.
func (h *Handler) UpdateItem(w http.ResponseWriter, r *http.Request) {
	c, ok := ctxFrom(r)
	if !ok {
		identityUnauthenticated(w, r)
		return
	}
	id, err := pathUUID(r, "itemId")
	if err != nil {
		platformhttp.WriteProblem(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Validation failed", err.Error())
		return
	}
	var req updateItemRequest
	if err := platformhttp.DecodeJSON(r, &req); err != nil {
		platformhttp.WriteProblem(w, r, http.StatusBadRequest, "MALFORMED_REQUEST", "Malformed request body", err.Error())
		return
	}
	if err := req.validate(); err != nil {
		platformhttp.WriteValidationErrors(w, r, err.(platformhttp.ValidationErrors))
		return
	}
	out, err := h.svc.UpdateItemQuantity(r.Context(), c, id, *req.Quantity)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	platformhttp.WriteJSON(w, http.StatusOK, out)
}

// RemoveItem handles DELETE /api/v1/procurement/cart/items/{itemId}.
func (h *Handler) RemoveItem(w http.ResponseWriter, r *http.Request) {
	c, ok := ctxFrom(r)
	if !ok {
		identityUnauthenticated(w, r)
		return
	}
	id, err := pathUUID(r, "itemId")
	if err != nil {
		platformhttp.WriteProblem(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Validation failed", err.Error())
		return
	}
	out, err := h.svc.RemoveItem(r.Context(), c, id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	platformhttp.WriteJSON(w, http.StatusOK, out)
}

// Submit handles POST /api/v1/procurement/submit.
func (h *Handler) Submit(w http.ResponseWriter, r *http.Request) {
	c, ok := ctxFrom(r)
	if !ok {
		identityUnauthenticated(w, r)
		return
	}
	out, err := h.svc.Submit(r.Context(), c, r.Header.Get("Idempotency-Key"))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	platformhttp.WriteJSON(w, http.StatusOK, out)
}

// ListRequests handles GET /api/v1/procurement/requests (buyer).
func (h *Handler) ListRequests(w http.ResponseWriter, r *http.Request) {
	c, ok := ctxFrom(r)
	if !ok {
		identityUnauthenticated(w, r)
		return
	}
	limit, offset := parsePagination(r)
	items, total, err := h.svc.ListRequests(r.Context(), c, limit, offset)
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

// GetRequest handles GET /api/v1/procurement/requests/{requestId} (buyer).
func (h *Handler) GetRequest(w http.ResponseWriter, r *http.Request) {
	c, ok := ctxFrom(r)
	if !ok {
		identityUnauthenticated(w, r)
		return
	}
	id, err := pathUUID(r, "requestId")
	if err != nil {
		platformhttp.WriteProblem(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Validation failed", err.Error())
		return
	}
	out, err := h.svc.GetRequest(r.Context(), c, id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	platformhttp.WriteJSON(w, http.StatusOK, out)
}

// CancelRequest handles PATCH /api/v1/procurement/requests/{requestId}/cancel (buyer).
func (h *Handler) CancelRequest(w http.ResponseWriter, r *http.Request) {
	c, ok := ctxFrom(r)
	if !ok {
		identityUnauthenticated(w, r)
		return
	}
	id, err := pathUUID(r, "requestId")
	if err != nil {
		platformhttp.WriteProblem(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Validation failed", err.Error())
		return
	}
	out, err := h.svc.CancelRequest(r.Context(), c, id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	platformhttp.WriteJSON(w, http.StatusOK, out)
}

// ListIncoming handles GET /api/v1/procurement/incoming (supplier).
func (h *Handler) ListIncoming(w http.ResponseWriter, r *http.Request) {
	c, ok := ctxFrom(r)
	if !ok {
		identityUnauthenticated(w, r)
		return
	}
	limit, offset := parsePagination(r)
	items, total, err := h.svc.ListIncoming(r.Context(), c, limit, offset)
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

// GetIncoming handles GET /api/v1/procurement/incoming/{requestId} (supplier).
func (h *Handler) GetIncoming(w http.ResponseWriter, r *http.Request) {
	c, ok := ctxFrom(r)
	if !ok {
		identityUnauthenticated(w, r)
		return
	}
	id, err := pathUUID(r, "requestId")
	if err != nil {
		platformhttp.WriteProblem(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Validation failed", err.Error())
		return
	}
	out, err := h.svc.GetIncoming(r.Context(), c, id)
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
	h.log.Error().Err(err).Msg("procurement request failed")
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