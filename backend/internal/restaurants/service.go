// Package restaurants implements the restaurants module: restaurant records
// owned by buyer organizations and their locations (the approved `outlets`
// entity). Every operation is tenant-scoped through RLS and guarded by an
// organization-type rule (restaurants belong to buyer organizations only).
package restaurants

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

// Error is a domain error with an HTTP mapping and a stable machine code,
// following the pattern used by the identity module.
type Error struct {
	Code       string
	HTTPStatus int
	Msg        string
}

func (e *Error) Error() string { return e.Msg }

var (
	ErrNotFound       = &Error{Code: "NOT_FOUND", HTTPStatus: 404, Msg: "resource not found"}
	ErrForbidden      = &Error{Code: "FORBIDDEN", HTTPStatus: 403, Msg: "insufficient permissions"}
	ErrOrgType        = &Error{Code: "ORG_TYPE_MISMATCH", HTTPStatus: 422, Msg: "restaurants are only available to buyer organizations"}
	ErrInvalidOrgType = &Error{Code: "ORG_TYPE_MISMATCH", HTTPStatus: 422, Msg: "operation not available for this organization type"}
)

// Ctx carries the authenticated request context into the service.
type Ctx struct {
	UserID    uuid.UUID
	OrgID     uuid.UUID
	Role      string
	IP        *netip.Addr
	UserAgent string
}

