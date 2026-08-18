package integration

import (
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func mustParseUUID(t *testing.T, s string) uuid.UUID {
	t.Helper()
	id, err := uuid.Parse(s)
	if err != nil {
		t.Fatalf("parse uuid %q: %v", s, err)
	}
	return id
}

// ---------------------------------------------------------------------------
// Suppliers
// ---------------------------------------------------------------------------

func (c *client) createSupplier(t *testing.T, token, body string) (int, map[string]any) {
	t.Helper()
	status, data, _, err := c.do(http.MethodPost, "/api/v1/suppliers", body, token, nil)
	if err != nil {
		t.Fatalf("create supplier: %v", err)
	}
	return status, decodeBody(t, data)
}

func (c *client) getSupplier(t *testing.T, token, id string) (int, map[string]any) {
	t.Helper()
	status, data, _, err := c.do(http.MethodGet, "/api/v1/suppliers/"+id, "", token, nil)
	if err != nil {
		t.Fatalf("get supplier: %v", err)
	}
	return status, decodeBody(t, data)
}

func (c *client) patchSupplier(t *testing.T, token, id, body string) (int, map[string]any) {
	t.Helper()
	status, data, _, err := c.do(http.MethodPatch, "/api/v1/suppliers/"+id, body, token, nil)
	if err != nil {
		t.Fatalf("patch supplier: %v", err)
	}
	return status, decodeBody(t, data)
}

func (c *client) listSuppliers(t *testing.T, token string) (int, map[string]any) {
	t.Helper()
	status, data, _, err := c.do(http.MethodGet, "/api/v1/suppliers", "", token, nil)
	if err != nil {
		t.Fatalf("list suppliers: %v", err)
	}
	return status, decodeBody(t, data)
}

func mustCreateSupplier(t *testing.T, c *client, token string) string {
	t.Helper()
	status, body := c.createSupplier(t, token, `{"name":"Test Supplier","legal_name":"Test Supplier LLP","status":"active","country":"KZ","default_currency":"KZT","phone":"+77001112233","email":"ts@example.kz"}`)
	if status != http.StatusCreated {
		t.Fatalf("create supplier: status=%d body=%v", status, body)
	}
	id, _ := body["supplier_id"].(string)
	if id == "" {
		t.Fatalf("create supplier: no supplier_id in %v", body)
	}
	return id
}

func TestSupplierCRUD(t *testing.T) {
	c := newClient(t)
	token, _ := mustSupplier(t, c, "scrud")

	status, body := c.createSupplier(t, token, `{"name":"SCrud Foods","legal_name":"SCrud Foods LLP","status":"active","country":"KZ","default_currency":"KZT","phone":"+77000000002","email":"scrud@example.kz"}`)
	if status != http.StatusCreated {
		t.Fatalf("create status=%d body=%v", status, body)
	}
	id := body["supplier_id"].(string)
	if body["name"] != "SCrud Foods" || body["status"] != "active" || body["org_id"] == "" {
		t.Errorf("unexpected supplier body: %v", body)
	}

	if s, b := c.getSupplier(t, token, id); s != http.StatusOK || b["supplier_id"] != id {
		t.Errorf("get status=%d body=%v", s, b)
	}

	if s, b := c.listSuppliers(t, token); s != http.StatusOK {
		t.Errorf("list status=%d body=%v", s, b)
	} else if arr, ok := b["data"].([]any); !ok || len(arr) != 1 {
		t.Errorf("list data = %v, want 1 item", b["data"])
	}

	status, body = c.patchSupplier(t, token, id, `{"name":"SCrud Foods Updated","legal_name":"SCrud Foods LLP","status":"suspended","country":"KZ","default_currency":"KZT"}`)
	if status != http.StatusOK || body["name"] != "SCrud Foods Updated" || body["status"] != "suspended" {
		t.Errorf("patch status=%d body=%v", status, body)
	}
}

func TestSupplierValidation(t *testing.T) {
	c := newClient(t)
	token, _ := mustSupplier(t, c, "svalid")

	cases := []struct {
		name string
		body string
	}{
		{"empty name", `{"name":"","status":"active","country":"KZ","default_currency":"KZT"}`},
		{"bad status", `{"name":"X","status":"weird","country":"KZ","default_currency":"KZT"}`},
		{"bad country", `{"name":"X","status":"active","country":"KZ2","default_currency":"KZT"}`},
		{"bad currency", `{"name":"X","status":"active","country":"KZ","default_currency":"kzt"}`},
		{"malformed json", `{"name":`},
	}
	for _, tc := range cases {
		status, body := c.createSupplier(t, token, tc.body)
		if status != http.StatusBadRequest {
			t.Errorf("%s: status=%d body=%v", tc.name, status, body)
		}
	}
}

func TestSupplierOrgTypeRestriction(t *testing.T) {
	c := newClient(t)
	buyerToken, _ := mustBuyer(t, c, "sorgtype")

	status, body := c.createSupplier(t, buyerToken, `{"name":"X","status":"active","country":"KZ","default_currency":"KZT"}`)
	if status != 422 || body["code"] != "ORG_TYPE_MISMATCH" {
		t.Errorf("buyer creating supplier: status=%d body=%v, want 422 ORG_TYPE_MISMATCH", status, body)
	}
	status, body = c.listSuppliers(t, buyerToken)
	if status != 422 || body["code"] != "ORG_TYPE_MISMATCH" {
		t.Errorf("buyer listing suppliers: status=%d body=%v, want 422 ORG_TYPE_MISMATCH", status, body)
	}
}

func TestSupplierProfileOnePerOrg(t *testing.T) {
	c := newClient(t)
	token, _ := mustSupplier(t, c, "sdup")
	mustCreateSupplier(t, c, token)

	status, body := c.createSupplier(t, token, `{"name":"Second","legal_name":"Second LLP","status":"active","country":"KZ","default_currency":"KZT"}`)
	if status != http.StatusConflict || body["code"] != "SUPPLIER_PROFILE_EXISTS" {
		t.Errorf("duplicate supplier: status=%d body=%v, want 409 SUPPLIER_PROFILE_EXISTS", status, body)
	}
}

func TestSupplierRBAC(t *testing.T) {
	c := newClient(t)
	_, orgA := mustSupplier(t, c, "srbac")

	bobEmail := uniqueEmail("srbacbob")
	_, bobReg, _ := c.register(t, bobEmail, "Password-1234!", "Bob", "Bob Org", "supplier")
	bobID := mustParseUUID(t, bobReg["user"].(map[string]any)["user_id"].(string))
	orgAUUID := mustParseUUID(t, orgA)
	mustAddMembership(t, orgAUUID, bobID, mustRoleID(t, orgAUUID, "viewer"))
	_, bobViewer, _ := c.loginWithOrg(t, bobEmail, "Password-1234!", orgA)
	bobToken := accessToken(t, bobViewer)

	if s, _ := c.listSuppliers(t, bobToken); s != http.StatusOK {
		t.Errorf("viewer list suppliers status=%d, want 200", s)
	}
	status, body := c.createSupplier(t, bobToken, `{"name":"X","status":"active","country":"KZ","default_currency":"KZT"}`)
	if status != http.StatusForbidden || body["code"] != "FORBIDDEN" {
		t.Errorf("viewer create supplier status=%d body=%v, want 403 FORBIDDEN", status, body)
	}
}

func TestSupplierCrossTenantAccess(t *testing.T) {
	c := newClient(t)
	orgAToken, _ := mustSupplier(t, c, "sxorga")
	orgBToken, _ := mustSupplier(t, c, "sxorgb")

	supID := mustCreateSupplier(t, c, orgAToken)

	if s, b := c.getSupplier(t, orgBToken, supID); s != http.StatusNotFound {
		t.Errorf("cross-tenant get status=%d body=%v, want 404", s, b)
	}
	if s, _ := c.patchSupplier(t, orgBToken, supID, `{"name":"Hack","status":"active","country":"KZ","default_currency":"KZT"}`); s != http.StatusNotFound {
		t.Errorf("cross-tenant patch status=%d, want 404", s)
	}
}

func TestSupplierRLSIsolation(t *testing.T) {
	c := newClient(t)
	orgAToken, orgA := mustSupplier(t, c, "srlsa")
	_, orgB := mustSupplier(t, c, "srlsb")

	mustCreateSupplier(t, c, orgAToken)

	tx, err := appPool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, err := tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", orgA); err != nil {
		t.Fatalf("set tenant: %v", err)
	}
	var visible int
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM suppliers").Scan(&visible); err != nil {
		t.Fatalf("count suppliers: %v", err)
	}
	if visible != 1 {
		t.Errorf("tenant %s sees %d suppliers, want 1", orgA, visible)
	}
	var cross int
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM suppliers WHERE org_id = $1", orgB).Scan(&cross); err != nil {
		t.Fatalf("cross-tenant query: %v", err)
	}
	if cross != 0 {
		t.Errorf("tenant %s read %d rows of tenant %s (leak)", orgA, cross, orgB)
	}
	_ = tx.Rollback(ctx)

	tx2, err := appPool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx2.Rollback(ctx) }()
	if _, err := tx2.Exec(ctx, "SELECT count(*) FROM suppliers"); err == nil {
		t.Errorf("supplier query without tenant context succeeded; want RLS error")
	} else if !strings.Contains(err.Error(), "22P02") {
		t.Errorf("unexpected error without tenant context: %v", err)
	}
}
