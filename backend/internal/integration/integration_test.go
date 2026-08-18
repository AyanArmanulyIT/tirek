// Package integration runs the identity module end-to-end against a real
// PostgreSQL instance using the low-privilege tirek_app role, so RLS is
// exercised. Provisioning (roles, schema reset, migrations) happens in
// TestMain; point TIREK_TEST_DATABASE_URL / TIREK_TEST_APP_DATABASE_URL at a
// disposable database (default: local postgres, tirek_test).
package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"tirek/backend/internal/httpapi"
	"tirek/backend/internal/identity"
	"tirek/backend/internal/organizations"
	"tirek/backend/internal/platform/config"
	"tirek/backend/internal/platform/db/gen"
)

var (
	adminPool *pgxpool.Pool
	appPool   *pgxpool.Pool
	api       *httpapi.Server
	cfg       config.Config
	ctx       = context.Background()
	cookie    = "tirek_refresh"
)

func TestMain(m *testing.M) {
	adminPool, appPool = provision(ctx)
	defer adminPool.Close()
	defer appPool.Close()

	cfg = testConfig()
	cookie = cfg.Auth.CookieName
	logger := zerolog.New(os.Stderr).Level(zerolog.ErrorLevel)
	api = httpapi.New(cfg, appPool, logger)

	os.Exit(m.Run())
}

func testConfig() config.Config {
	return config.Config{
		Env:      "test",
		LogLevel: "error",
		API: config.APIConfig{
			Port: "0", ExternalURL: "http://localhost:8080",
			WebOrigin: "http://localhost:3000", WebhookSecret: "test-webhook-secret",
		},
		Database: config.DatabaseConfig{URL: appURL(), PoolMax: 10},
		Redis:    config.RedisConfig{URL: ""},
		Auth: config.AuthConfig{
			AccessSecret:  "test-access-secret-0123456789abcdef0123456789",
			RefreshSecret: "test-refresh-secret-0123456789abcdef0123456789",
			AccessTTL:     15 * time.Minute,
			RefreshTTL:    720 * time.Hour,
			CookieDomain:  "localhost",
			CookieSecure:  false,
			CookieName:    "tirek_refresh",
		},
		Payments:  config.PaymentsConfig{Provider: "mock", Mode: "sandbox"},
		Financing: config.FinancingConfig{Provider: "mock"},
		Worker:    config.WorkerConfig{OutboxPollInterval: time.Second, OutboxBatchSize: 10},
	}
}

// ---------------------------------------------------------------------------
// HTTP test client
// ---------------------------------------------------------------------------

type client struct {
	srv *httptest.Server
	hc  *http.Client
}

func newClient(t *testing.T) *client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookie jar: %v", err)
	}
	srv := httptest.NewServer(api.Handler())
	t.Cleanup(srv.Close)
	return &client{srv: srv, hc: &http.Client{Jar: jar}}
}

// do performs a request and returns the status, body, and the refresh-cookie
// value from Set-Cookie ("" when absent). It never calls t.Fatal so it is safe
// for concurrent use; callers check err.
func (c *client) do(method, path, body, token string, headers map[string]string) (int, []byte, string, error) {
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, c.srv.URL+path, rdr)
	if err != nil {
		return 0, nil, "", err
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := c.hc.Do(req)
	if err != nil {
		return 0, nil, "", err
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		return 0, nil, "", err
	}
	return res.StatusCode, data, refreshCookie(res), nil
}

func refreshCookie(res *http.Response) string {
	for _, sc := range res.Header.Values("Set-Cookie") {
		c, err := http.ParseSetCookie(sc)
		if err == nil && c.Name == cookie {
			return c.Value
		}
	}
	return ""
}

func (c *client) register(t *testing.T, email, password, fullName, orgName, orgType string) (int, map[string]any, string) {
	t.Helper()
	body := fmt.Sprintf(`{"email":%q,"password":%q,"full_name":%q,"org_name":%q,"org_type":%q}`,
		email, password, fullName, orgName, orgType)
	status, data, refresh, err := c.do(http.MethodPost, "/api/v1/auth/register", body, "", nil)
	if err != nil {
		t.Fatalf("register request: %v", err)
	}
	return status, decodeBody(t, data), refresh
}

