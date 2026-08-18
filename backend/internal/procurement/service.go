// Package procurement implements the buyer procurement workflow: a persistent
// server-side cart (procurement_carts / procurement_cart_items) and per-supplier
// purchase requests (rfqs / rfq_items, the approved domain model's RFQ).
//
// Cart lines hold an immutable PRICE SNAPSHOT (BIGINT minor units + CHAR(3)
// currency, see platform/money) taken when the product is first added. Adding
// the same product again merges quantities at the original snapshot; a later
// supplier price change never silently alters a line. Submitting the cart is a
// single transaction: it groups items by supplier, creates one purchase request
// per supplier with immutable item snapshots and server-computed totals, clears
// the submitted items, writes audit events and a procurement.request.submitted
// outbox event, and records an idempotency key so retries never duplicate
// requests. Cart/request access is tenant-scoped through RLS; cart management
// is buyer-only and viewing incoming requests is supplier-only.
package procurement

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/netip"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"tirek/backend/internal/platform/db"
	"tirek/backend/internal/platform/db/gen"
	"tirek/backend/internal/platform/money"
)

// Error is a domain error with an HTTP mapping and a stable machine code.
type Error struct {
	Code       string
	HTTPStatus int
	Msg        string
}

func (e *Error) Error() string { return e.Msg }

var (
	ErrNotFound            = &Error{Code: "NOT_FOUND", HTTPStatus: 404, Msg: "resource not found"}
	ErrForbidden           = &Error{Code: "FORBIDDEN", HTTPStatus: 403, Msg: "insufficient permissions"}
	ErrOrgTypeBuyer        = &Error{Code: "ORG_TYPE_MISMATCH", HTTPStatus: 422, Msg: "procurement is only available to buyer organizations"}
	ErrOrgTypeSupplier     = &Error{Code: "ORG_TYPE_MISMATCH", HTTPStatus: 422, Msg: "incoming purchase requests are only available to supplier organizations"}
	ErrCartEmpty           = &Error{Code: "CART_EMPTY", HTTPStatus: 422, Msg: "the cart is empty"}
	ErrCartItemNotFound    = &Error{Code: "CART_ITEM_NOT_FOUND", HTTPStatus: 404, Msg: "cart item not found"}
	ErrInvalidQuantity     = &Error{Code: "INVALID_QUANTITY", HTTPStatus: 422, Msg: "quantity must be a positive integer"}
	ErrProductUnavailable  = &Error{Code: "PRODUCT_UNAVAILABLE", HTTPStatus: 422, Msg: "product is not available for purchase"}
	ErrCurrencyMismatch    = &Error{Code: "CURRENCY_MISMATCH", HTTPStatus: 422, Msg: "all cart items must share the same currency"}
	ErrIdempotencyConflict = &Error{Code: "IDEMPOTENCY_CONFLICT", HTTPStatus: 422, Msg: "idempotency key was already used with a different request"}
	ErrIdempotencyInFlight = &Error{Code: "REQUEST_IN_FLIGHT", HTTPStatus: 409, Msg: "a submission with this idempotency key is already in progress"}
	ErrCannotCancel        = &Error{Code: "CANNOT_CANCEL", HTTPStatus: 409, Msg: "only submitted purchase requests can be cancelled"}
	ErrInsufficientStock   = &Error{Code: "QUANTITY_EXCEEDS_MINIMUM", HTTPStatus: 422, Msg: "quantity is below the product minimum"}
)

// Ctx carries the authenticated request context into the service.
type Ctx struct {
	UserID    uuid.UUID
	OrgID     uuid.UUID
	Role      string
	IP        *netip.Addr
	UserAgent string
}

// RequestStatus is the purchase request lifecycle. Requests are born
// `submitted` (creation and sending happen atomically on cart submission);
// `pending` is reserved for a future draft flow. Only a submitted request can
// be cancelled; cancellation is buyer-only.
const (
	RequestStatusSubmitted = "submitted"
	RequestStatusCancelled = "cancelled"
)

// submitEndpoint is the idempotency endpoint key for cart submission.
const submitEndpoint = "POST /v1/procurement/submit"

// CartItem is a single cart line with its price snapshot.
type CartItem struct {
	CartItemID     uuid.UUID `json:"cart_item_id"`
	CartID         uuid.UUID `json:"cart_id"`
	ProductID      uuid.UUID `json:"product_id"`
	SupplierOrgID  uuid.UUID `json:"supplier_org_id"`
	SupplierName   string    `json:"supplier_name"`
	ProductName    string    `json:"product_name"`
	Quantity       int       `json:"quantity"`
	Unit           string    `json:"unit"`
	UnitPriceMinor int64     `json:"unit_price_minor"`
	Currency       string    `json:"currency"`
	LineTotalMinor int64     `json:"line_total_minor"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// SupplierGroup groups cart items by supplier with a server-computed total.
type SupplierGroup struct {
	SupplierOrgID uuid.UUID   `json:"supplier_org_id"`
	SupplierName  string      `json:"supplier_name"`
	Items         []CartItem  `json:"items"`
	TotalMinor    int64       `json:"total_minor"`
	Currency      string      `json:"currency"`
}

// Cart is the buyer's active cart. The cart is single-currency (the currency of
// the first added product); groups and the overall total are unambiguous.
type Cart struct {
	CartID      uuid.UUID       `json:"cart_id"`
	OrgID       uuid.UUID       `json:"org_id"`
	Status      string          `json:"status"`
	Groups      []SupplierGroup `json:"groups"`
	TotalMinor  int64           `json:"total_minor"`
	Currency    string          `json:"currency"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// RequestItem is an immutable purchase request line snapshot.
type RequestItem struct {
	RequestItemID  uuid.UUID `json:"request_item_id"`
	RequestID      uuid.UUID `json:"request_id"`
	ProductID      uuid.UUID `json:"product_id"`
	ProductName    string    `json:"product_name"`
	SKU            string    `json:"sku,omitempty"`
	Quantity       int       `json:"quantity"`
	Unit           string    `json:"unit"`
	UnitPriceMinor int64     `json:"unit_price_minor"`
	Currency       string    `json:"currency"`
	LineTotalMinor int64     `json:"line_total_minor"`
}

// PurchaseRequest is a buyer→supplier purchase request (RFQ).
type PurchaseRequest struct {
	RequestID     uuid.UUID      `json:"request_id"`
	OrgID         uuid.UUID      `json:"org_id"`
	SupplierOrgID uuid.UUID      `json:"supplier_org_id"`
	SupplierName  string         `json:"supplier_name,omitempty"`
	BuyerName     string         `json:"buyer_name,omitempty"`
	Number        string         `json:"number"`
	Status        string         `json:"status"`
	Currency      string         `json:"currency"`
	TotalMinor    int64          `json:"total_minor"`
	Items         []RequestItem  `json:"items"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

// SubmitResult is returned by Submit and replayed for idempotent retries.
type SubmitResult struct {
	Requests []PurchaseRequest `json:"requests"`
}

// Service implements the procurement module operations.
type Service struct {
	pool *pgxpool.Pool
	log  zerolog.Logger
}

func NewService(pool *pgxpool.Pool, log zerolog.Logger) *Service {
	return &Service{pool: pool, log: log}
}

// GetCart returns the buyer's active cart grouped by supplier with totals.
func (s *Service) GetCart(ctx context.Context, c Ctx) (*Cart, error) {
	var out Cart
	err := db.WithTx(ctx, s.pool, c.OrgID.String(), c.Role, func(tx pgx.Tx) error {
		q := gen.New(tx)
		if err := s.requireBuyer(ctx, q, c.UserID, c.OrgID); err != nil {
			return err
		}
		cart, err := s.activeCart(ctx, q, c.OrgID)
		if err != nil {
			return err
		}
		loaded, err := s.loadCart(ctx, q, cart)
		if err != nil {
			return err
		}
		out = *loaded
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// AddItem adds a purchasable (active) product to the cart at the current price
// snapshot. Adding the same product again merges quantities and keeps the
// original snapshot price. The whole cart must stay single-currency.
func (s *Service) AddItem(ctx context.Context, c Ctx, productID uuid.UUID, quantity int) (*Cart, error) {
	var out Cart
	err := db.WithTx(ctx, s.pool, c.OrgID.String(), c.Role, func(tx pgx.Tx) error {
		q := gen.New(tx)
		if err := s.requireBuyer(ctx, q, c.UserID, c.OrgID); err != nil {
			return err
		}
		cart, err := s.activeCart(ctx, q, c.OrgID)
		if err != nil {
			return err
		}
		prod, err := q.GetPurchasableProduct(ctx, productID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrProductUnavailable
			}
			return err
		}
		if quantity < 1 {
			return ErrInvalidQuantity
		}
		if quantity < int(prod.MinOrderQty) {
			return ErrInsufficientStock
		}
		existing, err := q.ListCartItemsByCart(ctx, cart.CartID)
		if err != nil {
			return err
		}
		for _, it := range existing {
			if strings.TrimSpace(it.Currency) != strings.TrimSpace(prod.Currency) {
				return ErrCurrencyMismatch
			}
		}
		if _, err := q.UpsertCartItem(ctx, gen.UpsertCartItemParams{
			CartID: cart.CartID, OrgID: c.OrgID, ProductID: prod.ProductID,
			SupplierOrgID: prod.OrgID, ProductName: prod.Name, Quantity: int32(quantity),
			Unit: prod.Unit, UnitPriceMinor: prod.UnitPriceMinor,
			Currency: strings.TrimSpace(prod.Currency),
		}); err != nil {
			return err
		}
		if err := insertAudit(ctx, q, c, "procurement.cart_item_added", "cart_item",
			cart.CartID, nil, map[string]any{
				"product_id": prod.ProductID, "quantity": quantity, "cart_id": cart.CartID,
			}); err != nil {
			return err
		}
		loaded, err := s.loadCart(ctx, q, cart)
		if err != nil {
			return err
		}
		out = *loaded
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateItemQuantity changes the quantity of a cart line.
func (s *Service) UpdateItemQuantity(ctx context.Context, c Ctx, cartItemID uuid.UUID, quantity int) (*Cart, error) {
	var out Cart
	err := db.WithTx(ctx, s.pool, c.OrgID.String(), c.Role, func(tx pgx.Tx) error {
		q := gen.New(tx)
		if err := s.requireBuyer(ctx, q, c.UserID, c.OrgID); err != nil {
			return err
		}
		cart, err := s.activeCart(ctx, q, c.OrgID)
		if err != nil {
			return err
		}
		if quantity < 1 {
			return ErrInvalidQuantity
		}
		if _, err := q.UpdateCartItemQuantity(ctx, gen.UpdateCartItemQuantityParams{
			CartItemID: cartItemID, CartID: cart.CartID, Quantity: int32(quantity),
		}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrCartItemNotFound
			}
			return err
		}
		if err := insertAudit(ctx, q, c, "procurement.cart_item_updated", "cart_item",
			cartItemID, nil, map[string]any{"quantity": quantity, "cart_id": cart.CartID}); err != nil {
			return err
		}
		loaded, err := s.loadCart(ctx, q, cart)
		if err != nil {
			return err
		}
		out = *loaded
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// RemoveItem removes a cart line.
func (s *Service) RemoveItem(ctx context.Context, c Ctx, cartItemID uuid.UUID) (*Cart, error) {
	var out Cart
	err := db.WithTx(ctx, s.pool, c.OrgID.String(), c.Role, func(tx pgx.Tx) error {
		q := gen.New(tx)
		if err := s.requireBuyer(ctx, q, c.UserID, c.OrgID); err != nil {
			return err
		}
		cart, err := s.activeCart(ctx, q, c.OrgID)
		if err != nil {
			return err
		}
		n, err := q.DeleteCartItem(ctx, gen.DeleteCartItemParams{CartItemID: cartItemID, CartID: cart.CartID})
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrCartItemNotFound
		}
		if err := insertAudit(ctx, q, c, "procurement.cart_item_removed", "cart_item",
			cartItemID, nil, map[string]any{"cart_id": cart.CartID}); err != nil {
			return err
		}
		loaded, err := s.loadCart(ctx, q, cart)
		if err != nil {
			return err
		}
		out = *loaded
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// Submit converts the cart into per-supplier purchase requests in a single
// transaction. The operation is atomic and idempotent: on any validation
// failure nothing is created and the cart is unchanged; retrying with the same
// Idempotency-Key replays the stored result instead of creating duplicates.
func (s *Service) Submit(ctx context.Context, c Ctx, idempotencyKey string) (*SubmitResult, error) {
	result := &SubmitResult{}
	err := db.WithTx(ctx, s.pool, c.OrgID.String(), c.Role, func(tx pgx.Tx) error {
		q := gen.New(tx)
		if err := s.requireBuyer(ctx, q, c.UserID, c.OrgID); err != nil {
			return err
		}

		// Idempotency: claim the key before doing any work so concurrent
		// duplicates are impossible. A claimed-and-completed key replays the
		// stored response; a claimed-but-incomplete key means an in-flight
		// request. On failure the whole transaction rolls back (key included).
		if idempotencyKey != "" {
			replay, err := s.claimIdempotency(ctx, q, c.OrgID, idempotencyKey)
			if err != nil {
				return err
			}
			if replay != nil {
				*result = *replay
				return nil
			}
		}

		cart, err := q.GetActiveCartForUpdate(ctx, c.OrgID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrCartEmpty
			}
			return err
		}
		items, err := q.ListCartItemsByCartForUpdate(ctx, cart.CartID)
		if err != nil {
			return err
		}
		if len(items) == 0 {
			return ErrCartEmpty
		}

		// Validate every line is still purchasable (product still active) and
		// gather current catalog data for snapshots. Any failure aborts the
		// whole submission atomically; the cart stays untouched.
		type group struct {
			supplierOrgID uuid.UUID
			currency      string
			totalMinor    int64
			items         []gen.ProcurementCartItem
		}
		groups := make(map[uuid.UUID]*group)
		for _, it := range items {
			prod, err := q.GetPurchasableProduct(ctx, it.ProductID)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return ErrProductUnavailable
				}
				return err
			}
			if strings.TrimSpace(prod.Currency) != strings.TrimSpace(it.Currency) {
				return ErrProductUnavailable
			}
			g, ok := groups[it.SupplierOrgID]
			if !ok {
				g = &group{supplierOrgID: it.SupplierOrgID, currency: strings.TrimSpace(it.Currency)}
				groups[it.SupplierOrgID] = g
			}
			lineTotal, err := mulMinor(int64(it.Quantity), it.UnitPriceMinor)
			if err != nil {
				return err
			}
			g.totalMinor += lineTotal
			g.items = append(g.items, it)
		}

		requests := make([]PurchaseRequest, 0, len(groups))
		for _, g := range groups {
			if _, err := money.New(g.totalMinor, g.currency); err != nil {
				return ErrCurrencyMismatch
			}
			rfq, err := q.CreatePurchaseRequest(ctx, gen.CreatePurchaseRequestParams{
				OrgID: c.OrgID, SupplierOrgID: g.supplierOrgID,
				Number: requestNumber(), Currency: g.currency, TotalMinor: g.totalMinor,
				CreatedBy: pgUUID(c.UserID),
			})
			if err != nil {
				return err
			}
			request := PurchaseRequest{
				RequestID: rfq.RfqID, OrgID: rfq.OrgID, SupplierOrgID: rfq.SupplierOrgID,
				Number: rfq.Number, Status: rfq.Status, Currency: strings.TrimSpace(rfq.Currency),
				TotalMinor: rfq.TotalMinor, CreatedAt: rfq.CreatedAt, UpdatedAt: rfq.UpdatedAt,
			}
			for _, it := range g.items {
				lineTotal, err := mulMinor(int64(it.Quantity), it.UnitPriceMinor)
				if err != nil {
					return err
				}
				item, err := q.CreatePurchaseRequestItem(ctx, gen.CreatePurchaseRequestItemParams{
					RfqID: rfq.RfqID, OrgID: c.OrgID, ProductID: it.ProductID,
					Description: it.ProductName, Quantity: it.Quantity, Unit: it.Unit,
					ProductName: pgText(it.ProductName),
					UnitPriceMinor: pgtype.Int8{Int64: it.UnitPriceMinor, Valid: true},
					Currency:       pgtype.Text{String: strings.TrimSpace(it.Currency), Valid: true},
					LineTotalMinor: pgtype.Int8{Int64: lineTotal, Valid: true},
				})
				if err != nil {
					return err
				}
				request.Items = append(request.Items, RequestItem{
					RequestItemID: item.RfqItemID, RequestID: item.RfqID, ProductID: item.ProductID,
					ProductName: item.ProductName.String, Unit: item.Unit, Quantity: int(item.Quantity),
					UnitPriceMinor: item.UnitPriceMinor.Int64, Currency: strings.TrimSpace(item.Currency.String),
					LineTotalMinor: item.LineTotalMinor.Int64,
				})
			}
			if err := insertAudit(ctx, q, c, "procurement.request_submitted", "rfq", rfq.RfqID,
				nil, map[string]any{"number": rfq.Number, "supplier_org_id": g.supplierOrgID, "total_minor": rfq.TotalMinor}); err != nil {
				return err
			}
			payload, _ := json.Marshal(map[string]any{
				"request_id": rfq.RfqID, "number": rfq.Number,
				"buyer_org_id": c.OrgID, "supplier_org_id": g.supplierOrgID,
				"currency": g.currency, "total_minor": rfq.TotalMinor,
			})
			if err := q.InsertOutboxEvent(ctx, gen.InsertOutboxEventParams{
				OrgID: pgUUID(c.OrgID), Topic: "procurement.request.submitted",
				EntityID: rfq.RfqID.String(), Payload: payload,
			}); err != nil {
				return err
			}
			requests = append(requests, request)
		}

		// Only after every request was created do we clear the cart.
		if err := q.DeleteCartItems(ctx, cart.CartID); err != nil {
			return err
		}

		result.Requests = requests
		if idempotencyKey != "" {
			body, _ := json.Marshal(result)
			if err := q.CompleteIdempotencyKey(ctx, gen.CompleteIdempotencyKeyParams{
				OrgID: pgUUID(c.OrgID), Key: idempotencyKey, Endpoint: submitEndpoint,
				ResponseBody: body, ResponseCode: pgtype.Int4{Int32: 200, Valid: true},
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// ListRequests returns the buyer's own purchase requests.
func (s *Service) ListRequests(ctx context.Context, c Ctx, limit, offset int) ([]PurchaseRequest, int64, error) {
	var (
		items []PurchaseRequest
		total int64
	)
	err := db.WithTx(ctx, s.pool, c.OrgID.String(), c.Role, func(tx pgx.Tx) error {
		q := gen.New(tx)
		if err := s.requireBuyer(ctx, q, c.UserID, c.OrgID); err != nil {
			return err
		}
		n, err := q.CountPurchaseRequests(ctx, c.OrgID)
		if err != nil {
			return err
		}
		total = n
		rows, err := q.ListPurchaseRequests(ctx, gen.ListPurchaseRequestsParams{
			OrgID: c.OrgID, Limit: int32(limit), Offset: int32(offset),
		})
		if err != nil {
			return err
		}
		items = make([]PurchaseRequest, 0, len(rows))
		for _, r := range rows {
			items = append(items, PurchaseRequest{
				RequestID: r.RfqID, OrgID: r.OrgID, SupplierOrgID: r.SupplierOrgID,
				SupplierName: r.SupplierName, Number: r.Number, Status: r.Status,
				Currency: strings.TrimSpace(r.Currency), TotalMinor: r.TotalMinor,
				CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
			})
		}
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// GetRequest returns one purchase request with its items. It works for the
// buyer (own request) and the supplier (incoming request); RLS scopes the row.
func (s *Service) GetRequest(ctx context.Context, c Ctx, requestID uuid.UUID) (*PurchaseRequest, error) {
	var out PurchaseRequest
	err := db.WithTx(ctx, s.pool, c.OrgID.String(), c.Role, func(tx pgx.Tx) error {
		q := gen.New(tx)
		if err := s.requireTenantMember(ctx, q, c.UserID, c.OrgID); err != nil {
			return err
		}
		return s.loadRequest(ctx, q, requestID, &out)
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// CancelRequest cancels a buyer's own submitted purchase request.
func (s *Service) CancelRequest(ctx context.Context, c Ctx, requestID uuid.UUID) (*PurchaseRequest, error) {
	var out PurchaseRequest
	err := db.WithTx(ctx, s.pool, c.OrgID.String(), c.Role, func(tx pgx.Tx) error {
		q := gen.New(tx)
		if err := s.requireBuyer(ctx, q, c.UserID, c.OrgID); err != nil {
			return err
		}
		row, err := q.CancelPurchaseRequest(ctx, requestID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				// Distinguish "not found/foreign" from "already cancelled": an
				// UPDATE with no matching row returns no rows for both. Check
				// existence with a scoped read to give a precise error.
				if _, err := q.GetPurchaseRequest(ctx, requestID); err != nil {
					if errors.Is(err, pgx.ErrNoRows) {
						return ErrNotFound
					}
					return err
				}
				return ErrCannotCancel
			}
			return err
		}
		request := PurchaseRequest{
			RequestID: row.RfqID, OrgID: row.OrgID, SupplierOrgID: row.SupplierOrgID,
			Number: row.Number, Status: row.Status, Currency: strings.TrimSpace(row.Currency),
			TotalMinor: row.TotalMinor, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		}
		if err := insertAudit(ctx, q, c, "procurement.request_cancelled", "rfq", requestID,
			nil, map[string]any{"number": request.Number}); err != nil {
			return err
		}
		out = request
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// ListIncoming returns the supplier's incoming purchase requests.
func (s *Service) ListIncoming(ctx context.Context, c Ctx, limit, offset int) ([]PurchaseRequest, int64, error) {
	var (
		items []PurchaseRequest
		total int64
	)
	err := db.WithTx(ctx, s.pool, c.OrgID.String(), c.Role, func(tx pgx.Tx) error {
		q := gen.New(tx)
		if err := s.requireSupplier(ctx, q, c.UserID, c.OrgID); err != nil {
			return err
		}
		n, err := q.CountIncomingRequests(ctx, c.OrgID)
		if err != nil {
			return err
		}
		total = n
		rows, err := q.ListIncomingRequests(ctx, gen.ListIncomingRequestsParams{
			SupplierOrgID: c.OrgID, Limit: int32(limit), Offset: int32(offset),
		})
		if err != nil {
			return err
		}
		items = make([]PurchaseRequest, 0, len(rows))
		for _, r := range rows {
			items = append(items, PurchaseRequest{
				RequestID: r.RfqID, OrgID: r.OrgID, SupplierOrgID: r.SupplierOrgID,
				BuyerName: r.BuyerName, Number: r.Number, Status: r.Status,
				Currency: strings.TrimSpace(r.Currency), TotalMinor: r.TotalMinor,
				CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
			})
		}
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// GetIncoming returns one incoming purchase request (supplier tenant; RLS
// guarantees it is addressed to the caller's organization).
func (s *Service) GetIncoming(ctx context.Context, c Ctx, requestID uuid.UUID) (*PurchaseRequest, error) {
	var out PurchaseRequest
	err := db.WithTx(ctx, s.pool, c.OrgID.String(), c.Role, func(tx pgx.Tx) error {
		q := gen.New(tx)
		if err := s.requireSupplier(ctx, q, c.UserID, c.OrgID); err != nil {
			return err
		}
		if err := s.loadRequest(ctx, q, requestID, &out); err != nil {
			return err
		}
		if org, err := q.GetOrganizationByID(ctx, out.OrgID); err == nil {
			out.BuyerName = org.Name
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// ---------------------------------------------------------------------------
// internal helpers
// ---------------------------------------------------------------------------

// activeCart returns the single active cart for the org, creating it on first
// use. The unique partial index makes concurrent first-uses safe.
func (s *Service) activeCart(ctx context.Context, q *gen.Queries, orgID uuid.UUID) (gen.ProcurementCart, error) {
	cart, err := q.GetOrCreateActiveCart(ctx, orgID)
	if err == nil {
		return cart, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return gen.ProcurementCart{}, err
	}
	cart, err = q.GetActiveCart(ctx, orgID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return gen.ProcurementCart{}, ErrCartEmpty
		}
		return gen.ProcurementCart{}, err
	}
	return cart, nil
}

// loadCart builds the grouped cart view with server-computed totals.
func (s *Service) loadCart(ctx context.Context, q *gen.Queries, cart gen.ProcurementCart) (*Cart, error) {
	rows, err := q.ListCartItemsByCart(ctx, cart.CartID)
	if err != nil {
		return nil, err
	}
	loaded := &Cart{
		CartID: cart.CartID, OrgID: cart.OrgID, Status: cart.Status,
		CreatedAt: cart.CreatedAt, UpdatedAt: cart.UpdatedAt,
		Groups: make([]SupplierGroup, 0),
	}
	var overall int64
	index := make(map[uuid.UUID]int)
	for _, r := range rows {
		lineTotal := r.UnitPriceMinor * int64(r.Quantity)
		overall += lineTotal
		if loaded.Currency == "" {
			loaded.Currency = strings.TrimSpace(r.Currency)
		}
		gi, ok := index[r.SupplierOrgID]
		if !ok {
			gi = len(loaded.Groups)
			index[r.SupplierOrgID] = gi
			loaded.Groups = append(loaded.Groups, SupplierGroup{
				SupplierOrgID: r.SupplierOrgID, SupplierName: r.SupplierName,
				Currency: strings.TrimSpace(r.Currency),
			})
		}
		loaded.Groups[gi].Items = append(loaded.Groups[gi].Items, CartItem{
			CartItemID: r.CartItemID, CartID: r.CartID, ProductID: r.ProductID,
			SupplierOrgID: r.SupplierOrgID, SupplierName: r.SupplierName,
			ProductName: r.ProductName, Quantity: int(r.Quantity), Unit: r.Unit,
			UnitPriceMinor: r.UnitPriceMinor, Currency: strings.TrimSpace(r.Currency),
			LineTotalMinor: lineTotal, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		})
		loaded.Groups[gi].TotalMinor += lineTotal
	}
	loaded.TotalMinor = overall
	return loaded, nil
}

// loadRequest loads a request header + its immutable items into out. RLS scopes
// the query to the caller's tenant (buyer own / supplier incoming).
func (s *Service) loadRequest(ctx context.Context, q *gen.Queries, requestID uuid.UUID, out *PurchaseRequest) error {
	header, err := q.GetPurchaseRequest(ctx, requestID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	items, err := q.GetPurchaseRequestItems(ctx, requestID)
	if err != nil {
		return err
	}
	*out = PurchaseRequest{
		RequestID: header.RfqID, OrgID: header.OrgID, SupplierOrgID: header.SupplierOrgID,
		SupplierName: header.SupplierName, Number: header.Number, Status: header.Status,
		Currency: strings.TrimSpace(header.Currency), TotalMinor: header.TotalMinor,
		CreatedAt: header.CreatedAt, UpdatedAt: header.UpdatedAt,
	}
	out.Items = make([]RequestItem, 0, len(items))
	for _, it := range items {
		out.Items = append(out.Items, RequestItem{
			RequestItemID: it.RfqItemID, RequestID: it.RfqID, ProductID: it.ProductID,
			ProductName: it.ProductName.String, SKU: it.Sku.String, Quantity: int(it.Quantity),
			Unit: it.Unit, UnitPriceMinor: it.UnitPriceMinor.Int64,
			Currency: strings.TrimSpace(it.Currency.String), LineTotalMinor: it.LineTotalMinor.Int64,
		})
	}
	return nil
}

// claimIdempotency claims idempotencyKey for submit and reports whether the
// caller should replay a stored result (nil = proceed with a fresh submission).
func (s *Service) claimIdempotency(ctx context.Context, q *gen.Queries, orgID uuid.UUID, key string) (*SubmitResult, error) {
	claimErr := func() error {
		_, err := q.ClaimIdempotencyKey(ctx, gen.ClaimIdempotencyKeyParams{
			OrgID: pgUUID(orgID), Key: key, Endpoint: submitEndpoint,
			RequestHash: submitHash(),
		})
		return err
	}()
	if claimErr == nil {
		// We own the claim; proceed with the fresh submission.
		return nil, nil
	}
	if !errors.Is(claimErr, pgx.ErrNoRows) {
		return nil, claimErr
	}
	// The key already exists: another request claimed it (concurrent, or a
	// committed replay).
	existing, err := q.GetIdempotencyKey(ctx, gen.GetIdempotencyKeyParams{
		OrgID: pgUUID(orgID), Key: key, Endpoint: submitEndpoint,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if existing.RequestHash != submitHash() {
		return nil, ErrIdempotencyConflict
	}
	if len(existing.ResponseBody) == 0 {
		return nil, ErrIdempotencyInFlight
	}
	var result SubmitResult
	if err := json.Unmarshal(existing.ResponseBody, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// requestNumber generates a short human-friendly purchase request number.
func requestNumber() string {
	id := uuid.NewString()
	return "PR-" + strings.ToUpper(id[:8])
}

// submitHash is the canonical request hash for the (bodiless) submit endpoint.
func submitHash() string {
	sum := sha256.Sum256([]byte(submitEndpoint))
	return hex.EncodeToString(sum[:])
}

// mulMinor multiplies quantity × unit price with overflow protection.
func mulMinor(quantity, unitPriceMinor int64) (int64, error) {
	if quantity < 0 || unitPriceMinor < 0 {
		return 0, ErrInvalidQuantity
	}
	if quantity != 0 && unitPriceMinor > (1<<63-1)/quantity {
		return 0, errors.New("price overflow")
	}
	return quantity * unitPriceMinor, nil
}

// requireTenantMember verifies the caller is a member of the active org.
func (s *Service) requireTenantMember(ctx context.Context, q *gen.Queries, userID, orgID uuid.UUID) error {
	if _, err := q.GetMembershipByUserOrg(ctx, gen.GetMembershipByUserOrgParams{UserID: userID, OrgID: orgID}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrForbidden
		}
		return err
	}
	return nil
}

// requireBuyer verifies tenant membership and the organization type.
func (s *Service) requireBuyer(ctx context.Context, q *gen.Queries, userID, orgID uuid.UUID) error {
	if err := s.requireTenantMember(ctx, q, userID, orgID); err != nil {
		return err
	}
	org, err := q.GetOrganizationByID(ctx, orgID)
	if err != nil {
		return err
	}
	if org.Type != "buyer" {
		return ErrOrgTypeBuyer
	}
	return nil
}

// requireSupplier verifies tenant membership and the organization type.
func (s *Service) requireSupplier(ctx context.Context, q *gen.Queries, userID, orgID uuid.UUID) error {
	if err := s.requireTenantMember(ctx, q, userID, orgID); err != nil {
		return err
	}
	org, err := q.GetOrganizationByID(ctx, orgID)
	if err != nil {
		return err
	}
	if org.Type != "supplier" {
		return ErrOrgTypeSupplier
	}
	return nil
}

// insertAudit records an audit log row in the same transaction as the mutation.
func insertAudit(ctx context.Context, q *gen.Queries, c Ctx, action, entityType string, entityID uuid.UUID, before, after any) error {
	var beforeJSON, afterJSON []byte
	var err error
	if before != nil {
		if beforeJSON, err = json.Marshal(before); err != nil {
			return err
		}
	}
	if afterJSON, err = json.Marshal(after); err != nil {
		return err
	}
	return q.InsertAuditLog(ctx, gen.InsertAuditLogParams{
		OrgID:      pgUUID(c.OrgID),
		ActorID:    pgUUID(c.UserID),
		Action:     action,
		EntityType: entityType,
		EntityID:   pgText(entityID.String()),
		Before:     beforeJSON,
		After:      afterJSON,
		Ip:         c.IP,
		UserAgent:  pgText(c.UserAgent),
	})
}

func pgUUID(u uuid.UUID) pgtype.UUID { return pgtype.UUID{Bytes: u, Valid: true} }
func pgText(s string) pgtype.Text    { return pgtype.Text{String: s, Valid: s != ""} }