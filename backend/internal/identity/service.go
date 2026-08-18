package identity

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/netip"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"tirek/backend/internal/organizations"
	"tirek/backend/internal/platform/db"
	"tirek/backend/internal/platform/db/gen"
)

// Service errors map directly to HTTP responses (see http.go).
var (
	ErrEmailTaken          = &Error{Code: "EMAIL_TAKEN", HTTPStatus: 409, Msg: "an account with this email already exists"}
	ErrInvalidCredentials  = &Error{Code: "INVALID_CREDENTIALS", HTTPStatus: 401, Msg: "invalid email or password"}
	ErrInvalidRefreshToken = &Error{Code: "INVALID_REFRESH_TOKEN", HTTPStatus: 401, Msg: "refresh token is invalid or expired"}
	ErrRefreshTokenReuse   = &Error{Code: "REFRESH_TOKEN_REUSE", HTTPStatus: 401, Msg: "refresh token reuse detected; session revoked"}
	ErrRateLimited         = &Error{Code: "RATE_LIMITED", HTTPStatus: 429, Msg: "too many failed login attempts; try again later"}
	ErrForbidden           = &Error{Code: "FORBIDDEN", HTTPStatus: 403, Msg: "insufficient permissions"}
	ErrUnauthenticated     = &Error{Code: "UNAUTHENTICATED", HTTPStatus: 401, Msg: "authentication required"}
	ErrNotFound            = &Error{Code: "NOT_FOUND", HTTPStatus: 404, Msg: "resource not found"}
)

// Error is a domain error with an HTTP mapping and a stable machine code.
type Error struct {
	Code       string
	HTTPStatus int
	Msg        string
}

func (e *Error) Error() string { return e.Msg }

// LoginThrottle configures brute-force protection for the login endpoint.
type LoginThrottle struct {
	MaxFailuresPerUser int
	MaxFailuresPerIP   int
	Window             time.Duration
}

// Service implements the identity module operations.
type Service struct {
	pool     *pgxpool.Pool
	tokens   TokenManager
	hasher   PasswordHasher
	orgs     *organizations.Service
	log      zerolog.Logger
	throttle LoginThrottle
}

func NewService(pool *pgxpool.Pool, tokens TokenManager, orgs *organizations.Service, log zerolog.Logger) *Service {
	return &Service{
		pool:   pool,
		tokens: tokens,
		orgs:   orgs,
		log:    log,
		throttle: LoginThrottle{
			MaxFailuresPerUser: 5,
			MaxFailuresPerIP:   20,
			Window:             15 * time.Minute,
		},
	}
}

// SetThrottle overrides the brute-force protection settings.
func (s *Service) SetThrottle(t LoginThrottle) { s.throttle = t }

// User is the identity-facing representation of a user.
type User struct {
	UserID       uuid.UUID
	Email        string
	FullName     string
	Phone        string
	Status       string
	DefaultOrgID *uuid.UUID
}

// Organization is the identity-facing representation of an organization.
type Organization struct {
	OrgID           uuid.UUID
	Name            string
	Type            string
	Country         string
	DefaultCurrency string
	Bin             string
	Status          string
}

// Session is the result of an auth operation.
type Session struct {
	AccessToken      string
	RefreshToken     string
	AccessExpiresAt  time.Time
	RefreshExpiresAt time.Time
	User             User
	Org              Organization
	Role             string
}

// Me is the /me response payload.
type Me struct {
	User        User
	Org         Organization
	Role        string
	Permissions []string
}

type RegisterInput struct {
	Email    string
	Password string
	FullName string
	Phone    string
	OrgName  string
	OrgType  string
}

type LoginInput struct {
	Email    string
	Password string
	OrgID    *uuid.UUID
}

// Register creates a user, a new organization with seeded system roles, the
// owner membership, and an initial session. Everything commits atomically.
func (s *Service) Register(ctx context.Context, in RegisterInput, ip *netip.Addr, userAgent string) (*Session, error) {
	passwordHash, err := s.hasher.Hash(in.Password)
	if err != nil {
		return nil, err
	}

	var result Session
	err = db.WithTx(ctx, s.pool, "", "", func(tx pgx.Tx) error {
		q := gen.New(tx)

		org, err := q.CreateOrganization(ctx, gen.CreateOrganizationParams{
			Name:            in.OrgName,
			Type:            in.OrgType,
			Country:         "KZ",
			DefaultCurrency: "KZT",
		})
		if err != nil {
			return err
		}
		if err := db.SetTenant(ctx, tx, org.OrgID.String(), "owner"); err != nil {
			return err
		}

		user, err := q.CreateUser(ctx, gen.CreateUserParams{
			Email:        normalizeEmail(in.Email),
			PasswordHash: passwordHash,
			FullName:     in.FullName,
			Phone:        pgText(in.Phone),
		})
		if err != nil {
			return err
		}
		if err := q.SetUserDefaultOrg(ctx, gen.SetUserDefaultOrgParams{
			UserID:       user.UserID,
			DefaultOrgID: pgUUID(org.OrgID),
		}); err != nil {
			return err
		}

		ownerRoleID, err := organizations.SeedOrgRoles(ctx, q, org.OrgID)
		if err != nil {
			return err
		}
		if _, err := q.CreateMembership(ctx, gen.CreateMembershipParams{
			OrgID: org.OrgID, UserID: user.UserID, RoleID: ownerRoleID,
		}); err != nil {
			return err
		}

		sess, refreshExp, err := s.createSession(ctx, q, user.UserID, org.OrgID, ip, userAgent)
		if err != nil {
			return err
		}
		if err := q.InsertAuthEvent(ctx, gen.InsertAuthEventParams{
			UserID: pgUUID(user.UserID), OrgID: pgUUID(org.OrgID),
			Action: "register", Ip: ip, UserAgent: pgText(userAgent),
		}); err != nil {
			return err
		}

		access, accessExp, err := s.tokens.IssueAccessToken(user.UserID, org.OrgID, sess.Row.SessionID, "owner")
		if err != nil {
			return err
		}
		result = Session{
			AccessToken: access, RefreshToken: sess.RefreshRaw,
			AccessExpiresAt: accessExp, RefreshExpiresAt: refreshExp,
			User: userFromRow(user), Org: orgFromRow(org), Role: "owner",
		}
		return nil
	})
	if err != nil {
		if db.IsUniqueViolation(err) {
			return nil, ErrEmailTaken
		}
		return nil, err
	}
	return &result, nil
}

// Login verifies credentials and starts a new session in the requested (or
// default) organization.
func (s *Service) Login(ctx context.Context, in LoginInput, ip *netip.Addr, userAgent string) (*Session, error) {
	email := normalizeEmail(in.Email)

	user, err := gen.New(s.pool).GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			s.recordLoginFailure(ctx, nil, ip, userAgent, email)
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}

	blocked, err := s.throttled(ctx, user.UserID, ip)
	if err != nil {
		return nil, err
	}
	if blocked {
		return nil, ErrRateLimited
	}

	ok, err := s.hasher.Verify(in.Password, user.PasswordHash)
	if err != nil || !ok {
		s.recordLoginFailure(ctx, &user.UserID, ip, userAgent, "")
		return nil, ErrInvalidCredentials
	}

	orgID := in.OrgID
	if orgID == nil {
		if !user.DefaultOrgID.Valid {
			return nil, ErrInvalidCredentials
		}
		id := uuid.UUID(user.DefaultOrgID.Bytes)
		orgID = &id
	}

	var result Session
	err = db.WithTx(ctx, s.pool, orgID.String(), "", func(tx pgx.Tx) error {
		q := gen.New(tx)
		membership, err := q.GetMembershipByUserOrg(ctx, gen.GetMembershipByUserOrgParams{
			UserID: user.UserID, OrgID: *orgID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrInvalidCredentials
			}
			return err
		}
		role, err := q.GetRoleByID(ctx, membership.RoleID)
		if err != nil {
			return err
		}
		org, err := q.GetOrganizationByID(ctx, *orgID)
		if err != nil {
			return err
		}

		sess, refreshExp, err := s.createSession(ctx, q, user.UserID, *orgID, ip, userAgent)
		if err != nil {
			return err
		}
		if err := q.InsertAuthEvent(ctx, gen.InsertAuthEventParams{
			UserID: pgUUID(user.UserID), OrgID: pgUUID(*orgID),
			Action: "login", Ip: ip, UserAgent: pgText(userAgent),
		}); err != nil {
			return err
		}

		access, accessExp, err := s.tokens.IssueAccessToken(user.UserID, *orgID, sess.Row.SessionID, role.Name)
		if err != nil {
			return err
		}
		result = Session{
			AccessToken: access, RefreshToken: sess.RefreshRaw,
			AccessExpiresAt: accessExp, RefreshExpiresAt: refreshExp,
			User: userFromRow(user), Org: orgFromRow(org), Role: role.Name,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// Refresh rotates the refresh token and issues a new pair. Reuse of a rotated
// token revokes the session (committed separately from the error return).
func (s *Service) Refresh(ctx context.Context, rawRefresh string, ip *netip.Addr, userAgent string) (*Session, error) {
	if rawRefresh == "" {
		return nil, ErrInvalidRefreshToken
	}
	hash := HashRefreshToken(rawRefresh)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := gen.New(tx)

	sess, err := q.GetSessionByRefreshHashForUpdate(ctx, hash)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	if errors.Is(err, pgx.ErrNoRows) {
		if err := s.handleRefreshReuse(ctx, q, hash, ip, userAgent); err != nil {
			return nil, err
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		return nil, ErrRefreshTokenReuse
	}

	if sess.RevokedAt.Valid {
		return nil, ErrInvalidRefreshToken
	}
	if time.Now().UTC().After(sess.ExpiresAt) {
		return nil, ErrInvalidRefreshToken
	}
	if !sess.OrgID.Valid {
		return nil, ErrInvalidRefreshToken
	}

	// Record the old hash (enables reuse detection) and rotate.
	if err := q.InsertRefreshHistory(ctx, gen.InsertRefreshHistoryParams{
		SessionID: sess.SessionID, RefreshHash: sess.RefreshHash,
	}); err != nil {
		return nil, err
	}
	raw, newHash, err := NewRefreshToken()
	if err != nil {
		return nil, err
	}
	refreshExp := time.Now().UTC().Add(s.tokens.RefreshTTL)
	if _, err := q.RotateSession(ctx, gen.RotateSessionParams{
		SessionID: sess.SessionID, RefreshHash: newHash, ExpiresAt: refreshExp,
	}); err != nil {
		return nil, err
	}

	// Resolve current role/org under the tenant context (RLS).
	if err := db.SetTenant(ctx, tx, uuid.UUID(sess.OrgID.Bytes).String(), ""); err != nil {
		return nil, err
	}
	membership, err := q.GetMembershipByUserOrg(ctx, gen.GetMembershipByUserOrgParams{
		UserID: sess.UserID, OrgID: sess.OrgID.Bytes,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrInvalidRefreshToken
		}
		return nil, err
	}
	role, err := q.GetRoleByID(ctx, membership.RoleID)
	if err != nil {
		return nil, err
	}
	org, err := q.GetOrganizationByID(ctx, uuid.UUID(sess.OrgID.Bytes))
	if err != nil {
		return nil, err
	}
	user, err := q.GetUserByID(ctx, sess.UserID)
	if err != nil {
		return nil, err
	}

	if err := q.InsertAuthEvent(ctx, gen.InsertAuthEventParams{
		UserID: pgUUID(sess.UserID), OrgID: sess.OrgID,
		Action: "refresh", Ip: ip, UserAgent: pgText(userAgent),
	}); err != nil {
		return nil, err
	}

	access, accessExp, err := s.tokens.IssueAccessToken(sess.UserID, uuid.UUID(sess.OrgID.Bytes), sess.SessionID, role.Name)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &Session{
		AccessToken: access, RefreshToken: raw,
		AccessExpiresAt: accessExp, RefreshExpiresAt: refreshExp,
		User: userFromRow(user), Org: orgFromRow(org), Role: role.Name,
	}, nil
}

// handleRefreshReuse revokes the session whose previously-rotated token was
// presented again. Returns nil when reuse was detected and handled; the caller
// commits the transaction and reports ErrRefreshTokenReuse.
func (s *Service) handleRefreshReuse(ctx context.Context, q *gen.Queries, hash string, ip *netip.Addr, userAgent string) error {
	sid, err := q.GetSessionIDByHistoryHash(ctx, hash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvalidRefreshToken
		}
		return err
	}
	if _, err := q.RevokeSession(ctx, gen.RevokeSessionParams{
		SessionID: sid, RevokedReason: pgText("refresh_token_reuse"),
	}); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if err := q.InsertAuthEvent(ctx, gen.InsertAuthEventParams{
		Action: "refresh_reuse", Ip: ip, UserAgent: pgText(userAgent),
	}); err != nil {
		return err
	}
	s.log.Warn().Str("session_id", sid.String()).Msg("refresh token reuse detected; session revoked")
	return nil
}

// Logout revokes the session identified by the presented refresh token.
func (s *Service) Logout(ctx context.Context, rawRefresh string, ip *netip.Addr, userAgent string) error {
	if rawRefresh == "" {
		return nil
	}
	hash := HashRefreshToken(rawRefresh)
	return db.WithTx(ctx, s.pool, "", "", func(tx pgx.Tx) error {
		q := gen.New(tx)
		sess, err := q.GetSessionByRefreshHash(ctx, hash)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil // idempotent
			}
			return err
		}
		if sess.RevokedAt.Valid {
			return nil
		}
		if _, err := q.RevokeSession(ctx, gen.RevokeSessionParams{
			SessionID: sess.SessionID, RevokedReason: pgText("logout"),
		}); err != nil {
			return err
		}
		return q.InsertAuthEvent(ctx, gen.InsertAuthEventParams{
			UserID: pgUUID(sess.UserID), OrgID: sess.OrgID,
			Action: "logout", Ip: ip, UserAgent: pgText(userAgent),
		})
	})
}

// Me returns the current user, active organization, role, and permissions.
func (s *Service) Me(ctx context.Context, userID, orgID uuid.UUID) (*Me, error) {
	var out Me
	err := db.WithTx(ctx, s.pool, orgID.String(), "", func(tx pgx.Tx) error {
		q := gen.New(tx)
		user, err := q.GetUserByID(ctx, userID)
		if err != nil {
			return err
		}
		org, err := q.GetOrganizationByID(ctx, orgID)
		if err != nil {
			return err
		}
		membership, err := q.GetMembershipByUserOrg(ctx, gen.GetMembershipByUserOrgParams{
			UserID: userID, OrgID: orgID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrForbidden
			}
			return err
		}
		role, err := q.GetRoleByID(ctx, membership.RoleID)
		if err != nil {
			return err
		}
		perms, err := q.ListRolePermissions(ctx, membership.RoleID)
		if err != nil {
			return err
		}
		out = Me{User: userFromRow(user), Org: orgFromRow(org), Role: role.Name, Permissions: perms}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// sessionResult carries a created session row plus the raw refresh token.
type sessionResult struct {
	Row        gen.Session
	RefreshRaw string
}

func (s *Service) createSession(ctx context.Context, q *gen.Queries, userID, orgID uuid.UUID, ip *netip.Addr, userAgent string) (sessionResult, time.Time, error) {
	raw, hash, err := NewRefreshToken()
	if err != nil {
		return sessionResult{}, time.Time{}, err
	}
	refreshExp := time.Now().UTC().Add(s.tokens.RefreshTTL)
	row, err := q.CreateSession(ctx, gen.CreateSessionParams{
		UserID: userID, OrgID: pgUUID(orgID),
		RefreshHash: hash, ExpiresAt: refreshExp,
		Ip: ip, UserAgent: pgText(userAgent),
	})
	if err != nil {
		return sessionResult{}, time.Time{}, err
	}
	return sessionResult{Row: row, RefreshRaw: raw}, refreshExp, nil
}

// throttled reports whether the user or IP has exceeded the failed-login limit.
func (s *Service) throttled(ctx context.Context, userID uuid.UUID, ip *netip.Addr) (bool, error) {
	q := gen.New(s.pool)
	cutoff := time.Now().UTC().Add(-s.throttle.Window)
	n, err := q.CountRecentLoginFailuresByUser(ctx, gen.CountRecentLoginFailuresByUserParams{
		UserID: pgUUID(userID), CreatedAt: cutoff,
	})
	if err != nil {
		return false, err
	}
	if n >= int64(s.throttle.MaxFailuresPerUser) {
		return true, nil
	}
	if ip != nil {
		m, err := q.CountRecentLoginFailuresByIP(ctx, gen.CountRecentLoginFailuresByIPParams{
			Ip: ip, CreatedAt: cutoff,
		})
		if err != nil {
			return false, err
		}
		if m >= int64(s.throttle.MaxFailuresPerIP) {
			return true, nil
		}
	}
	return false, nil
}

// recordLoginFailure appends a login_failed auth event. For unknown emails the
// user_id is null and a hashed identifier is stored instead (no PII).
func (s *Service) recordLoginFailure(ctx context.Context, userID *uuid.UUID, ip *netip.Addr, userAgent, email string) {
	params := gen.InsertAuthEventParams{Action: "login_failed", Ip: ip, UserAgent: pgText(userAgent)}
	if userID != nil {
		params.UserID = pgUUID(*userID)
	} else if email != "" {
		sum := sha256.Sum256([]byte(normalizeEmail(email)))
		params.Identifier = pgText(hex.EncodeToString(sum[:]))
	}
	if err := gen.New(s.pool).InsertAuthEvent(ctx, params); err != nil {
		s.log.Warn().Err(err).Msg("record login failure")
	}
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func pgUUID(u uuid.UUID) pgtype.UUID { return pgtype.UUID{Bytes: u, Valid: true} }

func pgText(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }

func userFromRow(u gen.User) User {
	user := User{UserID: u.UserID, Email: u.Email, FullName: u.FullName, Status: u.Status}
	if u.Phone.Valid {
		user.Phone = u.Phone.String
	}
	if u.DefaultOrgID.Valid {
		id := uuid.UUID(u.DefaultOrgID.Bytes)
		user.DefaultOrgID = &id
	}
	return user
}

func orgFromRow(o gen.Organization) Organization {
	org := Organization{
		OrgID: o.OrgID, Name: o.Name, Type: o.Type,
		Country: o.Country, DefaultCurrency: o.DefaultCurrency, Status: o.Status,
	}
	if o.Bin.Valid {
		org.Bin = o.Bin.String
	}
	return org
}
