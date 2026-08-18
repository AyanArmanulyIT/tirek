package integration

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// Restaurants
// ---------------------------------------------------------------------------

func (c *client) createRestaurant(t *testing.T, token, body string) (int, map[string]any) {
	t.Helper()
	status, data, _, err := c.do(http.MethodPost, "/api/v1/restaurants", body, token, nil)
	if err != nil {
		t.Fatalf("create restaurant: %v", err)
	}
	return status, decodeBody(t, data)
}

func (c *client) getRestaurant(t *testing.T, token, id string) (int, map[string]any) {
	t.Helper()
	status, data, _, err := c.do(http.MethodGet, "/api/v1/restaurants/"+id, "", token, nil)
	if err != nil {
		t.Fatalf("get restaurant: %v", err)
	}
	return status, decodeBody(t, data)
}

func (c *client) patchRestaurant(t *testing.T, token, id, body string) (int, map[string]any) {
	t.Helper()
	status, data, _, err := c.do(http.MethodPatch, "/api/v1/restaurants/"+id, body, token, nil)
	if err != nil {
		t.Fatalf("patch restaurant: %v", err)
	}
	return status, decodeBody(t, data)
}

func (c *client) listRestaurants(t *testing.T, token string) (int, map[string]any) {
	t.Helper()
	status, data, _, err := c.do(http.MethodGet, "/api/v1/restaurants", "", token, nil)
	if err != nil {
		t.Fatalf("list restaurants: %v", err)
	}
	return status, decodeBody(t, data)
}

func (c *client) createLocation(t *testing.T, token, restaurantID, body string) (int, map[string]any) {
	t.Helper()
	status, data, _, err := c.do(http.MethodPost, "/api/v1/restaurants/"+restaurantID+"/locations", body, token, nil)
	if err != nil {
		t.Fatalf("create location: %v", err)
	}
	return status, decodeBody(t, data)
}

func (c *client) listLocations(t *testing.T, token, restaurantID string) (int, map[string]any) {
	t.Helper()
	status, data, _, err := c.do(http.MethodGet, "/api/v1/restaurants/"+restaurantID+"/locations", "", token, nil)
	if err != nil {
		t.Fatalf("list locations: %v", err)
	}
	return status, decodeBody(t, data)
}

func (c *client) patchLocation(t *testing.T, token, restaurantID, locationID, body string) (int, map[string]any) {
	t.Helper()
	status, data, _, err := c.do(http.MethodPatch, "/api/v1/restaurants/"+restaurantID+"/locations/"+locationID, body, token, nil)
	if err != nil {
		t.Fatalf("patch location: %v", err)
	}
	return status, decodeBody(t, data)
}

func mustBuyer(t *testing.T, c *client, tag string) (token, orgID string) {
	t.Helper()
	status, body, _ := c.register(t, uniqueEmail(tag), "Password-1234!", "Buyer "+tag, "Buyer Org "+tag, "buyer")
	if status != http.StatusCreated {
		t.Fatalf("register buyer: status=%d body=%v", status, body)
	}
	return accessToken(t, body), body["org"].(map[string]any)["org_id"].(string)
}

func mustSupplier(t *testing.T, c *client, tag string) (token, orgID string) {
	t.Helper()
	status, body, _ := c.register(t, uniqueEmail(tag), "Password-1234!", "Supplier "+tag, "Supplier Org "+tag, "supplier")
	if status != http.StatusCreated {
		t.Fatalf("register supplier: status=%d body=%v", status, body)
	}
	return accessToken(t, body), body["org"].(map[string]any)["org_id"].(string)
}

func mustCreateRestaurant(t *testing.T, c *client, token string) string {
	t.Helper()
	status, body := c.createRestaurant(t, token, `{"name":"Test Restaurant","legal_name":"Test Restaurant LLP","status":"active","country":"KZ","default_currency":"KZT","phone":"+77001112233","email":"tr@example.kz"}`)
	if status != http.StatusCreated {
		t.Fatalf("create restaurant: status=%d body=%v", status, body)
	}
	id, _ := body["restaurant_id"].(string)
	if id == "" {
		t.Fatalf("create restaurant: no restaurant_id in %v", body)
	}
	return id
}