// Restaurant is the domain representation of a restaurant.
type Restaurant struct {
	RestaurantID    uuid.UUID `json:"restaurant_id"`
	OrgID           uuid.UUID `json:"org_id"`
	Name            string    `json:"name"`
	LegalName       string    `json:"legal_name,omitempty"`
	Status          string    `json:"status"`
	Country         string    `json:"country"`
	DefaultCurrency string    `json:"default_currency"`
	Phone           string    `json:"phone,omitempty"`
	Email           string    `json:"email,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// Location is the domain representation of a restaurant location (outlet).
type Location struct {
	LocationID   uuid.UUID `json:"location_id"`
	RestaurantID uuid.UUID `json:"restaurant_id"`
	OrgID        uuid.UUID `json:"org_id"`
	Name         string    `json:"name"`
	Address      string    `json:"address"`
	City         string    `json:"city"`
	Country      string    `json:"country"`
	Status       string    `json:"status"`
	Phone        string    `json:"phone,omitempty"`
	Email        string    `json:"email,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// CreateInput is the validated input for creating a restaurant.
type CreateInput struct {
	Name            string
	LegalName       string
	Status          string
	Country         string
	DefaultCurrency string
	Phone           string
	Email           string
}

// UpdateInput is the validated input for updating a restaurant.
type UpdateInput struct {
	Name            string
	LegalName       string
	Status          string
	Country         string
	DefaultCurrency string
	Phone           string
	Email           string
}

// CreateLocationInput is the validated input for creating a location.
type CreateLocationInput struct {
	Name    string
	Address string
	City    string
	Country string
	Status  string
	Phone   string
	Email   string
}

// UpdateLocationInput is the validated input for updating a location.
type UpdateLocationInput struct {
	Name    string
	Address string
	City    string
	Country string
	Status  string
	Phone   string
	Email   string
}

// Service implements the restaurants module operations.
type Service struct {
	pool *pgxpool.Pool
	log  zerolog.Logger
}

func NewService(pool *pgxpool.Pool, log zerolog.Logger) *Service {
	return &Service{pool: pool, log: log}
}

// Create creates a restaurant owned by the caller's organization. Fails with
// ErrOrgType when the organization is not a buyer organization.
func (s *Service) Create(ctx context.Context, c Ctx, in CreateInput) (*Restaurant, error) {
	var out Restaurant
	err := db.WithTx(ctx, s.pool, c.OrgID.String(), c.Role, func(tx pgx.Tx) error {
		q := gen.New(tx)
		if err := s.requireBuyer(ctx, q, c.UserID, c.OrgID); err != nil {
			return err
		}
		row, err := q.CreateRestaurant(ctx, gen.CreateRestaurantParams{
			OrgID: c.OrgID, Name: in.Name, LegalName: pgText(in.LegalName),
			Bin: pgtype.Text{}, Status: in.Status, Country: in.Country,
			DefaultCurrency: in.DefaultCurrency, Phone: pgText(in.Phone), Email: pgText(in.Email),
		})
		if err != nil {
			return err
		}
		if err := insertAudit(ctx, q, c, "restaurant.created", "restaurant", row.RestaurantID, nil, row); err != nil {
			return err
		}
		out = restaurantFromRow(row)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// Get returns one restaurant by id (tenant-scoped).
func (s *Service) Get(ctx context.Context, c Ctx, restaurantID uuid.UUID) (*Restaurant, error) {
	var out Restaurant
	err := db.WithTx(ctx, s.pool, c.OrgID.String(), c.Role, func(tx pgx.Tx) error {
		q := gen.New(tx)
		if err := s.requireTenantMember(ctx, q, c.UserID, c.OrgID); err != nil {
			return err
		}
		row, err := q.GetRestaurantByID(ctx, restaurantID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		out = restaurantFromRow(row)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// List returns the caller's restaurants with offset pagination.
func (s *Service) List(ctx context.Context, c Ctx, limit, offset int) ([]Restaurant, int64, error) {
	var (
		items []Restaurant
		total int64
	)
	err := db.WithTx(ctx, s.pool, c.OrgID.String(), c.Role, func(tx pgx.Tx) error {
		q := gen.New(tx)
		if err := s.requireBuyer(ctx, q, c.UserID, c.OrgID); err != nil {
			return err
		}
		n, err := q.CountRestaurants(ctx, c.OrgID)
		if err != nil {
			return err
		}
		total = n
		rows, err := q.ListRestaurants(ctx, gen.ListRestaurantsParams{
			OrgID: c.OrgID, Limit: int32(limit), Offset: int32(offset),
		})
		if err != nil {
			return err
		}
		items = make([]Restaurant, 0, len(rows))
		for _, r := range rows {
			items = append(items, restaurantFromRow(r))
		}
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// Update updates a restaurant (tenant-scoped). The organization-type rule is
// re-checked so a supplier organization can never mutate restaurant rows.
func (s *Service) Update(ctx context.Context, c Ctx, restaurantID uuid.UUID, in UpdateInput) (*Restaurant, error) {
	var out Restaurant
	err := db.WithTx(ctx, s.pool, c.OrgID.String(), c.Role, func(tx pgx.Tx) error {
		q := gen.New(tx)
		if err := s.requireBuyer(ctx, q, c.UserID, c.OrgID); err != nil {
			return err
		}
		before, err := q.GetRestaurantByID(ctx, restaurantID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		row, err := q.UpdateRestaurant(ctx, gen.UpdateRestaurantParams{
			RestaurantID: restaurantID, Name: in.Name, LegalName: pgText(in.LegalName),
			Status: in.Status, Country: in.Country, DefaultCurrency: in.DefaultCurrency,
			Phone: pgText(in.Phone), Email: pgText(in.Email),
		})
		if err != nil {
			return err
		}
		if err := insertAudit(ctx, q, c, "restaurant.updated", "restaurant", row.RestaurantID, before, row); err != nil {
			return err
		}
		out = restaurantFromRow(row)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateLocation creates a location under a restaurant in the caller's org.
func (s *Service) CreateLocation(ctx context.Context, c Ctx, restaurantID uuid.UUID, in CreateLocationInput) (*Location, error) {
	var out Location
	err := db.WithTx(ctx, s.pool, c.OrgID.String(), c.Role, func(tx pgx.Tx) error {
		q := gen.New(tx)
		if err := s.requireBuyer(ctx, q, c.UserID, c.OrgID); err != nil {
			return err
		}
		// GetRestaurantByID is RLS-scoped: a restaurant outside the tenant
		// returns no rows and the location create fails with 404.
		if _, err := q.GetRestaurantByID(ctx, restaurantID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		row, err := q.CreateRestaurantLocation(ctx, gen.CreateRestaurantLocationParams{
			RestaurantID: restaurantID, OrgID: c.OrgID, Name: in.Name,
			Address: in.Address, City: in.City, Country: in.Country,
			Status: in.Status, Phone: pgText(in.Phone), Email: pgText(in.Email),
		})
		if err != nil {
			return err
		}
		if err := insertAudit(ctx, q, c, "restaurant.location.created", "outlet", row.OutletID, nil, row); err != nil {
			return err
		}
		out = locationFromRow(row)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// GetLocation returns one location (tenant-scoped; restaurant must also be in
// the tenant, enforced by RLS on outlets + the outlet/restaurant org trigger).
func (s *Service) GetLocation(ctx context.Context, c Ctx, locationID uuid.UUID) (*Location, error) {
	var out Location
	err := db.WithTx(ctx, s.pool, c.OrgID.String(), c.Role, func(tx pgx.Tx) error {
		q := gen.New(tx)
		if err := s.requireTenantMember(ctx, q, c.UserID, c.OrgID); err != nil {
			return err
		}
		row, err := q.GetRestaurantLocationByID(ctx, locationID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		out = locationFromRow(row)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// ListLocations returns the locations of a restaurant in the caller's org.
func (s *Service) ListLocations(ctx context.Context, c Ctx, restaurantID uuid.UUID, limit, offset int) ([]Location, int64, error) {
	var (
		items []Location
		total int64
	)
	err := db.WithTx(ctx, s.pool, c.OrgID.String(), c.Role, func(tx pgx.Tx) error {
		q := gen.New(tx)
		if err := s.requireTenantMember(ctx, q, c.UserID, c.OrgID); err != nil {
			return err
		}
		if _, err := q.GetRestaurantByID(ctx, restaurantID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		n, err := q.CountRestaurantLocations(ctx, gen.CountRestaurantLocationsParams{
			RestaurantID: restaurantID, OrgID: c.OrgID,
		})
		if err != nil {
			return err
		}
		total = n
		rows, err := q.ListRestaurantLocations(ctx, gen.ListRestaurantLocationsParams{
			RestaurantID: restaurantID, OrgID: c.OrgID, Limit: int32(limit), Offset: int32(offset),
		})
		if err != nil {
			return err
		}
		items = make([]Location, 0, len(rows))
		for _, l := range rows {
			items = append(items, locationFromRow(l))
		}
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// UpdateLocation updates a location in the caller's org.
func (s *Service) UpdateLocation(ctx context.Context, c Ctx, locationID uuid.UUID, in UpdateLocationInput) (*Location, error) {
	var out Location
	err := db.WithTx(ctx, s.pool, c.OrgID.String(), c.Role, func(tx pgx.Tx) error {
		q := gen.New(tx)
		if err := s.requireBuyer(ctx, q, c.UserID, c.OrgID); err != nil {
			return err
		}
		before, err := q.GetRestaurantLocationByID(ctx, locationID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		row, err := q.UpdateRestaurantLocation(ctx, gen.UpdateRestaurantLocationParams{
			OutletID: locationID, Name: in.Name, Address: in.Address, City: in.City,
			Country: in.Country, Status: in.Status, Phone: pgText(in.Phone), Email: pgText(in.Email),
		})
		if err != nil {
			return err
		}
		if err := insertAudit(ctx, q, c, "restaurant.location.updated", "outlet", row.OutletID, before, row); err != nil {
			return err
		}
		out = locationFromRow(row)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// requireTenantMember verifies the caller is a member of the active org. This
// mirrors the /me defense against forged cross-tenant tokens; RLS provides the
// second layer.
func (s *Service) requireTenantMember(ctx context.Context, q *gen.Queries, userID, orgID uuid.UUID) error {
	if _, err := q.GetMembershipByUserOrg(ctx, gen.GetMembershipByUserOrgParams{UserID: userID, OrgID: orgID}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrForbidden
		}
		return err
	}
	return nil
}

// requireBuyer verifies tenant membership and the organization type. The type
// check is the application-level enforcement of the buyer-only rule (backed by
// the enforce_restaurant_org_type DB trigger).
func (s *Service) requireBuyer(ctx context.Context, q *gen.Queries, userID, orgID uuid.UUID) error {
	if err := s.requireTenantMember(ctx, q, userID, orgID); err != nil {
		return err
	}
	org, err := q.GetOrganizationByID(ctx, orgID)
	if err != nil {
		return err
	}
	if org.Type != "buyer" {
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

func restaurantFromRow(r gen.Restaurant) Restaurant {
	return Restaurant{
		RestaurantID: r.RestaurantID, OrgID: r.OrgID, Name: r.Name,
		LegalName: r.LegalName.String, Status: r.Status, Country: r.Country,
		DefaultCurrency: r.DefaultCurrency, Phone: r.Phone.String, Email: r.Email.String,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func locationFromRow(o gen.Outlet) Location {
	return Location{
		LocationID: o.OutletID, RestaurantID: o.RestaurantID, OrgID: o.OrgID,
		Name: o.Name, Address: o.Address, City: o.City, Country: o.Country,
		Status: o.Status, Phone: o.Phone.String, Email: o.Email.String,
		CreatedAt: o.CreatedAt, UpdatedAt: o.UpdatedAt,
	}
}

func pgUUID(u uuid.UUID) pgtype.UUID { return pgtype.UUID{Bytes: u, Valid: true} }

func pgText(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }
