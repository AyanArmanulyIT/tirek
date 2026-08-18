// Package catalog implements the product catalog module: supplier-owned SKUs
// (catalog_products), per-supplier categories (catalog_categories), a single
// current price per product (catalog_prices), and buyer-side marketplace
// browsing of active products.
//
// Per the approved domain model there is no separate Catalog entity — products
// belong directly to the supplier organization via org_id. Money is BIGINT
// minor units + CHAR(3) currency (see platform/money). Every operation is
// tenant-scoped through RLS; product management is supplier-only, while
// browsing the marketplace is buyer-only (both enforced at the service and by
// the database triggers/policies in 0004_catalog_products.sql).
package catalog

import (
	"context"
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
	ErrNotFound          = &Error{Code: "NOT_FOUND", HTTPStatus: 404, Msg: "resource not found"}
	ErrForbidden         = &Error{Code: "FORBIDDEN", HTTPStatus: 403, Msg: "insufficient permissions"}
	ErrOrgTypeSupplier   = &Error{Code: "ORG_TYPE_MISMATCH", HTTPStatus: 422, Msg: "product management is only available to supplier organizations"}
	ErrOrgTypeBuyer      = &Error{Code: "ORG_TYPE_MISMATCH", HTTPStatus: 422, Msg: "browsing the marketplace catalog is only available to buyer organizations"}
	ErrCategoryNotFound  = &Error{Code: "CATEGORY_NOT_FOUND", HTTPStatus: 404, Msg: "category not found"}
	ErrInvalidPrice      = &Error{Code: "INVALID_PRICE", HTTPStatus: 422, Msg: "invalid price"}
	ErrArchivedProduct   = &Error{Code: "PRODUCT_ARCHIVED", HTTPStatus: 409, Msg: "archived products cannot be modified"}
	ErrCategoryNameTaken = &Error{Code: "CATEGORY_NAME_TAKEN", HTTPStatus: 409, Msg: "a category with this name already exists"}
)

// Ctx carries the authenticated request context into the service.
type Ctx struct {
	UserID    uuid.UUID
	OrgID     uuid.UUID
	Role      string
	IP        *netip.Addr
	UserAgent string
}

// ProductStatus is the product lifecycle.
const (
	StatusDraft    = "draft"
	StatusActive   = "active"
	StatusArchived = "archived"
)

// Price is a single current price point (BIGINT minor units + currency).
type Price struct {
	AmountMinor int64  `json:"amount_minor"`
	Currency    string `json:"currency"`
}

// Product is the domain representation of a product.
type Product struct {
	ProductID   uuid.UUID  `json:"product_id"`
	OrgID       uuid.UUID  `json:"org_id"`
	CategoryID  *uuid.UUID `json:"category_id,omitempty"`
	Name        string     `json:"name"`
	SKU         string     `json:"sku,omitempty"`
	Description string     `json:"description,omitempty"`
	Unit        string     `json:"unit"`
	Status      string     `json:"status"`
	VatRateBps  int32      `json:"vat_rate_bps"`
	ImageURL    string     `json:"image_url,omitempty"`
	MinOrderQty int32      `json:"min_order_qty"`
	Price       *Price     `json:"price,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// BrowseItem is a marketplace search result (buyer view of an active product).
type BrowseItem struct {
	ProductID    uuid.UUID  `json:"product_id"`
	OrgID        uuid.UUID  `json:"org_id"`
	CategoryID   *uuid.UUID `json:"category_id,omitempty"`
	Name         string     `json:"name"`
	SKU          string     `json:"sku,omitempty"`
	Description  string     `json:"description,omitempty"`
	Unit         string     `json:"unit"`
	VatRateBps   int32      `json:"vat_rate_bps"`
	ImageURL     string     `json:"image_url,omitempty"`
	MinOrderQty  int32      `json:"min_order_qty"`
	Price        Price      `json:"price"`
	SupplierName string     `json:"supplier_name"`
	CategoryName string     `json:"category_name,omitempty"`
}

// ProductInput is the validated input for creating or updating a product.
// Status defaults to active at the HTTP layer; category_id is optional.
type ProductInput struct {
	Name        string
	SKU         string
	Description string
	Unit        string
	Status      string
	CategoryID  *uuid.UUID
	ImageURL    string
	MinOrderQty int32
	VatRateBps  int32
	PriceMinor  int64
	Currency    string
}

// CategoryInput is the validated input for creating a category.
type CategoryInput struct {
	Name string
}

// Category is the domain representation of a category.
type Category struct {
	CategoryID uuid.UUID `json:"category_id"`
	OrgID      uuid.UUID `json:"org_id"`
	Name       string    `json:"name"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// Service implements the catalog module operations.
type Service struct {
	pool *pgxpool.Pool
	log  zerolog.Logger
}

func NewService(pool *pgxpool.Pool, log zerolog.Logger) *Service {
	return &Service{pool: pool, log: log}
}

// Create creates a product (with its current price) for the caller's supplier
// organization. Fails with ErrOrgTypeSupplier for buyer organizations.
func (s *Service) Create(ctx context.Context, c Ctx, in ProductInput) (*Product, error) {
	var out Product
	err := db.WithTx(ctx, s.pool, c.OrgID.String(), c.Role, func(tx pgx.Tx) error {
		q := gen.New(tx)
		if err := s.requireSupplier(ctx, q, c.UserID, c.OrgID); err != nil {
			return err
		}
		if err := s.checkCategory(ctx, q, in.CategoryID); err != nil {
			return err
		}
		if err := validatePrice(in.PriceMinor, in.Currency); err != nil {
			return err
		}
		row, err := q.CreateCatalogProduct(ctx, gen.CreateCatalogProductParams{
			OrgID: c.OrgID, CategoryID: pgOptUUID(in.CategoryID), Name: in.Name,
			Sku: pgText(in.SKU), Description: pgText(in.Description), Unit: in.Unit,
			Status: in.Status, VatRateBps: in.VatRateBps, ImageS3Key: pgText(in.ImageURL),
			MinOrderQty: in.MinOrderQty,
		})
		if err != nil {
			return err
		}
		price, err := q.UpsertCatalogPrice(ctx, gen.UpsertCatalogPriceParams{
			ProductID: row.ProductID, OrgID: c.OrgID, Currency: in.Currency, UnitPriceMinor: in.PriceMinor,
		})
		if err != nil {
			return err
		}
		product := productFromRow(row.ProductID, row.OrgID, row.CategoryID, row.Name, row.Sku,
			row.Description, row.Unit, row.Status, row.VatRateBps, row.ImageS3Key,
			row.MinOrderQty, row.CreatedAt, row.UpdatedAt)
		product.Price = priceFromRow(price)
		if err := insertAudit(ctx, q, c, "product.created", "product", row.ProductID, nil, product); err != nil {
			return err
		}
		out = product
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// Get returns one product by id (tenant-scoped, supplier-only management).
func (s *Service) Get(ctx context.Context, c Ctx, productID uuid.UUID) (*Product, error) {
	var out Product
	err := db.WithTx(ctx, s.pool, c.OrgID.String(), c.Role, func(tx pgx.Tx) error {
		q := gen.New(tx)
		if err := s.requireSupplier(ctx, q, c.UserID, c.OrgID); err != nil {
			return err
		}
		row, err := q.GetCatalogProductByID(ctx, productID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		product := productFromRow(row.ProductID, row.OrgID, row.CategoryID, row.Name, row.Sku,
			row.Description, row.Unit, row.Status, row.VatRateBps, row.ImageS3Key,
			row.MinOrderQty, row.CreatedAt, row.UpdatedAt)
		if p, err := q.GetCatalogPrice(ctx, productID); err == nil {
			product.Price = priceFromRow(p)
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		out = product
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// List returns the caller's products with offset pagination.
func (s *Service) List(ctx context.Context, c Ctx, limit, offset int) ([]Product, int64, error) {
	var (
		items []Product
		total int64
	)
	err := db.WithTx(ctx, s.pool, c.OrgID.String(), c.Role, func(tx pgx.Tx) error {
		q := gen.New(tx)
		if err := s.requireSupplier(ctx, q, c.UserID, c.OrgID); err != nil {
			return err
		}
		n, err := q.CountCatalogProducts(ctx, c.OrgID)
		if err != nil {
			return err
		}
		total = n
		rows, err := q.ListCatalogProducts(ctx, gen.ListCatalogProductsParams{
			OrgID: c.OrgID, Limit: int32(limit), Offset: int32(offset),
		})
		if err != nil {
			return err
		}
		items = make([]Product, 0, len(rows))
		for _, s := range rows {
			product := productFromRow(s.ProductID, s.OrgID, s.CategoryID, s.Name, s.Sku,
				s.Description, s.Unit, s.Status, s.VatRateBps, s.ImageS3Key,
				s.MinOrderQty, s.CreatedAt, s.UpdatedAt)
			if p, err := q.GetCatalogPrice(ctx, s.ProductID); err == nil {
				product.Price = priceFromRow(p)
			} else if !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			items = append(items, product)
		}
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// Update updates a product (full update, supplier-only). Archived products are
// terminal and cannot be modified. Price changes upsert the current price row.
func (s *Service) Update(ctx context.Context, c Ctx, productID uuid.UUID, in ProductInput) (*Product, error) {
	var out Product
	err := db.WithTx(ctx, s.pool, c.OrgID.String(), c.Role, func(tx pgx.Tx) error {
		q := gen.New(tx)
		if err := s.requireSupplier(ctx, q, c.UserID, c.OrgID); err != nil {
			return err
		}
		before, err := q.GetCatalogProductByID(ctx, productID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		if before.Status == StatusArchived {
			return ErrArchivedProduct
		}
		if err := s.checkCategory(ctx, q, in.CategoryID); err != nil {
			return err
		}
		if err := validatePrice(in.PriceMinor, in.Currency); err != nil {
			return err
		}
		row, err := q.UpdateCatalogProduct(ctx, gen.UpdateCatalogProductParams{
			ProductID: productID, Name: in.Name, CategoryID: pgOptUUID(in.CategoryID),
			Sku: pgText(in.SKU), Description: pgText(in.Description), Unit: in.Unit,
			Status: in.Status, VatRateBps: in.VatRateBps, ImageS3Key: pgText(in.ImageURL),
			MinOrderQty: in.MinOrderQty,
		})
		if err != nil {
			return err
		}
		if _, err := q.UpsertCatalogPrice(ctx, gen.UpsertCatalogPriceParams{
			ProductID: productID, OrgID: c.OrgID, Currency: in.Currency, UnitPriceMinor: in.PriceMinor,
		}); err != nil {
			return err
		}
		product := productFromRow(row.ProductID, row.OrgID, row.CategoryID, row.Name, row.Sku,
			row.Description, row.Unit, row.Status, row.VatRateBps, row.ImageS3Key,
			row.MinOrderQty, row.CreatedAt, row.UpdatedAt)
		product.Price = &Price{AmountMinor: in.PriceMinor, Currency: in.Currency}
		action := "product.updated"
		if before.Status != StatusArchived && in.Status == StatusArchived {
			action = "product.archived"
		}
		if err := insertAudit(ctx, q, c, action, "product", productID, productFromRow(before.ProductID, before.OrgID, before.CategoryID, before.Name, before.Sku,
			before.Description, before.Unit, before.Status, before.VatRateBps, before.ImageS3Key,
			before.MinOrderQty, before.CreatedAt, before.UpdatedAt), product); err != nil {
			return err
		}
		out = product
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateCategory creates a category for the caller's supplier organization.
func (s *Service) CreateCategory(ctx context.Context, c Ctx, in CategoryInput) (*Category, error) {
	var out Category
	err := db.WithTx(ctx, s.pool, c.OrgID.String(), c.Role, func(tx pgx.Tx) error {
		q := gen.New(tx)
		if err := s.requireSupplier(ctx, q, c.UserID, c.OrgID); err != nil {
			return err
		}
		row, err := q.CreateCatalogCategory(ctx, gen.CreateCatalogCategoryParams{OrgID: c.OrgID, Name: in.Name})
		if err != nil {
			if db.IsUniqueViolation(err) {
				return ErrCategoryNameTaken
			}
			return err
		}
		category := categoryFromRow(row.CategoryID, row.OrgID, row.Name, row.CreatedAt, row.UpdatedAt)
		if err := insertAudit(ctx, q, c, "category.created", "category", row.CategoryID, nil, category); err != nil {
			return err
		}
		out = category
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// ListCategories returns the caller's categories.
func (s *Service) ListCategories(ctx context.Context, c Ctx, limit, offset int) ([]Category, int64, error) {
	var (
		items []Category
		total int64
	)
	err := db.WithTx(ctx, s.pool, c.OrgID.String(), c.Role, func(tx pgx.Tx) error {
		q := gen.New(tx)
		if err := s.requireSupplier(ctx, q, c.UserID, c.OrgID); err != nil {
			return err
		}
		n, err := q.CountCatalogCategories(ctx, c.OrgID)
		if err != nil {
			return err
		}
		total = n
		rows, err := q.ListCatalogCategories(ctx, c.OrgID)
		if err != nil {
			return err
		}
		items = make([]Category, 0, len(rows))
		for _, s := range rows {
			items = append(items, categoryFromRow(s.CategoryID, s.OrgID, s.Name, s.CreatedAt, s.UpdatedAt))
		}
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// Browse searches the active marketplace catalog for a buyer organization.
// q is a case-insensitive name search; supplierID and categoryID are optional
// filters. RLS guarantees only active products of any supplier are visible.
func (s *Service) Browse(ctx context.Context, c Ctx, q string, supplierID, categoryID *uuid.UUID, limit, offset int) ([]BrowseItem, int64, error) {
	var (
		items []BrowseItem
		total int64
	)
	err := db.WithTx(ctx, s.pool, c.OrgID.String(), c.Role, func(tx pgx.Tx) error {
		queries := gen.New(tx)
		if err := s.requireBuyer(ctx, queries, c.UserID, c.OrgID); err != nil {
			return err
		}
		filter := gen.CountBrowseCatalogProductsParams{
			Q: pgtype.Text{String: q, Valid: true},
		}
		if supplierID != nil {
			filter.SupplierID = pgUUID(*supplierID)
		}
		if categoryID != nil {
			filter.CategoryID = pgUUID(*categoryID)
		}
		n, err := queries.CountBrowseCatalogProducts(ctx, filter)
		if err != nil {
			return err
		}
		total = n
		rows, err := queries.BrowseCatalogProducts(ctx, gen.BrowseCatalogProductsParams{
			Q: pgtype.Text{String: q, Valid: true},
			SupplierID: pgOptUUID(supplierID),
			CategoryID: pgOptUUID(categoryID),
			Limit:      int32(limit), Offset: int32(offset),
		})
		if err != nil {
			return err
		}
		items = make([]BrowseItem, 0, len(rows))
		for _, s := range rows {
			items = append(items, browseItemFromRow(s))
		}
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// BrowseCategories lists the categories visible to a buyer organization in the
// marketplace (RLS exposes all suppliers' categories to buyers). Used to build
// the category filter on the marketplace page.
func (s *Service) BrowseCategories(ctx context.Context, c Ctx) ([]Category, int64, error) {
	var (
		items []Category
		total int64
	)
	err := db.WithTx(ctx, s.pool, c.OrgID.String(), c.Role, func(tx pgx.Tx) error {
		q := gen.New(tx)
		if err := s.requireBuyer(ctx, q, c.UserID, c.OrgID); err != nil {
			return err
		}
		n, err := q.CountCatalogCategoriesForBrowse(ctx)
		if err != nil {
			return err
		}
		total = n
		rows, err := q.ListCatalogCategoriesForBrowse(ctx)
		if err != nil {
			return err
		}
		items = make([]Category, 0, len(rows))
		for _, s := range rows {
			items = append(items, Category{CategoryID: s.CategoryID, OrgID: s.OrgID, Name: s.Name})
		}
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// checkCategory verifies an optional category belongs to the caller's org
// (RLS already scopes the lookup to the tenant).
func (s *Service) checkCategory(ctx context.Context, q *gen.Queries, categoryID *uuid.UUID) error {
	if categoryID == nil {
		return nil
	}
	if _, err := q.GetCatalogCategoryByID(ctx, *categoryID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrCategoryNotFound
		}
		return err
	}
	return nil
}

// requireTenantMember verifies the caller is a member of the active org
// (mirrors the /me defense against forged cross-tenant tokens).
func (s *Service) requireTenantMember(ctx context.Context, q *gen.Queries, userID, orgID uuid.UUID) error {
	if _, err := q.GetMembershipByUserOrg(ctx, gen.GetMembershipByUserOrgParams{UserID: userID, OrgID: orgID}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrForbidden
		}
		return err
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

// validatePrice validates money through the single money representation.
func validatePrice(amountMinor int64, currency string) error {
	if _, err := money.New(amountMinor, currency); err != nil {
		return ErrInvalidPrice
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

// productFromRow maps the shared product row columns to the domain type.
func productFromRow(productID, orgID uuid.UUID, categoryID pgtype.UUID, name string, sku, description pgtype.Text, unit, status string, vatRateBps int32, imageS3Key pgtype.Text, minOrderQty int32, createdAt, updatedAt time.Time) Product {
	return Product{
		ProductID: productID, OrgID: orgID,
		CategoryID:  optUUID(categoryID),
		Name:        name,
		SKU:         sku.String,
		Description: description.String,
		Unit:        unit,
		Status:      status,
		VatRateBps:  vatRateBps,
		ImageURL:    imageS3Key.String,
		MinOrderQty: minOrderQty,
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
	}
}

func priceFromRow(p gen.CatalogPrice) *Price {
	return &Price{AmountMinor: p.UnitPriceMinor, Currency: strings.TrimSpace(p.Currency)}
}

func categoryFromRow(id, orgID uuid.UUID, name string, createdAt, updatedAt time.Time) Category {
	return Category{CategoryID: id, OrgID: orgID, Name: name, CreatedAt: createdAt, UpdatedAt: updatedAt}
}

func browseItemFromRow(r gen.BrowseCatalogProductsRow) BrowseItem {
	return BrowseItem{
		ProductID: r.ProductID, OrgID: r.OrgID, CategoryID: optUUID(r.CategoryID),
		Name: r.Name, SKU: r.Sku.String, Description: r.Description.String, Unit: r.Unit,
		VatRateBps: r.VatRateBps, ImageURL: r.ImageS3Key.String, MinOrderQty: r.MinOrderQty,
		Price: Price{AmountMinor: r.UnitPriceMinor, Currency: strings.TrimSpace(r.Currency)},
		SupplierName: r.SupplierName, CategoryName: r.CategoryName.String,
	}
}

func pgUUID(u uuid.UUID) pgtype.UUID { return pgtype.UUID{Bytes: u, Valid: true} }
func pgText(s string) pgtype.Text    { return pgtype.Text{String: s, Valid: s != ""} }

func pgOptUUID(u *uuid.UUID) pgtype.UUID {
	if u == nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: *u, Valid: true}
}

func optUUID(u pgtype.UUID) *uuid.UUID {
	if !u.Valid {
		return nil
	}
	id := uuid.UUID(u.Bytes)
	return &id
}