func TestRestaurantCRUD(t *testing.T) {
	c := newClient(t)
	token, _ := mustBuyer(t, c, "crud")

	// Create.
	status, body := c.createRestaurant(t, token, `{"name":"CRUD Restaurant","legal_name":"CRUD LLP","status":"active","country":"KZ","default_currency":"KZT","phone":"+77000000001","email":"crud@example.kz"}`)
	if status != http.StatusCreated {
		t.Fatalf("create status=%d body=%v", status, body)
	}
	id := body["restaurant_id"].(string)
	if body["name"] != "CRUD Restaurant" || body["status"] != "active" || body["org_id"] == "" {
		t.Errorf("unexpected restaurant body: %v", body)
	}

	// Read.
	if s, b := c.getRestaurant(t, token, id); s != http.StatusOK || b["restaurant_id"] != id {
		t.Errorf("get status=%d body=%v", s, b)
	}

	// List.
	if s, b := c.listRestaurants(t, token); s != http.StatusOK {
		t.Errorf("list status=%d body=%v", s, b)
	} else if arr, ok := b["data"].([]any); !ok || len(arr) != 1 {
		t.Errorf("list data = %v, want 1 item", b["data"])
	}

	// Update.
	status, body = c.patchRestaurant(t, token, id, `{"name":"CRUD Updated","legal_name":"CRUD LLP","status":"suspended","country":"KZ","default_currency":"KZT"}`)
	if status != http.StatusOK {
		t.Fatalf("patch status=%d body=%v", status, body)
	}
	if body["name"] != "CRUD Updated" || body["status"] != "suspended" {
		t.Errorf("patch body = %v", body)
	}

	// Location create + list + update.
	status, body = c.createLocation(t, token, id, `{"name":"Downtown","address":"Abay 10","city":"Almaty","country":"KZ","status":"active"}`)
	if status != http.StatusCreated {
		t.Fatalf("create location status=%d body=%v", status, body)
	}
	locID := body["location_id"].(string)

	if s, b := c.listLocations(t, token, id); s != http.StatusOK {
		t.Errorf("list locations status=%d body=%v", s, b)
	} else if arr, ok := b["data"].([]any); !ok || len(arr) != 1 {
		t.Errorf("locations data = %v, want 1", b["data"])
	}

	status, body = c.patchLocation(t, token, id, locID, `{"name":"Downtown East","address":"Abay 20","city":"Almaty","country":"KZ","status":"inactive"}`)
	if status != http.StatusOK || body["name"] != "Downtown East" || body["status"] != "inactive" {
		t.Errorf("patch location status=%d body=%v", status, body)
	}
}

func TestRestaurantValidation(t *testing.T) {
	c := newClient(t)
	token, _ := mustBuyer(t, c, "valid")

	cases := []struct {
		name string
		body string
	}{
		{"empty name", `{"name":"","status":"active","country":"KZ","default_currency":"KZT"}`},
		{"bad status", `{"name":"X","status":"weird","country":"KZ","default_currency":"KZT"}`},
		{"bad country", `{"name":"X","status":"active","country":"KZ2","default_currency":"KZT"}`},
		{"bad currency", `{"name":"X","status":"active","country":"KZ","default_currency":"kzt"}`},
		{"bad email", `{"name":"X","status":"active","country":"KZ","default_currency":"KZT","email":"nope"}`},
		{"malformed json", `{"name":`},
	}
	for _, tc := range cases {
		status, body := c.createRestaurant(t, token, tc.body)
		if status != http.StatusBadRequest || body["code"] != "VALIDATION_ERROR" && body["code"] != "MALFORMED_REQUEST" {
			t.Errorf("%s: status=%d body=%v", tc.name, status, body)
		}
	}
}

func TestRestaurantOrgTypeRestriction(t *testing.T) {
	c := newClient(t)
	supToken, _ := mustSupplier(t, c, "orgtype")

	status, body := c.createRestaurant(t, supToken, `{"name":"X","status":"active","country":"KZ","default_currency":"KZT"}`)
	if status != 422 || body["code"] != "ORG_TYPE_MISMATCH" {
		t.Errorf("supplier creating restaurant: status=%d body=%v, want 422 ORG_TYPE_MISMATCH", status, body)
	}
	status, body = c.listRestaurants(t, supToken)
	if status != 422 || body["code"] != "ORG_TYPE_MISMATCH" {
		t.Errorf("supplier listing restaurants: status=%d body=%v, want 422 ORG_TYPE_MISMATCH", status, body)
	}
}

func TestRestaurantRBAC(t *testing.T) {
	c := newClient(t)
	_, orgA := mustBuyer(t, c, "rbac")

	// Bob joins Org A as viewer (read-only).
	bobEmail := uniqueEmail("rbacbob")
	_, bobReg, _ := c.register(t, bobEmail, "Password-1234!", "Bob", "Bob Org", "buyer")
	bobID, _ := uuid.Parse(bobReg["user"].(map[string]any)["user_id"].(string))
	orgAUUID, _ := uuid.Parse(orgA)
	mustAddMembership(t, orgAUUID, bobID, mustRoleID(t, orgAUUID, "viewer"))
	_, bobViewer, _ := c.loginWithOrg(t, bobEmail, "Password-1234!", orgA)
	bobToken := accessToken(t, bobViewer)

	// Viewer can read.
	status, _ := c.listRestaurants(t, bobToken)
	if status != http.StatusOK {
		t.Errorf("viewer list status=%d, want 200", status)
	}

	// Viewer cannot write.
	status, body := c.createRestaurant(t, bobToken, `{"name":"X","status":"active","country":"KZ","default_currency":"KZT"}`)
	if status != http.StatusForbidden || body["code"] != "FORBIDDEN" {
		t.Errorf("viewer create status=%d body=%v, want 403 FORBIDDEN", status, body)
	}
}

func TestRestaurantCrossTenantAccess(t *testing.T) {
	c := newClient(t)
	orgAToken, _ := mustBuyer(t, c, "xorga")
	orgBToken, _ := mustBuyer(t, c, "xorgb")

	restID := mustCreateRestaurant(t, c, orgAToken)
	locStatus, locBody := c.createLocation(t, orgAToken, restID, `{"name":"A1","address":"Addr 1","city":"City","country":"KZ","status":"active"}`)
	if locStatus != http.StatusCreated {
		t.Fatalf("org A create location: %d %v", locStatus, locBody)
	}
	locID := locBody["location_id"].(string)

	// Org B cannot read/patch org A's restaurant.
	if s, b := c.getRestaurant(t, orgBToken, restID); s != http.StatusNotFound {
		t.Errorf("cross-tenant get status=%d body=%v, want 404", s, b)
	}
	if s, _ := c.patchRestaurant(t, orgBToken, restID, `{"name":"Hack","status":"active","country":"KZ","default_currency":"KZT"}`); s != http.StatusNotFound {
		t.Errorf("cross-tenant patch status=%d, want 404", s)
	}

	// Org B cannot list org A's locations or patch them.
	if s, b := c.listLocations(t, orgBToken, restID); s != http.StatusNotFound {
		t.Errorf("cross-tenant list locations status=%d body=%v, want 404", s, b)
	}
	if s, _ := c.patchLocation(t, orgBToken, restID, locID, `{"name":"Hack","address":"X","city":"Y","country":"KZ","status":"active"}`); s != http.StatusNotFound {
		t.Errorf("cross-tenant patch location status=%d, want 404", s)
	}
}

func TestRestaurantForgedCrossTenantToken(t *testing.T) {
	c := newClient(t)
	_, orgABody, _ := c.register(t, uniqueEmail("forgea"), "Password-1234!", "Forge A", "Forge Org A", "buyer")
	_, orgBBody, _ := c.register(t, uniqueEmail("forgeb"), "Password-1234!", "Forge B", "Forge Org B", "buyer")

	userA, _ := uuid.Parse(orgABody["user"].(map[string]any)["user_id"].(string))
	orgB, _ := uuid.Parse(orgBBody["org"].(map[string]any)["org_id"].(string))
	forged := mustSignClaims(t, cfg.Auth.AccessSecret, userA, orgB, uuid.New(), "owner", time.Now().Add(time.Hour))

	// User A is not a member of org B → create fails with 403 (forged ID fails
	// safely) even though the token is signed with the server secret.
	status, body := c.createRestaurant(t, forged, `{"name":"X","status":"active","country":"KZ","default_currency":"KZT"}`)
	if status != http.StatusForbidden {
		t.Errorf("forged cross-tenant create status=%d body=%v, want 403", status, body)
	}
}

func TestRestaurantRLSIsolation(t *testing.T) {
	c := newClient(t)
	orgAToken, orgA := mustBuyer(t, c, "rlsa")
	_, orgB := mustBuyer(t, c, "rlsb")

	mustCreateRestaurant(t, c, orgAToken)

	// As the app role with tenant A, only A's restaurants are visible.
	tx, err := appPool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, err := tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", orgA); err != nil {
		t.Fatalf("set tenant: %v", err)
	}
	var visible int
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM restaurants").Scan(&visible); err != nil {
		t.Fatalf("count restaurants: %v", err)
	}
	if visible != 1 {
		t.Errorf("tenant %s sees %d restaurants, want 1", orgA, visible)
	}
	var cross int
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM restaurants WHERE org_id = $1", orgB).Scan(&cross); err != nil {
		t.Fatalf("cross-tenant query: %v", err)
	}
	if cross != 0 {
		t.Errorf("tenant %s read %d rows of tenant %s (leak)", orgA, cross, orgB)
	}
	_ = tx.Rollback(ctx)

	// Missing tenant context fails closed (RLS casts NULL::uuid → error).
	tx2, err := appPool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx2.Rollback(ctx) }()
	if _, err := tx2.Exec(ctx, "SELECT count(*) FROM restaurants"); err == nil {
		t.Errorf("restaurant query without tenant context succeeded; want RLS error")
	} else if !strings.Contains(err.Error(), "22P02") {
		t.Errorf("unexpected error without tenant context: %v", err)
	}
}
