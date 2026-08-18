// Package organizations implements the organizations module: tenant
// organizations, memberships, roles, and permissions (RBAC).
package organizations

import (
	"context"

	"github.com/jackc/pgx/v5"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"tirek/backend/internal/platform/db"
	"tirek/backend/internal/platform/db/gen"
)

// SystemRoleOrder is the deterministic creation order for system roles.
var SystemRoleOrder = []string{"owner", "admin", "procurement", "accountant", "finance", "viewer"}

// SystemRoles maps system role names to their permission sets. It mirrors the
// seed data in backend/migrations/0001_init.sql (0003 for the
// restaurants/suppliers permissions, 0004 for catalog.read, 0005 for the
// procurement permissions) and is applied to every new organization at
// registration.
var SystemRoles = map[string][]string{
	"owner":       {"org.owner"},
	"admin":       {"orders.create", "orders.confirm", "catalog.manage", "invoices.approve", "payments.approve", "payments.refund", "finance.request", "ledger.read", "audit.read", "members.manage", "settings.manage", "restaurants.manage", "suppliers.manage", "procurement.read", "procurement.write", "procurement.manage"},
	"procurement": {"orders.create", "catalog.manage", "restaurants.read", "procurement.read", "procurement.write"},
	"accountant":  {"invoices.approve", "ledger.read", "catalog.read", "procurement.read"},
	"finance":     {"finance.request", "ledger.read", "catalog.read", "procurement.read"},
	"viewer":      {"ledger.read", "restaurants.read", "suppliers.read", "catalog.read", "procurement.read"},
}

// Service is the organizations module service.
type Service struct {
	pool  *pgxpool.Pool
	log   zerolog.Logger
	cache *PermissionCache
}

func NewService(pool *pgxpool.Pool, log zerolog.Logger) *Service {
	return &Service{pool: pool, log: log, cache: newPermissionCache(60 * time.Second)}
}

// SeedOrgRoles creates the system roles (and their permission mappings) for a
// new organization and returns the id of the owner role. It must be called
// inside a transaction with the tenant context already set to orgID (the
// roles and memberships tables are RLS-protected).
func SeedOrgRoles(ctx context.Context, q gen.Querier, orgID uuid.UUID) (ownerRoleID uuid.UUID, err error) {
	for _, name := range SystemRoleOrder {
		role, err := q.CreateRole(ctx, gen.CreateRoleParams{OrgID: orgID, Name: name, IsSystem: true})
		if err != nil {
			return uuid.Nil, err
		}
		if name == "owner" {
			ownerRoleID = role.RoleID
		}
		for _, perm := range SystemRoles[name] {
			if err := q.CreateRolePermission(ctx, gen.CreateRolePermissionParams{RoleID: role.RoleID, PermissionID: perm}); err != nil {
				return uuid.Nil, err
			}
		}
	}
	return ownerRoleID, nil
}

// Permissions returns the permission set for role in org, resolving it from the
// database on cache miss and caching it for the configured TTL. The result
// treats the implicit org.owner wildcard as "every permission".
func (s *Service) Permissions(ctx context.Context, orgID uuid.UUID, roleName string) (map[string]bool, error) {
	key := orgID.String() + ":" + roleName
	if perms, ok := s.cache.get(key); ok {
		return perms, nil
	}

	var perms map[string]bool
	err := db.WithTx(ctx, s.pool, orgID.String(), roleName, func(tx pgx.Tx) error {
		q := gen.New(tx)
		role, err := q.GetRoleByOrgName(ctx, gen.GetRoleByOrgNameParams{OrgID: orgID, Name: roleName})
		if err != nil {
			return err
		}
		list, err := q.ListRolePermissions(ctx, role.RoleID)
		if err != nil {
			return err
		}
		perms = make(map[string]bool, len(list)+1)
		for _, p := range list {
			perms[p] = true
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.cache.set(key, perms)
	return perms, nil
}

// permissionCache is a minimal TTL cache for role permission sets.
type PermissionCache struct {
	mu      sync.Mutex
	ttl     time.Duration
	entries map[string]permissionEntry
}

type permissionEntry struct {
	perms     map[string]bool
	expiresAt time.Time
}

func newPermissionCache(ttl time.Duration) *PermissionCache {
	return &PermissionCache{ttl: ttl, entries: make(map[string]permissionEntry)}
}

func (c *PermissionCache) get(key string) (map[string]bool, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok || time.Now().After(e.expiresAt) {
		delete(c.entries, key)
		return nil, false
	}
	return e.perms, true
}

func (c *PermissionCache) set(key string, perms map[string]bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = permissionEntry{perms: perms, expiresAt: time.Now().Add(c.ttl)}
}

// Flush clears the permission cache (used in tests).
func (s *Service) Flush() {
	s.cache.mu.Lock()
	defer s.cache.mu.Unlock()
	s.cache.entries = make(map[string]permissionEntry)
}