func (c *client) login(t *testing.T, email, password string) (int, map[string]any, string) {
	t.Helper()
	body := fmt.Sprintf(`{"email":%q,"password":%q}`, email, password)
	status, data, refresh, err := c.do(http.MethodPost, "/api/v1/auth/login", body, "", nil)
	if err != nil {
		t.Fatalf("login request: %v", err)
	}
	return status, decodeBody(t, data), refresh
}

func (c *client) loginWithOrg(t *testing.T, email, password, orgID string) (int, map[string]any, string) {
	t.Helper()
	body := fmt.Sprintf(`{"email":%q,"password":%q,"org_id":%q}`, email, password, orgID)
	status, data, refresh, err := c.do(http.MethodPost, "/api/v1/auth/login", body, "", nil)
	if err != nil {
		t.Fatalf("login request: %v", err)
	}
	return status, decodeBody(t, data), refresh
}

// refresh posts to /api/v1/auth/refresh with an explicit refresh cookie.
func (c *client) refresh(t *testing.T, refreshToken string) (int, map[string]any, string) {
	t.Helper()
	status, data, newCookie, err := c.do(http.MethodPost, "/api/v1/auth/refresh", "", "", map[string]string{
		"Cookie": cookie + "=" + refreshToken,
	})
	if err != nil {
		t.Fatalf("refresh request: %v", err)
	}
	return status, decodeBody(t, data), newCookie
}

func (c *client) refreshRaw(refreshToken string) (int, string) {
	status, _, newCookie, err := c.do(http.MethodPost, "/api/v1/auth/refresh", "", "", map[string]string{
		"Cookie": cookie + "=" + refreshToken,
	})
	if err != nil {
		return -1, ""
	}
	return status, newCookie
}

func (c *client) logout(t *testing.T, refreshToken string) (int, error) {
	t.Helper()
	headers := map[string]string{}
	if refreshToken != "" {
		headers["Cookie"] = cookie + "=" + refreshToken
	}
	status, _, _, err := c.do(http.MethodPost, "/api/v1/auth/logout", "", "", headers)
	return status, err
}

func (c *client) me(t *testing.T, accessToken string) (int, map[string]any) {
	t.Helper()
	status, data, _, err := c.do(http.MethodGet, "/api/v1/me", "", accessToken, nil)
	if err != nil {
		t.Fatalf("me request: %v", err)
	}
	return status, decodeBody(t, data)
}

func decodeBody(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if len(data) == 0 {
		return m
	}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("decode body %q: %v", string(data), err)
	}
	return m
}

func accessToken(t *testing.T, body map[string]any) string {
	t.Helper()
	tok, _ := body["access_token"].(string)
	if tok == "" {
		t.Fatalf("no access_token in response: %v", body)
	}
	return tok
}

func uniqueEmail(prefix string) string {
	return fmt.Sprintf("%s-%s@example.kz", prefix, uuid.NewString()[:8])
}

