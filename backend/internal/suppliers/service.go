// Package suppliers implements the suppliers module: the business profile of a
// supplier organization. Per the approved domain model (ORGANIZATION ||--o{
// SUPPLIER : is) there is exactly one supplier profile per supplier
// organization (enforced by a unique index and by the service). Every
// operation is tenant-scoped through RLS and guarded by an organization-type
// rule (supplier profiles belong to supplier organizations only).
package suppliers

import (
	"context"
	"encoding/json"
	"errors"
	"net/netip"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"tirek/backend/internal/platform/db"
	"tirek/backend/internal/platform/db/gen"
)

// Error is a domain error with an HTTP mapping and a stable machine code.
type Error struct {
	Code       string
	HTTPStatus int
	Msg        string
}

func (e *Error) Error() string { return e.Msg }

var (
	ErrNotFound      = &Error{Code: "NOT_FOUND", HTTPStatus: 404, Msg: "resource not found"}
	ErrForbidden     = &Error{Code: "FORBIDDEN", HTTPStatus: 403, Msg: "insufficient permissions"}
	ErrOrgType       = &Error{Code: "ORG_TYPE_MISMATCH", HTTPStatus: 422, Msg: "supplier profiles are only available to supplier organizations"}
	ErrProfileExists = &Error{Code: "SUPPLIER_PROFILE_EXISTS", HTTPStatus: 409, Msg: "this organization already has a supplier profile"}
)

// Ctx carries the authenticated request context into the service.
type Ctx struct {
	UserID    uuid.UUID
	OrgID     uuid.UUID
	Role      string
	IP        *netip.Addr
	UserAgent string
}

// Supplier is the domain representation of a supplier profile. Sensitive
// fields (payout_account, bin) are never exposed over the API.
type Supplier struct {
	SupplierID      uuid.UUID `json:"supplier_id"`
	OrgID           uuid.UUID `json:"org_id"`
	Name            string    `json:"name"`
	LegalName       string    `json:"legal_name,omitempty"`
	Status          string    `json:"status"`
	Country         string    `json:"country"`
	DefaultCurrency string    `json:"default_currency"`
	Phone           string    `json:"phone,omitempty"`
	Email           string    `json:"email,omitempty"`
	PaymentTerms    string    `json:"payment_terms"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// CreateInput is the validated input for creating a supplier profile.
type CreateInput struct {
	Name            string
	LegalName       string
	Status          string
	Country         string
	DefaultCurrency string
	Phone           string
	Email           string
}

// UpdateInput is the validated input for updating a supplier profile.
type UpdateInput struct {
	Name            string
	LegalName       string
	Status          string
	Country         string
	DefaultCurrency string
	Phone           string
	Email           string
}

// Service implements the suppliers module operations.
type Service struct {
	pool *pgxpool.Pool
	log  zerolog.Logger
}

func NewService(pool *pgxpool.Pool, log zerolog.Logger) *Service {
	return &Service{pool: pool, log: log}
}

// Create creates the supplier profile for the caller's organization. Fails
// with ErrOrgType when the organization is not a supplier organization and
// with ErrProfileExists when a profile already exists (1:1 rule).
func (s *Service) Create(ctx context.Context, c Ctx, in CreateInput) (*Supplier, error) {
	var out Supplier
	err := db.WithTx(ctx, s.pool, c.OrgID.String(), c.Role, func(tx pgx.Tx) error {
		q := gen.New(tx)
		if err := s.requireSupplier(ctx, q, c.UserID, c.OrgID); err != nil {
			return err
		}
		if _, err := q.GetSupplierByOrgID(ctx, c.OrgID); err == nil {
			return ErrProfileExists
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		row, err := q.CreateSupplier(ctx, gen.CreateSupplierParams{
			OrgID: c.OrgID, Name: in.Name, LegalName: pgText(in.LegalName),
			Bin: pgtype.Text{}, PaymentTerms: "net14",
			Status: in.Status, Country: in.Country, DefaultCurrency: in.DefaultCurrency,
			Phone: pgText(in.Phone), Email: pgText(in.Email),
		})
		if err != nil {
			return err
		}
		if err := insertAudit(ctx, q, c, "supplier.created", "supplier", row.SupplierID, nil, row); err != nil {
			return err
		}
		out = supplierFromRow(row)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// Get returns one supplier profile by id (tenant-scoped).
func (s *Service) Get(ctx context.Context, c Ctx, supplierID uuid.UUID) (*Supplier, error) {
	var out Supplier
	err := db.WithTx(ctx, s.pool, c.OrgID.String(), c.Role, func(tx pgx.Tx) error {
		q := gen.New(tx)
		if err := s.requireTenantMember(ctx, q, c.UserID, c.OrgID); err != nil {
			return err
		}
		row, err := q.GetSupplierByID(ctx, supplierID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		out = supplierFromRow(row)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// List returns the caller's supplier profiles (at most one per org) with
// offset pagination.
func (s *Service) List(ctx context.Context, c Ctx, limit, offset int) ([]Supplier, int64, error) {
	var (
		items []Supplier
		total int64
	)
	err := db.WithTx(ctx, s.pool, c.OrgID.String(), c.Role, func(tx pgx.Tx) error {
		q := gen.New(tx)
		if err := s.requireSupplier(ctx, q, c.UserID, c.OrgID); err != nil {
			return err
		}
		n, err := q.CountSuppliers(ctx, c.OrgID)
		if err != nil {
			return err
		}
		total = n
		rows, err := q.ListSuppliers(ctx, gen.ListSuppliersParams{
			OrgID: c.OrgID, Limit: int32(limit), Offset: int32(offset),
		})
		if err != nil {
			return err
		}
		items = make([]Supplier, 0, len(rows))
		for _, s := range rows {
			items = append(items, supplierFromRow(s))
		}
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// Update updates a supplier profile (tenant-scoped). The organization-type
// rule is re-checked so a buyer organization can never mutate supplier rows.
func (s *Service) Update(ctx context.Context, c Ctx, supplierID uuid.UUID, in UpdateInput) (*Supplier, error) {
	var out Supplier
	err := db.WithTx(ctx, s.pool, c.OrgID.String(), c.Role, func(tx pgx.Tx) error {
		q := gen.New(tx)
		if err := s.requireSupplier(ctx, q, c.UserID, c.OrgID); err != nil {
			return err
		}
		before, err := q.GetSupplierByID(ctx, supplierID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		row, err := q.UpdateSupplier(ctx, gen.UpdateSupplierParams{
			SupplierID: supplierID, Name: in.Name, LegalName: pgText(in.LegalName),
			Status: in.Status, Country: in.Country, DefaultCurrency: in.DefaultCurrency,
			Phone: pgText(in.Phone), Email: pgText(in.Email),
		})
		if err != nil {
			return err
		}
		if err := insertAudit(ctx, q, c, "supplier.updated", "supplier", row.SupplierID, before, row); err != nil {
			return err
		}
		out = supplierFromRow(row)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
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
		return ErrOrgType
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

func supplierFromRow(r gen.Supplier) Supplier {
	return Supplier{
		SupplierID: r.SupplierID, OrgID: r.OrgID, Name: r.Name,
		LegalName: r.LegalName.String, Status: r.Status, Country: r.Country,
		DefaultCurrency: r.DefaultCurrency, Phone: r.Phone.String, Email: r.Email.String,
		PaymentTerms: r.PaymentTerms, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func pgUUID(u uuid.UUID) pgtype.UUID { return pgtype.UUID{Bytes: u, Valid: true} }

func pgText(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }
