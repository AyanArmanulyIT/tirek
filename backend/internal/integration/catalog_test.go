package integration

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// Catalog helpers
// ---------------------------------------------------------------------------

func (c *client) createCategory(t *testing.T, token, name string) (int, map[string]any) {
	t.Helper()
	status, data, _, err := c.do(http.MethodPost, "/api/v1/products/categories",
		fmt.Sprintf(`{"name":%q}`, name), token, nil)
	if err != nil {
		t.Fatalf("create category: %v", err)
	}
	return status, decodeBody(t, data)
}

func (c *client) listCategories(t *testing.T, token string) (int, map[string]any) {
	t.Helper()
	status, data, _, err := c.do(http.MethodGet, "/api/v1/products/categories", "", token, nil)
	if err != nil {
		t.Fatalf("list categories: %v", err)
	}
	return status, decodeBody(t, data)
}

func (c *client) createProduct(t *testing.T, token, body string) (int, map[string]any) {
	t.Helper()
	status, data, _, err := c.do(http.MethodPost, "/api/v1/products", body, token, nil)
	if err != nil {
		t.Fatalf("create product: %v", err)
	}
	return status, decodeBody(t, data)
}

func (c *client) getProduct(t *testing.T, token, id string) (int, map[string]any) {
	t.Helper()
	status, data, _, err := c.do(http.MethodGet, "/api/v1/products/"+id, "", token, nil)
	if err != nil {
		t.Fatalf("get product: %v", err)
	}
	return status, decodeBody(t, data)
}

func (c *client) listProducts(t *testing.T, token string) (int, map[string]any) {
	t.Helper()
	status, data, _, err := c.do(http.MethodGet, "/api/v1/products", "", token, nil)
	if err != nil {
		t.Fatalf("list products: %v", err)
	}
	return status, decodeBody(t, data)
}

func (c *client) patchProduct(t *testing.T, token, id, body string) (int, map[string]any) {
	t.Helper()
	status, data, _, err := c.do(http.MethodPatch, "/api/v1/products/"+id, body, token, nil)
	if err != nil {
		t.Fatalf("patch product: %v", err)
	}
	return status, decodeBody(t, data)
}

func (c *client) browseCatalog(t *testing.T, token, query string) (int, map[string]any) {
	t.Helper()
	status, data, _, err := c.do(http.MethodGet, "/api/v1/catalog/products"+query, "", token, nil)
	if err != nil {
		t.Fatalf("browse catalog: %v", err)
	}
	return status, decodeBody(t, data)
}

func (c *client) browseCategories(t *testing.T, token string) (int, map[string]any) {
	t.Helper()
	status, data, _, err := c.do(http.MethodGet, "/api/v1/catalog/categories", "", token, nil)
	if err != nil {
		t.Fatalf("browse categories: %v", err)
	}
	return status, decodeBody(t, data)
}

func mustCreateProduct(t *testing.T, c *client, token, name, unit, status string, priceMinor int64, currency string) string {
	t.Helper()
	code, body := c.createProduct(t, token, fmt.Sprintf(
		`{"name":%q,"unit":%q,"status":%q,"price_minor":%d,"currency":%q}`,
		name, unit, status, priceMinor, currency))
	if code != http.StatusCreated {
		t.Fatalf("create product: status=%d body=%v", code, body)
	}
	id, _ := body["product_id"].(string)
	if id == "" {
		t.Fatalf("create product: no product_id in %v", body)
	}
	return id
}

func priceOf(t *testing.T, body map[string]any) (float64, string) {
	t.Helper()
	p, ok := body["price"].(map[string]any)
	if !ok {
		t.Fatalf("no price in %v", body)
	}
	am, _ := p["amount_minor"].(float64)
	cur, _ := p["currency"].(string)
	return am, cur
}

// ---------------------------------------------------------------------------
// Catalog CRUD
// ---------------------------------------------------------------------------

func TestCatalogCRUD(t *testing.T) {
	c := newClient(t)
	token, _ := mustSupplier(t, c, "catcrud")

	// Create a category.
	status, body := c.createCategory(t, token, "Produce")
	if status != http.StatusCreated {
		t.Fatalf("create category status=%d body=%v", status, body)
	}
	catID := body["category_id"].(string)

	// Duplicate category name → 409.
	status, body = c.createCategory(t, token, "produce")
	if status != http.StatusConflict || body["code"] != "CATEGORY_NAME_TAKEN" {
		t.Errorf("duplicate category: status=%d body=%v, want 409 CATEGORY_NAME_TAKEN", status, body)
	}

	// List categories.
	if s, b := c.listCategories(t, token); s != http.StatusOK {
		t.Errorf("list categories status=%d body=%v", s, b)
	} else if arr, ok := b["data"].([]any); !ok || len(arr) != 1 {
		t.Errorf("categories data = %v, want 1", b["data"])
	}

	// Create a product with a price.
	status, body = c.createProduct(t, token, fmt.Sprintf(
		`{"name":"Tomatoes","sku":"TOM-001","description":"Fresh","unit":"kg","status":"active","category_id":%q,"price_minor":85000,"currency":"KZT"}`,
		catID))
	if status != http.StatusCreated {
		t.Fatalf("create product status=%d body=%v", status, body)
	}
	prodID := body["product_id"].(string)
	if body["name"] != "Tomatoes" || body["status"] != "active" || body["category_id"] != catID {
		t.Errorf("unexpected product body: %v", body)
	}
	if am, cur := priceOf(t, body); am != 85000 || cur != "KZT" {
		t.Errorf("product price = %v %v, want 85000 KZT", am, cur)
	}

	// Get.
	if s, b := c.getProduct(t, token, prodID); s != http.StatusOK || b["product_id"] != prodID {
		t.Errorf("get product status=%d body=%v", s, b)
	}

	// List shows the product with price.
	if s, b := c.listProducts(t, token); s != http.StatusOK {
		t.Errorf("list products status=%d body=%v", s, b)
	} else if arr, ok := b["data"].([]any); !ok || len(arr) != 1 {
		t.Errorf("products data = %v, want 1", b["data"])
	} else {
		priceOf(t, arr[0].(map[string]any)) // asserts the price is present
	}

	// Update: rename, price change, status → draft (not archived yet).
	status, body = c.patchProduct(t, token, prodID, fmt.Sprintf(
		`{"name":"Tomatoes Ripe","sku":"TOM-001","description":"Ripe","unit":"kg","status":"draft","category_id":%q,"price_minor":90000,"currency":"KZT"}`,
		catID))
	if status != http.StatusOK {
		t.Fatalf("patch product status=%d body=%v", status, body)
	}
	if body["name"] != "Tomatoes Ripe" || body["status"] != "draft" {
		t.Errorf("patch body = %v", body)
	}
	if am, _ := priceOf(t, body); am != 90000 {
		t.Errorf("updated price = %v, want 90000", am)
	}

	// Archive via PATCH (status → archived).
	status, body = c.patchProduct(t, token, prodID, fmt.Sprintf(
		`{"name":"Tomatoes Ripe","sku":"TOM-001","description":"Ripe","unit":"kg","status":"archived","category_id":%q,"price_minor":90000,"currency":"KZT"}`,
		catID))
	if status != http.StatusOK || body["status"] != "archived" {
		t.Errorf("archive: status=%d body=%v", status, body)
	}

	// Archived products are terminal: further updates → 409.
	status, body = c.patchProduct(t, token, prodID, fmt.Sprintf(
		`{"name":"Tomatoes","sku":"TOM-001","description":"","unit":"kg","status":"active","category_id":%q,"price_minor":90000,"currency":"KZT"}`,
		catID))
	if status != http.StatusConflict || body["code"] != "PRODUCT_ARCHIVED" {
		t.Errorf("update archived product: status=%d body=%v, want 409 PRODUCT_ARCHIVED", status, body)
	}
}

func TestCatalogValidation(t *testing.T) {
	c := newClient(t)
	token, _ := mustSupplier(t, c, "catvalid")

	cases := []struct {
		name string
		body string
	}{
		{"empty name", `{"name":"","unit":"kg","status":"active","price_minor":100,"currency":"KZT"}`},
		{"bad unit", `{"name":"X","unit":"crate","status":"active","price_minor":100,"currency":"KZT"}`},
		{"bad status", `{"name":"X","unit":"kg","status":"retired","price_minor":100,"currency":"KZT"}`},
		{"bad currency", `{"name":"X","unit":"kg","status":"active","price_minor":100,"currency":"kzt"}`},
		{"negative price", `{"name":"X","unit":"kg","status":"active","price_minor":-1,"currency":"KZT"}`},
		{"bad category uuid", `{"name":"X","unit":"kg","status":"active","category_id":"nope","price_minor":100,"currency":"KZT"}`},
		{"malformed json", `{"name":`},
	}
	for _, tc := range cases {
		status, body := c.createProduct(t, token, tc.body)
		if status != http.StatusBadRequest || (body["code"] != "VALIDATION_ERROR" && body["code"] != "MALFORMED_REQUEST") {
			t.Errorf("%s: status=%d body=%v", tc.name, status, body)
		}
	}

	// An unsupported currency passes the HTTP format check but is rejected by
	// the single money representation (platform/money) at the service layer.
	status, body := c.createProduct(t, token, `{"name":"X","unit":"kg","status":"active","price_minor":100,"currency":"GBP"}`)
	if status != 422 || body["code"] != "INVALID_PRICE" {
		t.Errorf("unsupported currency: status=%d body=%v, want 422 INVALID_PRICE", status, body)
	}

	// Cross-org category reference → 404 (RLS hides the other tenant's category).
	otherToken, _ := mustSupplier(t, c, "catvalid2")
	_, otherCat := c.createCategory(t, otherToken, "Other")
	if s, b := c.createProduct(t, token, fmt.Sprintf(
		`{"name":"X","unit":"kg","status":"active","category_id":%q,"price_minor":100,"currency":"KZT"}`,
		otherCat["category_id"].(string))); s != http.StatusNotFound || b["code"] != "CATEGORY_NOT_FOUND" {
		t.Errorf("cross-org category: status=%d body=%v, want 404 CATEGORY_NOT_FOUND", s, b)
	}
}

func TestCatalogOrgTypeRestriction(t *testing.T) {
	c := newClient(t)
	buyerToken, _ := mustBuyer(t, c, "catorg")

	// Buyer managing products → 422 ORG_TYPE_MISMATCH.
	if s, b := c.createProduct(t, buyerToken, `{"name":"X","unit":"kg","status":"active","price_minor":100,"currency":"KZT"}`); s != 422 || b["code"] != "ORG_TYPE_MISMATCH" {
		t.Errorf("buyer creating product: status=%d body=%v, want 422", s, b)
	}
	if s, b := c.listProducts(t, buyerToken); s != 422 || b["code"] != "ORG_TYPE_MISMATCH" {
		t.Errorf("buyer listing products: status=%d body=%v, want 422", s, b)
	}
	if s, b := c.createCategory(t, buyerToken, "X"); s != 422 || b["code"] != "ORG_TYPE_MISMATCH" {
		t.Errorf("buyer creating category: status=%d body=%v, want 422", s, b)
	}

	// Supplier browsing the marketplace → 422 ORG_TYPE_MISMATCH.
	supToken, _ := mustSupplier(t, c, "catorg2")
	if s, b := c.browseCatalog(t, supToken, ""); s != 422 || b["code"] != "ORG_TYPE_MISMATCH" {
		t.Errorf("supplier browsing: status=%d body=%v, want 422", s, b)
	}
}

func TestCatalogCrossTenant(t *testing.T) {
	c := newClient(t)
	tokenA, _ := mustSupplier(t, c, "ctxa")
	tokenB, _ := mustSupplier(t, c, "ctxb")

	prodID := mustCreateProduct(t, c, tokenA, "A Tomatoes", "kg", "active", 100, "KZT")

	// Supplier B cannot read or modify supplier A's product (RLS → 404).
	if s, b := c.getProduct(t, tokenB, prodID); s != http.StatusNotFound || b["code"] != "NOT_FOUND" {
		t.Errorf("cross-tenant get: status=%d body=%v, want 404", s, b)
	}
	if s, b := c.patchProduct(t, tokenB, prodID, `{"name":"Hack","unit":"kg","status":"active","price_minor":1,"currency":"KZT"}`); s != http.StatusNotFound || b["code"] != "NOT_FOUND" {
		t.Errorf("cross-tenant patch: status=%d body=%v, want 404", s, b)
	}

	// B's list does not include A's product.
	if s, b := c.listProducts(t, tokenB); s != http.StatusOK {
		t.Errorf("list status=%d", s)
	} else if arr, ok := b["data"].([]any); !ok || len(arr) != 0 {
		t.Errorf("B sees A's products: %v", b["data"])
	}
}

// ---------------------------------------------------------------------------
// Marketplace browsing
// ---------------------------------------------------------------------------

func TestCatalogMarketplace(t *testing.T) {
	c := newClient(t)
	supToken, supOrg := mustSupplier(t, c, "mkt")
	buyerToken, _ := mustBuyer(t, c, "mkt")

	// One active, one draft, one archived product from the same supplier.
	beefID := mustCreateProduct(t, c, supToken, "Beef", "kg", "active", 420000, "KZT")
	mustCreateProduct(t, c, supToken, "Secret Sauce", "pack", "draft", 150000, "KZT")
	archived := mustCreateProduct(t, c, supToken, "Old Milk", "l", "active", 65000, "KZT")
	if s, _ := c.patchProduct(t, supToken, archived, `{"name":"Old Milk","unit":"l","status":"archived","price_minor":65000,"currency":"KZT"}`); s != http.StatusOK {
		t.Fatalf("archive product: status=%d", s)
	}

	// Buyer sees exactly the supplier's active product with name + price.
	// (Scoped by supplier_id because the marketplace is intentionally global —
	// other tests seed their own suppliers with active products.)
	status, body := c.browseCatalog(t, buyerToken, "?supplier_id="+supOrg)
	if status != http.StatusOK {
		t.Fatalf("browse status=%d body=%v", status, body)
	}
	arr, ok := body["data"].([]any)
	if !ok {
		t.Fatalf("browse data = %v", body["data"])
	}
	if len(arr) != 1 {
		t.Fatalf("browse returned %d items, want 1 (draft+archived must be hidden): %v", len(arr), body["data"])
	}
	item := arr[0].(map[string]any)
	if item["name"] != "Beef" {
		t.Errorf("browse item = %v, want Beef", item["name"])
	}
	if item["supplier_name"] == "" {
		t.Errorf("browse item missing supplier_name: %v", item)
	}
	if am, cur := priceOf(t, item); am != 420000 || cur != "KZT" {
		t.Errorf("browse item price = %v %v, want 420000 KZT", am, cur)
	}

	// Search scoped to the supplier.
	status, body = c.browseCatalog(t, buyerToken, "?q=beef&supplier_id="+supOrg)
	if status != http.StatusOK || len(body["data"].([]any)) != 1 {
		t.Errorf("search 'beef': status=%d data=%v", status, body["data"])
	}
	status, body = c.browseCatalog(t, buyerToken, "?q=secret&supplier_id="+supOrg)
	if status != http.StatusOK || len(body["data"].([]any)) != 0 {
		t.Errorf("search 'secret' must not expose draft products: status=%d data=%v", status, body["data"])
	}

	// Pagination scoped to the supplier.
	status, body = c.browseCatalog(t, buyerToken, "?limit=1&offset=1&supplier_id="+supOrg)
	if status != http.StatusOK {
		t.Fatalf("pagination status=%d", status)
	}
	if b := body; b["total"].(float64) != 1 || len(b["data"].([]any)) != 0 {
		t.Errorf("pagination (limit=1 offset=1): total=%v data=%v, want total=1 data=[]", b["total"], b["data"])
	}

	// The global marketplace is public: an unfiltered browse returns this
	// supplier's active product alongside other suppliers' active products.
	status, body = c.browseCatalog(t, buyerToken, "?q=Beef")
	if status != http.StatusOK {
		t.Fatalf("global browse status=%d", status)
	}
	found := false
	for _, it := range body["data"].([]any) {
		if it.(map[string]any)["product_id"] == beefID {
			found = true
		}
	}
	if !found {
		t.Errorf("global marketplace browse missing the active product %s", beefID)
	}

	// Invalid filter uuid → 400.
	status, body = c.browseCatalog(t, buyerToken, "?supplier_id=nope")
	if status != http.StatusBadRequest || body["code"] != "VALIDATION_ERROR" {
		t.Errorf("bad supplier_id: status=%d body=%v, want 400", status, body)
	}
}

func TestCatalogBuyerRolesCanBrowse(t *testing.T) {
	c := newClient(t)
	supToken, _ := mustSupplier(t, c, "rolex")
	mustCreateProduct(t, c, supToken, "Beef", "kg", "active", 100, "KZT")

	// A buyer org owner can browse (org.owner wildcard).
	buyerToken, buyerOrg := mustBuyer(t, c, "rolex")
	if s, _ := c.browseCatalog(t, buyerToken, ""); s != http.StatusOK {
		t.Errorf("buyer owner browse status=%d", s)
	}

	// A viewer (catalog.read) can browse.
	viewerEmail := uniqueEmail("rolev")
	_, reg, _ := c.register(t, viewerEmail, "Password-1234!", "Viewer", "Viewer Own Org", "buyer")
	viewerID, _ := uuid.Parse(reg["user"].(map[string]any)["user_id"].(string))
	orgID, _ := uuid.Parse(buyerOrg)
	mustAddMembership(t, orgID, viewerID, mustRoleID(t, orgID, "viewer"))
	_, viewerLogin, _ := c.loginWithOrg(t, viewerEmail, "Password-1234!", buyerOrg)
	if s, _ := c.browseCatalog(t, accessToken(t, viewerLogin), ""); s != http.StatusOK {
		t.Errorf("viewer browse status=%d, want 200 (catalog.read)", s)
	}
	// A viewer cannot manage products (no catalog.manage).
	if s, _ := c.createProduct(t, accessToken(t, viewerLogin), `{"name":"X","unit":"kg","status":"active","price_minor":1,"currency":"KZT"}`); s != http.StatusForbidden {
		t.Errorf("viewer create product status=%d, want 403", s)
	}

	// Buyers can list marketplace categories for the filter UI; suppliers cannot.
	c.createCategory(t, supToken, "Produce")
	if s, b := c.browseCategories(t, buyerToken); s != http.StatusOK {
		t.Errorf("buyer browse categories status=%d body=%v", s, b)
	} else if arr, ok := b["data"].([]any); !ok || len(arr) == 0 {
		t.Errorf("buyer categories data = %v, want categories", b["data"])
	}
	if s, _ := c.browseCategories(t, supToken); s != 422 {
		t.Errorf("supplier browse categories status=%d, want 422", s)
	}
}

// ---------------------------------------------------------------------------
// RLS + audit
// ---------------------------------------------------------------------------

func TestCatalogRLSMarketplaceIsolation(t *testing.T) {
	c := newClient(t)
	supToken, supOrg := mustSupplier(t, c, "rlscat")
	_, buyerOrgA := mustBuyer(t, c, "rlscat")
	_, buyerOrgB := mustBuyer(t, c, "rlscat2")

	prodID := mustCreateProduct(t, c, supToken, "Beef", "kg", "active", 100, "KZT")
	draftID := mustCreateProduct(t, c, supToken, "Draft", "kg", "draft", 50, "KZT")

	// Direct SQL as the low-privilege app role, scoped to the test supplier so
	// other tests' active products don't affect the counts (the marketplace is
	// intentionally global).
	tx, err := appPool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", buyerOrgA); err != nil {
		t.Fatalf("set tenant: %v", err)
	}
	var visible int
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM catalog_products WHERE org_id = $1", supOrg).Scan(&visible); err != nil {
		t.Fatalf("count products: %v", err)
	}
	if visible != 1 {
		t.Errorf("buyer tenant sees %d products of the supplier, want only 1 active", visible)
	}
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM catalog_products WHERE product_id = $1", prodID).Scan(&visible); err != nil {
		t.Fatalf("count active: %v", err)
	}
	if visible != 1 {
		t.Errorf("buyer cannot see active product: %d", visible)
	}
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM catalog_products WHERE product_id = $1", draftID).Scan(&visible); err != nil {
		t.Fatalf("count draft: %v", err)
	}
	if visible != 0 {
		t.Errorf("buyer leaked draft product: %d rows", visible)
	}
	// Price of an active product is visible; nothing else exists.
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM catalog_prices WHERE org_id = $1", supOrg).Scan(&visible); err != nil {
		t.Fatalf("count prices: %v", err)
	}
	if visible != 1 {
		t.Errorf("buyer sees %d price rows of the supplier, want 1 (active product only)", visible)
	}

	// Buyer tenant B also sees exactly the supplier's active product (the
	// marketplace is public across buyer organizations).
	tx2, err := appPool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx2.Rollback(ctx) }()
	if _, err := tx2.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", buyerOrgB); err != nil {
		t.Fatalf("set tenant: %v", err)
	}
	if err := tx2.QueryRow(ctx, "SELECT count(*) FROM catalog_products WHERE org_id = $1", supOrg).Scan(&visible); err != nil {
		t.Fatalf("count products B: %v", err)
	}
	if visible != 1 {
		t.Errorf("buyer B sees %d products of the supplier, want 1", visible)
	}

	// The supplier tenant sees ALL its own products (management view).
	tx3, err := appPool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx3.Rollback(ctx) }()
	if _, err := tx3.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", supOrg); err != nil {
		t.Fatalf("set tenant: %v", err)
	}
	if err := tx3.QueryRow(ctx, "SELECT count(*) FROM catalog_products").Scan(&visible); err != nil {
		t.Fatalf("count products supplier: %v", err)
	}
	if visible != 2 {
		t.Errorf("supplier sees %d products, want 2", visible)
	}
}

func TestCatalogAudit(t *testing.T) {
	c := newClient(t)
	token, orgID := mustSupplier(t, c, "audit")
	prodID := mustCreateProduct(t, c, token, "Audited", "kg", "active", 100, "KZT")
	if s, _ := c.patchProduct(t, token, prodID, `{"name":"Audited","unit":"kg","status":"active","price_minor":100,"currency":"KZT"}`); s != http.StatusOK {
		t.Fatalf("patch status=%d", s)
	}
	if s, _ := c.patchProduct(t, token, prodID, `{"name":"Audited","unit":"kg","status":"archived","price_minor":100,"currency":"KZT"}`); s != http.StatusOK {
		t.Fatalf("archive status=%d", s)
	}

	var actions string
	if err := adminPool.QueryRow(ctx, `
		SELECT string_agg(action, ',' ORDER BY created_at)
		FROM audit_logs WHERE org_id = $1 AND entity_id = $2`,
		orgID, prodID).Scan(&actions); err != nil {
		t.Fatalf("query audit: %v", err)
	}
	if !strings.Contains(actions, "product.created") {
		t.Errorf("audit missing product.created: %q", actions)
	}
	if !strings.Contains(actions, "product.updated") {
		t.Errorf("audit missing product.updated: %q", actions)
	}
	if !strings.Contains(actions, "product.archived") {
		t.Errorf("audit missing product.archived: %q", actions)
	}
}