// mustLoginRefresh registers nothing; it logs in and returns the refresh token.
func mustLoginRefresh(t *testing.T, c *client, email, password string) string {
	t.Helper()
	status, _, refresh := c.login(t, email, password)
	if status != http.StatusOK || refresh == "" {
		t.Fatalf("login for refresh: status=%d refresh=%q", status, refresh)
	}
	return refresh
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestRegister(t *testing.T) {
	c := newClient(t)
	email := uniqueEmail("reg")

	status, body, refresh := c.register(t, email, "Password-1234!", "Test User", "Org One", "buyer")
	if status != http.StatusCreated {
		t.Fatalf("register status = %d, body = %v", status, body)
	}
	if body["role"] != "owner" {
		t.Errorf("role = %v, want owner", body["role"])
	}
	if refresh == "" {
		t.Errorf("register should set a refresh cookie")
	}
	accessToken(t, body)

	// Duplicate email → 409 EMAIL_TAKEN.
	status, body, _ = c.register(t, email, "Password-1234!", "Test User", "Org Two", "buyer")
	if status != http.StatusConflict || body["code"] != "EMAIL_TAKEN" {
		t.Fatalf("duplicate register: status=%d body=%v", status, body)
	}

	// Transaction rollback: the failed register must not have created Org Two.
	var n int
	if err := adminPool.QueryRow(ctx, "SELECT count(*) FROM organizations WHERE name = $1", "Org Two").Scan(&n); err != nil {
		t.Fatalf("count orgs: %v", err)
	}
	if n != 0 {
		t.Errorf("rollback violated: %d rows for Org Two", n)
	}

	// Validation failures.
	status, body, _ = c.register(t, "not-an-email", "Password-1234!", "X", "Org", "buyer")
	if status != http.StatusBadRequest || body["code"] != "VALIDATION_ERROR" {
		t.Errorf("bad email: status=%d body=%v", status, body)
	}
	status, body, _ = c.register(t, uniqueEmail("short"), "short", "X", "Org", "buyer")
	if status != http.StatusBadRequest || body["code"] != "VALIDATION_ERROR" {
		t.Errorf("short password: status=%d body=%v", status, body)
	}
	status, body, _ = c.register(t, uniqueEmail("badtype"), "Password-1234!", "X", "Org", "notatype")
	if status != http.StatusBadRequest || body["code"] != "VALIDATION_ERROR" {
		t.Errorf("bad org_type: status=%d body=%v", status, body)
	}
}

func TestLogin(t *testing.T) {
	c := newClient(t)
	email := uniqueEmail("login")
	password := "Password-1234!"
	c.register(t, email, password, "Login User", "Login Org", "supplier")

	status, body, refresh := c.login(t, email, password)
	if status != http.StatusOK {
		t.Fatalf("login status = %d, body = %v", status, body)
	}
	if body["role"] != "owner" {
		t.Errorf("role = %v", body["role"])
	}
	if refresh == "" {
		t.Errorf("login should set a refresh cookie")
	}
	at := accessToken(t, body)
	if _, err := uuid.Parse(at); err == nil {
		t.Errorf("access_token should be a JWT, got %q", at)
	}

	// Wrong password.
	status, body, _ = c.login(t, email, "Wrong-Password-1")
	if status != http.StatusUnauthorized || body["code"] != "INVALID_CREDENTIALS" {
		t.Errorf("wrong password: status=%d body=%v", status, body)
	}

	// Unknown email.
	status, body, _ = c.login(t, uniqueEmail("ghost"), password)
	if status != http.StatusUnauthorized || body["code"] != "INVALID_CREDENTIALS" {
		t.Errorf("unknown email: status=%d body=%v", status, body)
	}
}

func TestRefreshRotationAndReuse(t *testing.T) {
	c := newClient(t)
	email := uniqueEmail("rot")
	status, _, _ := c.register(t, email, "Password-1234!", "Rotate User", "Rotate Org", "buyer")
	if status != http.StatusCreated {
		t.Fatalf("register status = %d", status)
	}
	r1 := mustLoginRefresh(t, c, email, "Password-1234!")

	// First refresh rotates R1 → R2.
	status, body, r2 := c.refresh(t, r1)
	if status != http.StatusOK {
		t.Fatalf("refresh status = %d body = %v", status, body)
	}
	if r2 == "" || r2 == r1 {
		t.Fatalf("expected a rotated refresh token, got r1=%q r2=%q", r1, r2)
	}
	at := accessToken(t, body)

	// The new access token works.
	if meStatus, _ := c.me(t, at); meStatus != http.StatusOK {
		t.Errorf("/me with rotated token status = %d", meStatus)
	}

	// Reusing R1 → reuse detected, session revoked.
	status, body, _ = c.refresh(t, r1)
	if status != http.StatusUnauthorized || body["code"] != "REFRESH_TOKEN_REUSE" {
		t.Errorf("reuse: status=%d body=%v", status, body)
	}

	// R2 is now revoked too (session family revocation).
	status, body, _ = c.refresh(t, r2)
	if status != http.StatusUnauthorized || body["code"] != "INVALID_REFRESH_TOKEN" {
		t.Errorf("refresh after reuse: status=%d body=%v", status, body)
	}
}

func TestConcurrentRefresh(t *testing.T) {
	c := newClient(t)
	email := uniqueEmail("conc")
	c.register(t, email, "Password-1234!", "Conc User", "Conc Org", "buyer")
	r1 := mustLoginRefresh(t, c, email, "Password-1234!")

	const n = 8
	statuses := make(chan int, n)
	for i := 0; i < n; i++ {
		go func() {
			s, _ := c.refreshRaw(r1)
			statuses <- s
		}()
	}

	successes, reuse := 0, 0
	for i := 0; i < n; i++ {
		switch <-statuses {
		case http.StatusOK:
			successes++
		case http.StatusUnauthorized:
			reuse++
		default:
			t.Errorf("unexpected status")
		}
	}
	if successes != 1 {
		t.Errorf("concurrent refresh: successes = %d, want exactly 1", successes)
	}
	if reuse != n-1 {
		t.Errorf("concurrent refresh: reuse = %d, want %d", reuse, n-1)
	}
}

func TestLogout(t *testing.T) {
	c := newClient(t)
	email := uniqueEmail("out")
	c.register(t, email, "Password-1234!", "Logout User", "Logout Org", "buyer")
	r1 := mustLoginRefresh(t, c, email, "Password-1234!")

	status, err := c.logout(t, r1)
	if err != nil {
		t.Fatalf("logout: %v", err)
	}
	if status != http.StatusNoContent {
		t.Errorf("logout status = %d", status)
	}

	// Session revoked → refresh fails.
	status, body, _ := c.refresh(t, r1)
	if status != http.StatusUnauthorized {
		t.Errorf("refresh after logout: status=%d body=%v", status, body)
	}

	// Logout is idempotent.
	if status, _ := c.logout(t, r1); status != http.StatusNoContent {
		t.Errorf("second logout status = %d", status)
	}
}

func TestExpiredAndInvalidJWT(t *testing.T) {
	c := newClient(t)
	email := uniqueEmail("jwt")
	c.register(t, email, "Password-1234!", "JWT User", "JWT Org", "buyer")
	_, body, _ := c.login(t, email, "Password-1234!")
	valid := accessToken(t, body)

	// Expired token signed with the correct secret.
	expired := signToken(t, cfg.Auth.AccessSecret, time.Now().Add(-time.Hour))
	if status, _ := c.me(t, expired); status != http.StatusUnauthorized {
		t.Errorf("expired token status = %d, want 401", status)
	}

	// Token signed with a different secret.
	forged := signToken(t, "a-different-secret-0123456789abcdef0123456789", time.Now().Add(time.Hour))
	if status, _ := c.me(t, forged); status != http.StatusUnauthorized {
		t.Errorf("forged token status = %d, want 401", status)
	}

	// Tampered valid token.
	parts := strings.Split(valid, ".")
	if len(parts) == 3 {
		sig := parts[2]
		if sig[0] == 'a' {
			sig = "b" + sig[1:]
		} else {
			sig = "a" + sig[1:]
		}
		tampered := parts[0] + "." + parts[1] + "." + sig
		if status, _ := c.me(t, tampered); status != http.StatusUnauthorized {
			t.Errorf("tampered token status = %d, want 401", status)
		}
	}

	// No token at all.
	if status, _ := c.me(t, ""); status != http.StatusUnauthorized {
		t.Errorf("no token status = %d, want 401", status)
	}
}

func TestRBAC(t *testing.T) {
	c := newClient(t)

	// Org A owned by Alice.
	alice := uniqueEmail("alice")
	_, ownerBody, _ := c.register(t, alice, "Password-1234!", "Alice", "RBAC Org A", "buyer")
	ownerToken := accessToken(t, ownerBody)

	// Org B owned by Charlie.
	charlie := uniqueEmail("charlie")
	c.register(t, charlie, "Password-1234!", "Charlie", "RBAC Org B", "buyer")

	// Bob registers (his own org) then joins Org A as viewer.
	bob := uniqueEmail("bob")
	_, bobReg, _ := c.register(t, bob, "Password-1234!", "Bob", "Bob's Own Org", "buyer")
	bobID, _ := uuid.Parse(bobReg["user"].(map[string]any)["user_id"].(string))
	orgA := ownerBody["org"].(map[string]any)["org_id"].(string)
	orgAUUID, _ := uuid.Parse(orgA)
	mustAddMembership(t, orgAUUID, bobID, mustRoleID(t, orgAUUID, "viewer"))

	// Router protected by RequirePermission("members.manage").
	orgs := organizations.NewService(appPool, zerolog.Nop())
	protected := chi.NewRouter()
	protected.Use(api.Identity().Authenticate)
	protected.Use(identity.RequirePermission("members.manage", orgs))
	protected.Get("/", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	psrv := httptest.NewServer(protected)
	t.Cleanup(psrv.Close)

	get := func(tok string) int {
		t.Helper()
		req, _ := http.NewRequest(http.MethodGet, psrv.URL+"/", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("protected request: %v", err)
		}
		defer res.Body.Close()
		_, _ = io.Copy(io.Discard, res.Body)
		return res.StatusCode
	}

	if got := get(ownerToken); got != http.StatusOK {
		t.Errorf("owner access status = %d, want 200", got)
	}

	// Bob explicitly logs into Org A (viewer) → 403.
	_, bobViewer, _ := c.loginWithOrg(t, bob, "Password-1234!", orgA)
	if got := get(accessToken(t, bobViewer)); got != http.StatusForbidden {
		t.Errorf("viewer access status = %d, want 403", got)
	}

	// Charlie (owner of Org B) passes inside his own org.
	_, charlieLogin, _ := c.login(t, charlie, "Password-1234!")
	if got := get(accessToken(t, charlieLogin)); got != http.StatusOK {
		t.Errorf("charlie (owner, own org) status = %d, want 200", got)
	}
}

func TestCrossTenantAccess(t *testing.T) {
	c := newClient(t)

	_, orgABody, _ := c.register(t, uniqueEmail("xta"), "Password-1234!", "Tenant A", "Tenant Org A", "buyer")
	_, orgBBody, _ := c.register(t, uniqueEmail("xtb"), "Password-1234!", "Tenant B", "Tenant Org B", "buyer")

	userA := orgABody["user"].(map[string]any)["user_id"].(string)
	orgB := orgBBody["org"].(map[string]any)["org_id"].(string)

	// User A's /me shows only Org A.
	_, aLogin, _ := c.login(t, orgABody["user"].(map[string]any)["email"].(string), "Password-1234!")
	meStatus, meBody := c.me(t, accessToken(t, aLogin))
	if meStatus != http.StatusOK {
		t.Fatalf("/me status = %d", meStatus)
	}
	if meBody["org"].(map[string]any)["org_id"] == orgB {
		t.Errorf("/me leaked org B to user A: %v", meBody["org"])
	}

	// Forge a token for user A claiming org B: /me must fail (no membership
	// in org B, RLS + membership check).
	userAUUID, _ := uuid.Parse(userA)
	orgBUUID, _ := uuid.Parse(orgB)
	forged := mustSignClaims(t, cfg.Auth.AccessSecret, userAUUID, orgBUUID, uuid.New(), "owner", time.Now().Add(time.Hour))
	status, body := c.me(t, forged)
	if status != http.StatusForbidden {
		t.Errorf("forged cross-tenant /me status = %d body=%v, want 403", status, body)
	}
}

func TestRLSIsolation(t *testing.T) {
	c := newClient(t)
	_, ab, _ := c.register(t, uniqueEmail("rlsa"), "Password-1234!", "RLS A", "RLS Org A", "buyer")
	_, bb, _ := c.register(t, uniqueEmail("rlsb"), "Password-1234!", "RLS B", "RLS Org B", "buyer")

	orgA := ab["org"].(map[string]any)["org_id"].(string)
	orgB := bb["org"].(map[string]any)["org_id"].(string)
	userB := bb["user"].(map[string]any)["user_id"].(string)

	// As the low-privilege app role with tenant A configured, only A's rows
	// are visible; B's membership is invisible (RLS).
	tx, err := appPool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, err := tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", orgA); err != nil {
		t.Fatalf("set tenant: %v", err)
	}
	var visible int
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM memberships").Scan(&visible); err != nil {
		t.Fatalf("count memberships: %v", err)
	}
	if visible != 1 {
		t.Errorf("tenant %s sees %d memberships, want 1", orgA, visible)
	}
	var cross int
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM memberships WHERE user_id = $1", userB).Scan(&cross); err != nil {
		t.Fatalf("cross-tenant query: %v", err)
	}
	if cross != 0 {
		t.Errorf("tenant %s read %d rows belonging to tenant %s (cross-tenant leak)", orgA, cross, orgB)
	}
	_ = tx.Rollback(ctx)

	// Without a tenant setting, tenant-scoped tables fail closed: RLS casts
	// the unset current_setting to uuid, which errors rather than leaking rows.
	tx2, err := appPool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx2.Rollback(ctx) }()
	if _, err := tx2.Exec(ctx, "SELECT count(*) FROM memberships"); err == nil {
		t.Errorf("tenant-scoped query without tenant context succeeded; want RLS error")
	} else if !strings.Contains(err.Error(), "22P02") {
		t.Errorf("unexpected error without tenant context: %v", err)
	}
}

func TestLoginThrottle(t *testing.T) {
	c := newClient(t)
	email := uniqueEmail("throttle")
	c.register(t, email, "Password-1234!", "Throttle User", "Throttle Org", "buyer")

	for i := 0; i < 5; i++ {
		status, _, _ := c.login(t, email, "Wrong-Password-1")
		if status != http.StatusUnauthorized {
			t.Fatalf("attempt %d status = %d, want 401", i, status)
		}
	}
	status, body, _ := c.login(t, email, "Wrong-Password-1")
	if status != http.StatusTooManyRequests || body["code"] != "RATE_LIMITED" {
		t.Errorf("throttled status = %d body = %v, want 429 RATE_LIMITED", status, body)
	}
	// Even a correct password is blocked while throttled.
	if status, _, _ := c.login(t, email, "Password-1234!"); status != http.StatusTooManyRequests {
		t.Errorf("throttled correct-password status = %d, want 429", status)
	}

	// Failed attempts were audited.
	var n int
	if err := adminPool.QueryRow(ctx,
		"SELECT count(*) FROM auth_events WHERE action = 'login_failed'").Scan(&n); err != nil {
		t.Fatalf("count auth_events: %v", err)
	}
	if n < 5 {
		t.Errorf("auth_events login_failed = %d, want >= 5", n)
	}
}

// ---------------------------------------------------------------------------
// DB + token helpers
// ---------------------------------------------------------------------------

func mustRoleID(t *testing.T, orgID uuid.UUID, name string) uuid.UUID {
	t.Helper()
	// Query as admin (BYPASSRLS): RLS requires a tenant context, which plain
	// admin-style setup queries don't have.
	role, err := gen.New(adminPool).GetRoleByOrgName(ctx, gen.GetRoleByOrgNameParams{OrgID: orgID, Name: name})
	if err != nil {
		t.Fatalf("get role %s: %v", name, err)
	}
	return role.RoleID
}

func mustAddMembership(t *testing.T, orgID, userID, roleID uuid.UUID) {
	t.Helper()
	// Insert as admin (BYPASSRLS) — the RBAC-protected membership API would do
	// this for a real admin.
	if _, err := adminPool.Exec(ctx,
		`INSERT INTO memberships (org_id, user_id, role_id) VALUES ($1, $2, $3)`, orgID, userID, roleID); err != nil {
		t.Fatalf("add membership: %v", err)
	}
}

func signToken(t *testing.T, secret string, expiresAt time.Time) string {
	t.Helper()
	return mustSignClaims(t, secret, uuid.New(), uuid.New(), uuid.New(), "owner", expiresAt)
}

func mustSignClaims(t *testing.T, secret string, userID, orgID, sessionID uuid.UUID, role string, expiresAt time.Time) string {
	t.Helper()
	claims := identity.Claims{
		OrgID: orgID.String(), Role: role, SessionID: sessionID.String(),
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: userID.String(), IssuedAt: jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(expiresAt), ID: uuid.NewString(),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signed
}