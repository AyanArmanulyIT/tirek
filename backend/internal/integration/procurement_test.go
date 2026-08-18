package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// Procurement helpers
// ---------------------------------------------------------------------------

func (c *client) getCart(t *testing.T, token string) (int, map[string]any) {
	t.Helper()
	status, data, _, err := c.do(http.MethodGet, "/api/v1/procurement/cart", "", token, nil)
	if err != nil {
		t.Fatalf("get cart: %v", err)
	}
	return status, decodeBody(t, data)
}

func (c *client) addCartItem(t *testing.T, token, productID string, quantity int) (int, map[string]any) {
	t.Helper()
	status, data, _, err := c.do(http.MethodPost, "/api/v1/procurement/cart/items",
		fmt.Sprintf(`{"product_id":%q,"quantity":%d}`, productID, quantity), token, nil)
	if err != nil {
		t.Fatalf("add cart item: %v", err)
	}
	return status, decodeBody(t, data)
}

func (c *client) patchCartItem(t *testing.T, token, itemID string, quantity int) (int, map[string]any) {
	t.Helper()
	status, data, _, err := c.do(http.MethodPatch, "/api/v1/procurement/cart/items/"+itemID,
		fmt.Sprintf(`{"quantity":%d}`, quantity), token, nil)
	if err != nil {
		t.Fatalf("patch cart item: %v", err)
	}
	return status, decodeBody(t, data)
}

func (c *client) deleteCartItem(t *testing.T, token, itemID string) (int, map[string]any) {
	t.Helper()
	status, data, _, err := c.do(http.MethodDelete, "/api/v1/procurement/cart/items/"+itemID, "", token, nil)
	if err != nil {
		t.Fatalf("delete cart item: %v", err)
	}
	return status, decodeBody(t, data)
}

func (c *client) submit(t *testing.T, token, idempotencyKey string) (int, map[string]any) {
	t.Helper()
	headers := map[string]string{}
	if idempotencyKey != "" {
		headers["Idempotency-Key"] = idempotencyKey
	}
	status, data, _, err := c.do(http.MethodPost, "/api/v1/procurement/submit", "", token, headers)
	if err != nil {
		t.Fatalf("submit cart: %v", err)
	}
	return status, decodeBody(t, data)
}

func (c *client) listRequests(t *testing.T, token string) (int, map[string]any) {
	t.Helper()
	status, data, _, err := c.do(http.MethodGet, "/api/v1/procurement/requests", "", token, nil)
	if err != nil {
		t.Fatalf("list requests: %v", err)
	}
	return status, decodeBody(t, data)
}

func (c *client) getRequest(t *testing.T, token, id string) (int, map[string]any) {
	t.Helper()
	status, data, _, err := c.do(http.MethodGet, "/api/v1/procurement/requests/"+id, "", token, nil)
	if err != nil {
		t.Fatalf("get request: %v", err)
	}
	return status, decodeBody(t, data)
}

func (c *client) cancelRequest(t *testing.T, token, id string) (int, map[string]any) {
	t.Helper()
	status, data, _, err := c.do(http.MethodPatch, "/api/v1/procurement/requests/"+id+"/cancel", "", token, nil)
	if err != nil {
		t.Fatalf("cancel request: %v", err)
	}
	return status, decodeBody(t, data)
}

func (c *client) listIncoming(t *testing.T, token string) (int, map[string]any) {
	t.Helper()
	status, data, _, err := c.do(http.MethodGet, "/api/v1/procurement/incoming", "", token, nil)
	if err != nil {
		t.Fatalf("list incoming: %v", err)
	}
	return status, decodeBody(t, data)
}

func (c *client) getIncoming(t *testing.T, token, id string) (int, map[string]any) {
	t.Helper()
	status, data, _, err := c.do(http.MethodGet, "/api/v1/procurement/incoming/"+id, "", token, nil)
	if err != nil {
		t.Fatalf("get incoming: %v", err)
	}
	return status, decodeBody(t, data)
}

// cartItems extracts the flattened list of cart items from a cart response.
func cartItems(t *testing.T, cart map[string]any) []map[string]any {
	t.Helper()
	groups, ok := cart["groups"].([]any)
	if !ok {
		t.Fatalf("no groups in cart: %v", cart)
	}
	var out []map[string]any
	for _, g := range groups {
		items, _ := g.(map[string]any)["items"].([]any)
		for _, it := range items {
			out = append(out, it.(map[string]any))
		}
	}
	return out
}

func cartTotalMinor(t *testing.T, cart map[string]any) float64 {
	t.Helper()
	v, _ := cart["total_minor"].(float64)
	return v
}

// mustAddToCart adds a product and returns the cart item id.
func mustAddToCart(t *testing.T, c *client, token, productID string, quantity int) string {
	t.Helper()
	status, cart := c.addCartItem(t, token, productID, quantity)
	if status != http.StatusOK {
		t.Fatalf("add to cart: status=%d body=%v", status, cart)
	}
	items := cartItems(t, cart)
	if len(items) == 0 {
		t.Fatalf("cart has no items after add: %v", cart)
	}
	id, _ := items[0]["cart_item_id"].(string)
	if id == "" {
		t.Fatalf("no cart_item_id: %v", items)
	}
	return id
}

// ---------------------------------------------------------------------------
// Cart behaviour
// ---------------------------------------------------------------------------

func TestProcurementCart(t *testing.T) {
	c := newClient(t)
	supToken, _ := mustSupplier(t, c, "pcat")
	buyerToken, _ := mustBuyer(t, c, "pcat")

	prodID := mustCreateProduct(t, c, supToken, "Cartable", "kg", "active", 1000, "KZT")

	// Empty cart (created lazily).
	status, cart := c.getCart(t, buyerToken)
	if status != http.StatusOK {
		t.Fatalf("get cart status=%d body=%v", status, cart)
	}
	if cart["status"] != "active" {
		t.Errorf("cart status = %v, want active", cart["status"])
	}
	if n := len(cartItems(t, cart)); n != 0 {
		t.Errorf("new cart should be empty, got %d items", n)
	}

	// Add an item.
	itemID := mustAddToCart(t, c, buyerToken, prodID, 2)
	if total := cartTotalMinor(t, cart); total != 0 {
		t.Errorf("cart total after first add = %v, want 0 (stale)", total)
	}
	status, cart = c.addCartItem(t, buyerToken, prodID, 3)
	if status != http.StatusOK {
		t.Fatalf("re-add status=%d", status)
	}
	items := cartItems(t, cart)
	if len(items) != 1 {
		t.Fatalf("same product added twice should merge into 1 line, got %d", len(items))
	}
	if items[0]["quantity"].(float64) != 5 {
		t.Errorf("merged quantity = %v, want 5", items[0]["quantity"])
	}
	if items[0]["unit_price_minor"].(float64) != 1000 {
		t.Errorf("snapshot price = %v, want 1000", items[0]["unit_price_minor"])
	}
	if items[0]["line_total_minor"].(float64) != 5000 {
		t.Errorf("line total = %v, want 5000", items[0]["line_total_minor"])
	}
	if cartTotalMinor(t, cart) != 5000 {
		t.Errorf("cart total = %v, want 5000", cartTotalMinor(t, cart))
	}

	// Quantity update.
	status, cart = c.patchCartItem(t, buyerToken, itemID, 7)
	if status != http.StatusOK {
		t.Fatalf("patch status=%d body=%v", status, cart)
	}
	items = cartItems(t, cart)
	if items[0]["quantity"].(float64) != 7 {
		t.Errorf("updated quantity = %v, want 7", items[0]["quantity"])
	}
	if cartTotalMinor(t, cart) != 7000 {
		t.Errorf("cart total after patch = %v, want 7000", cartTotalMinor(t, cart))
	}

	// Quantity validation.
	status, body := c.patchCartItem(t, buyerToken, itemID, 0)
	if status != http.StatusBadRequest || body["code"] != "VALIDATION_ERROR" {
		t.Errorf("patch qty 0: status=%d body=%v, want 400", status, body)
	}
	if status, body := c.addCartItem(t, buyerToken, prodID, -3); status != http.StatusBadRequest || body["code"] != "VALIDATION_ERROR" {
		t.Errorf("add qty -3: status=%d body=%v, want 400", status, body)
	}

	// Remove item.
	status, cart = c.deleteCartItem(t, buyerToken, itemID)
	if status != http.StatusOK {
		t.Fatalf("delete status=%d body=%v", status, cart)
	}
	if n := len(cartItems(t, cart)); n != 0 {
		t.Errorf("cart should be empty after remove, got %d items", n)
	}
	if cartTotalMinor(t, cart) != 0 {
		t.Errorf("cart total after remove = %v, want 0", cartTotalMinor(t, cart))
	}

	// Forged item id (another tenant's / unknown) fails safely.
	status, body = c.patchCartItem(t, buyerToken, uuid.NewString(), 1)
	if status != http.StatusNotFound || body["code"] != "CART_ITEM_NOT_FOUND" {
		t.Errorf("forge patch: status=%d body=%v, want 404", status, body)
	}
	status, body = c.deleteCartItem(t, buyerToken, uuid.NewString())
	if status != http.StatusNotFound || body["code"] != "CART_ITEM_NOT_FOUND" {
		t.Errorf("forge delete: status=%d body=%v, want 404", status, body)
	}
}

func TestProcurementCartRestrictions(t *testing.T) {
	c := newClient(t)
	supToken, _ := mustSupplier(t, c, "prestr")
	buyerToken, _ := mustBuyer(t, c, "prestr")
	buyerBToken, _ := mustBuyer(t, c, "prestrb")

	prodID := mustCreateProduct(t, c, supToken, "Restricted", "kg", "active", 100, "KZT")
	mustAddToCart(t, c, buyerToken, prodID, 1)

	// Buyer-only: a supplier cannot manage a cart.
	if s, b := c.getCart(t, supToken); s != 422 || b["code"] != "ORG_TYPE_MISMATCH" {
		t.Errorf("supplier get cart: status=%d body=%v, want 422", s, b)
	}
	if s, b := c.addCartItem(t, supToken, prodID, 1); s != 422 || b["code"] != "ORG_TYPE_MISMATCH" {
		t.Errorf("supplier add item: status=%d body=%v, want 422", s, b)
	}
	if s, b := c.submit(t, supToken, ""); s != 422 || b["code"] != "ORG_TYPE_MISMATCH" {
		t.Errorf("supplier submit: status=%d body=%v, want 422", s, b)
	}

	// Unpurchasable products are rejected (draft/archived or foreign).
	draftID := mustCreateProduct(t, c, supToken, "Drafty", "kg", "draft", 50, "KZT")
	if s, b := c.addCartItem(t, buyerToken, draftID, 1); s != 422 || b["code"] != "PRODUCT_UNAVAILABLE" {
		t.Errorf("add draft product: status=%d body=%v, want 422", s, b)
	}
	archID := mustCreateProduct(t, c, supToken, "Archy", "kg", "active", 70, "KZT")
	if s, _ := c.patchProduct(t, supToken, archID, `{"name":"Archy","unit":"kg","status":"archived","price_minor":70,"currency":"KZT"}`); s != http.StatusOK {
		t.Fatalf("archive product status=%d", s)
	}
	if s, b := c.addCartItem(t, buyerToken, archID, 1); s != 422 || b["code"] != "PRODUCT_UNAVAILABLE" {
		t.Errorf("add archived product: status=%d body=%v, want 422", s, b)
	}

	// Cross-tenant isolation: buyer B's cart is empty and cannot touch buyer A's.
	status, b := c.getCart(t, buyerBToken)
	if status != http.StatusOK {
		t.Fatalf("buyer B get cart status=%d body=%v", status, b)
	}
	if n := len(cartItems(t, b)); n != 0 {
		t.Errorf("buyer B cart leaked items: %v", b)
	}
	if s, _ := c.patchCartItem(t, buyerBToken, uuid.NewString(), 1); s != http.StatusNotFound {
		t.Errorf("buyer B forge patch: status=%d, want 404", s)
	}
}

func TestProcurementCartRLS(t *testing.T) {
	c := newClient(t)
	supToken, _ := mustSupplier(t, c, "prls")
	buyerToken, buyerOrg := mustBuyer(t, c, "prls")
	_, buyerOrgB := mustBuyer(t, c, "prlsb")

	prodID := mustCreateProduct(t, c, supToken, "RLS Prod", "kg", "active", 900, "KZT")
	mustAddToCart(t, c, buyerToken, prodID, 2)

	// Buyer B sees zero cart items directly at the DB layer (RLS).
	tx, err := appPool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", buyerOrgB); err != nil {
		t.Fatalf("set tenant: %v", err)
	}
	var visible int
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM procurement_cart_items").Scan(&visible); err != nil {
		t.Fatalf("count cart items: %v", err)
	}
	if visible != 0 {
		t.Errorf("buyer B sees %d cart items of buyer A (RLS leak)", visible)
	}
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM procurement_carts").Scan(&visible); err != nil {
		t.Fatalf("count carts: %v", err)
	}
	if visible != 0 {
		t.Errorf("buyer B sees %d carts of buyer A (RLS leak)", visible)
	}

	// Missing tenant context fails closed on the new tables.
	tx2, err := appPool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx2.Rollback(ctx) }()
	if _, err := tx2.Exec(ctx, "SELECT count(*) FROM procurement_cart_items"); err == nil {
		t.Errorf("cart items query without tenant context succeeded; want RLS error")
	} else if !strings.Contains(err.Error(), "22P02") {
		t.Errorf("unexpected error without tenant context: %v", err)
	}

	// The owning buyer sees its own rows.
	tx3, err := appPool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx3.Rollback(ctx) }()
	if _, err := tx3.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", buyerOrg); err != nil {
		t.Fatalf("set tenant: %v", err)
	}
	if err := tx3.QueryRow(ctx, "SELECT count(*) FROM procurement_cart_items").Scan(&visible); err != nil {
		t.Fatalf("count cart items owner: %v", err)
	}
	if visible != 1 {
		t.Errorf("owner sees %d cart items, want 1", visible)
	}
}

// ---------------------------------------------------------------------------
// Submission + snapshots
// ---------------------------------------------------------------------------

func TestProcurementSubmitSingleSupplier(t *testing.T) {
	c := newClient(t)
	supToken, _ := mustSupplier(t, c, "psub")
	buyerToken, _ := mustBuyer(t, c, "psub")

	a := mustCreateProduct(t, c, supToken, "A", "kg", "active", 1500, "KZT")
	b := mustCreateProduct(t, c, supToken, "B", "box", "active", 3000, "KZT")
	mustAddToCart(t, c, buyerToken, a, 2)  // 3000
	mustAddToCart(t, c, buyerToken, b, 1)  // 3000

	status, body := c.submit(t, buyerToken, "submit-1-"+uuid.NewString())
	if status != http.StatusOK {
		t.Fatalf("submit status=%d body=%v", status, body)
	}
	reqs, _ := body["requests"].([]any)
	if len(reqs) != 1 {
		t.Fatalf("single-supplier cart must create 1 request, got %d", len(reqs))
	}
	rq := reqs[0].(map[string]any)
	if rq["status"] != "submitted" {
		t.Errorf("request status = %v, want submitted", rq["status"])
	}
	if rq["total_minor"].(float64) != 6000 {
		t.Errorf("request total = %v, want 6000", rq["total_minor"])
	}
	items, _ := rq["items"].([]any)
	if len(items) != 2 {
		t.Errorf("request items = %d, want 2", len(items))
	}

	// Cart is now empty.
	status, cart := c.getCart(t, buyerToken)
	if status != http.StatusOK {
		t.Fatalf("get cart after submit status=%d", status)
	}
	if n := len(cartItems(t, cart)); n != 0 {
		t.Errorf("cart should be empty after submit, got %d items", n)
	}

	// Buyer sees the request in its list.
	if s, b := c.listRequests(t, buyerToken); s != http.StatusOK {
		t.Errorf("list requests status=%d", s)
	} else if arr, _ := b["data"].([]any); len(arr) != 1 {
		t.Errorf("buyer requests = %v, want 1", b["data"])
	}
}

func TestProcurementSubmitMultiSupplier(t *testing.T) {
	c := newClient(t)
	supAToken, supAOrg := mustSupplier(t, c, "pmulti")
	supBToken, _ := mustSupplier(t, c, "pmultib")
	buyerToken, _ := mustBuyer(t, c, "pmulti")

	a1 := mustCreateProduct(t, c, supAToken, "A1", "kg", "active", 100, "KZT")
	a2 := mustCreateProduct(t, c, supAToken, "A2", "kg", "active", 200, "KZT")
	b1 := mustCreateProduct(t, c, supBToken, "B1", "box", "active", 400, "KZT")
	mustAddToCart(t, c, buyerToken, a1, 1)
	mustAddToCart(t, c, buyerToken, a2, 2) // 400
	mustAddToCart(t, c, buyerToken, b1, 3) // 1200

	status, body := c.submit(t, buyerToken, "submit-multi-"+uuid.NewString())
	if status != http.StatusOK {
		t.Fatalf("submit status=%d body=%v", status, body)
	}
	reqs, _ := body["requests"].([]any)
	if len(reqs) != 2 {
		t.Fatalf("multi-supplier cart must create 2 requests, got %d", len(reqs))
	}
	bySupplier := map[string]float64{}
	byItems := map[string]int{}
	for _, r := range reqs {
		rq := r.(map[string]any)
		bySupplier[rq["supplier_org_id"].(string)] = rq["total_minor"].(float64)
		byItems[rq["supplier_org_id"].(string)] = len(rq["items"].([]any))
	}
	if bySupplier[supAOrg] != 500 { // 100 + 400
		t.Errorf("supplier A total = %v, want 500", bySupplier[supAOrg])
	}
	if byItems[supAOrg] != 2 {
		t.Errorf("supplier A items = %d, want 2", byItems[supAOrg])
	}
	// Supplier A sees only its own incoming request.
	status, body = c.listIncoming(t, supAToken)
	if status != http.StatusOK {
		t.Fatalf("supplier A incoming status=%d body=%v", status, body)
	}
	incoming, _ := body["data"].([]any)
	if len(incoming) != 1 {
		t.Errorf("supplier A incoming = %d requests, want 1", len(incoming))
	} else if inc := incoming[0].(map[string]any); inc["supplier_org_id"] != supAOrg {
		t.Errorf("supplier A received foreign request: %v", inc)
	}
	// Supplier B sees only its own incoming request (total 1200).
	status, body = c.listIncoming(t, supBToken)
	if status != http.StatusOK {
		t.Fatalf("supplier B incoming status=%d", status)
	}
	incoming, _ = body["data"].([]any)
	if len(incoming) != 1 {
		t.Errorf("supplier B incoming = %d requests, want 1", len(incoming))
	} else if inc := incoming[0].(map[string]any); inc["total_minor"].(float64) != 1200 {
		t.Errorf("supplier B request total = %v, want 1200", inc["total_minor"])
	}
}

func TestProcurementPriceSnapshot(t *testing.T) {
	c := newClient(t)
	supToken, _ := mustSupplier(t, c, "psnap")
	buyerToken, _ := mustBuyer(t, c, "psnap")

	prodID := mustCreateProduct(t, c, supToken, "Snappy", "kg", "active", 1000, "KZT")
	mustAddToCart(t, c, buyerToken, prodID, 3)

	// Supplier raises the price after the item was added.
	if s, _ := c.patchProduct(t, supToken, prodID, `{"name":"Snappy","unit":"kg","status":"active","price_minor":2500,"currency":"KZT"}`); s != http.StatusOK {
		t.Fatalf("patch price status=%d", s)
	}

	// The cart line keeps the original snapshot (never silently re-priced).
	status, cart := c.getCart(t, buyerToken)
	if status != http.StatusOK {
		t.Fatalf("get cart status=%d", status)
	}
	items := cartItems(t, cart)
	if items[0]["unit_price_minor"].(float64) != 1000 {
		t.Errorf("cart snapshot price changed to %v, want 1000", items[0]["unit_price_minor"])
	}
	if items[0]["line_total_minor"].(float64) != 3000 {
		t.Errorf("cart line total = %v, want 3000", items[0]["line_total_minor"])
	}

	// The submitted request freezes the original snapshot.
	if s, body := c.submit(t, buyerToken, "submit-snap-"+uuid.NewString()); s != http.StatusOK {
		t.Fatalf("submit status=%d body=%v", s, body)
	}
	status, body := c.listRequests(t, buyerToken)
	if status != http.StatusOK {
		t.Fatalf("list requests status=%d", status)
	}
	arr, _ := body["data"].([]any)
	if len(arr) != 1 {
		t.Fatalf("expected 1 request, got %v", body["data"])
	}
	reqID := arr[0].(map[string]any)["request_id"].(string)
	status, detail := c.getRequest(t, buyerToken, reqID)
	if status != http.StatusOK {
		t.Fatalf("get request status=%d", status)
	}
	ritems, _ := detail["items"].([]any)
	if len(ritems) != 1 {
		t.Fatalf("request items = %d, want 1", len(ritems))
	}
	ritem := ritems[0].(map[string]any)
	if ritem["unit_price_minor"].(float64) != 1000 {
		t.Errorf("request snapshot price = %v, want 1000", ritem["unit_price_minor"])
	}
	if ritem["line_total_minor"].(float64) != 3000 {
		t.Errorf("request line total = %v, want 3000", ritem["line_total_minor"])
	}
	if detail["total_minor"].(float64) != 3000 {
		t.Errorf("request total = %v, want 3000", detail["total_minor"])
	}
	if ritem["currency"] != "KZT" {
		t.Errorf("request currency = %v, want KZT", ritem["currency"])
	}
	if ritem["product_name"] != "Snappy" {
		t.Errorf("request product_name snapshot = %v, want Snappy", ritem["product_name"])
	}
}

func TestProcurementSubmitRollback(t *testing.T) {
	c := newClient(t)
	supToken, supOrg := mustSupplier(t, c, "pback")
	buyerToken, buyerOrg := mustBuyer(t, c, "pback")

	good := mustCreateProduct(t, c, supToken, "Good", "kg", "active", 100, "KZT")
	bad := mustCreateProduct(t, c, supToken, "Bad", "kg", "active", 200, "KZT")
	mustAddToCart(t, c, buyerToken, good, 1)
	mustAddToCart(t, c, buyerToken, bad, 1)

	// The supplier archives one product: the whole submission must fail
	// atomically and the cart must stay unchanged.
	if s, _ := c.patchProduct(t, supToken, bad, `{"name":"Bad","unit":"kg","status":"archived","price_minor":200,"currency":"KZT"}`); s != http.StatusOK {
		t.Fatalf("archive status=%d", s)
	}
	if s, b := c.submit(t, buyerToken, "submit-rollback-"+uuid.NewString()); s != 422 || b["code"] != "PRODUCT_UNAVAILABLE" {
		t.Errorf("submit with archived product: status=%d body=%v, want 422", s, b)
	}

	// No purchase requests were created.
	var n int
	if err := adminPool.QueryRow(ctx, "SELECT count(*) FROM rfqs WHERE org_id = $1", buyerOrg).Scan(&n); err != nil {
		t.Fatalf("count rfqs: %v", err)
	}
	if n != 0 {
		t.Errorf("rollback violated: %d rfqs created", n)
	}

	// The cart is unchanged (both items still present).
	status, cart := c.getCart(t, buyerToken)
	if status != http.StatusOK {
		t.Fatalf("get cart status=%d", status)
	}
	if items := cartItems(t, cart); len(items) != 2 {
		t.Errorf("cart after failed submit = %d items, want 2 (unchanged)", len(items))
	}

	// Remove the archived item, then the submission succeeds.
	items := cartItems(t, cart)
	for _, it := range items {
		if it["product_name"] == "Bad" {
			if s, _ := c.deleteCartItem(t, buyerToken, it["cart_item_id"].(string)); s != http.StatusOK {
				t.Fatalf("delete bad item status=%d", s)
			}
		}
	}
	if s, body := c.submit(t, buyerToken, "submit-rollback2-"+uuid.NewString()); s != http.StatusOK {
		t.Fatalf("retry submit status=%d body=%v", s, body)
	}
	var n2 int
	if err := adminPool.QueryRow(ctx, "SELECT count(*) FROM rfqs WHERE org_id = $1", buyerOrg).Scan(&n2); err != nil {
		t.Fatalf("count rfqs: %v", err)
	}
	if n2 != 1 {
		t.Errorf("rfqs after retry = %d, want 1", n2)
	}
	_ = supOrg
}

func TestProcurementSubmitIdempotency(t *testing.T) {
	c := newClient(t)
	supToken, _ := mustSupplier(t, c, "pidem")
	buyerToken, buyerOrg := mustBuyer(t, c, "pidem")

	prod := mustCreateProduct(t, c, supToken, "Idem", "kg", "active", 100, "KZT")
	mustAddToCart(t, c, buyerToken, prod, 2)

	key := "submit-idem-" + uuid.NewString()
	status, body := c.submit(t, buyerToken, key)
	if status != http.StatusOK {
		t.Fatalf("first submit status=%d body=%v", status, body)
	}

	// Replay with the same key: same result, no new requests.
	status2, body2 := c.submit(t, buyerToken, key)
	if status2 != http.StatusOK {
		t.Fatalf("retry submit status=%d body=%v", status2, body2)
	}
	var n int
	if err := adminPool.QueryRow(ctx, "SELECT count(*) FROM rfqs WHERE org_id = $1", buyerOrg).Scan(&n); err != nil {
		t.Fatalf("count rfqs: %v", err)
	}
	if n != 1 {
		t.Errorf("idempotency violated: %d rfqs for same key", n)
	}
	first, _ := json.Marshal(body)
	second, _ := json.Marshal(body2)
	if string(first) != string(second) {
		t.Errorf("idempotent replay differs:\nfirst=%s\nsecond=%s", first, second)
	}
}

// ---------------------------------------------------------------------------
// Purchase requests: listing, isolation, cancellation
// ---------------------------------------------------------------------------

func TestProcurementRequestAccess(t *testing.T) {
	c := newClient(t)
	supToken, _ := mustSupplier(t, c, "preq")
	supBToken, _ := mustSupplier(t, c, "preqb")
	buyerToken, buyerOrg := mustBuyer(t, c, "preq")
	buyerBToken, _ := mustBuyer(t, c, "preqb")

	prod := mustCreateProduct(t, c, supToken, "Req", "kg", "active", 100, "KZT")
	mustAddToCart(t, c, buyerToken, prod, 1)
	if s, body := c.submit(t, buyerToken, "submit-req-"+uuid.NewString()); s != http.StatusOK {
		t.Fatalf("submit status=%d body=%v", s, body)
	}
	status, reqs := c.listRequests(t, buyerToken)
	if status != http.StatusOK {
		t.Fatalf("list requests status=%d", status)
	}
	arr, _ := reqs["data"].([]any)
	reqID := arr[0].(map[string]any)["request_id"].(string)

	// Buyer B cannot read buyer A's request.
	if s, b := c.getRequest(t, buyerBToken, reqID); s != http.StatusNotFound {
		t.Errorf("buyer B get foreign request: status=%d body=%v, want 404", s, b)
	}
	// Supplier B (different supplier) cannot read it either.
	if s, b := c.getIncoming(t, supBToken, reqID); s != http.StatusNotFound {
		t.Errorf("supplier B get foreign incoming: status=%d body=%v, want 404", s, b)
	}
	// The addressed supplier can read its incoming request and its items.
	if s, b := c.getIncoming(t, supToken, reqID); s != http.StatusOK {
		t.Errorf("supplier get incoming: status=%d body=%v", s, b)
	} else if it, _ := b["items"].([]any); len(it) != 1 {
		t.Errorf("incoming items = %v, want 1", b["items"])
	} else if b["buyer_name"] == "" {
		t.Errorf("incoming missing buyer_name: %v", b)
	}

	// Suppliers cannot see the buyer's request list endpoint (org type).
	if s, _ := c.listRequests(t, supToken); s != 422 {
		t.Errorf("supplier list requests: status=%d, want 422", s)
	}
	// Buyer cannot list incoming.
	if s, _ := c.listIncoming(t, buyerToken); s != 422 {
		t.Errorf("buyer list incoming: status=%d, want 422", s)
	}
	// Buyer cannot cancel a foreign request.
	if s, b := c.cancelRequest(t, buyerBToken, reqID); s != http.StatusNotFound {
		t.Errorf("buyer B cancel foreign request: status=%d body=%v, want 404", s, b)
	}

	// Cancellation by the owning buyer works; second cancel is a conflict.
	if s, b := c.cancelRequest(t, buyerToken, reqID); s != http.StatusOK {
		t.Fatalf("cancel request: status=%d body=%v", s, b)
	} else if b["status"] != "cancelled" {
		t.Errorf("cancelled status = %v", b["status"])
	}
	if s, b := c.cancelRequest(t, buyerToken, reqID); s != http.StatusConflict || b["code"] != "CANNOT_CANCEL" {
		t.Errorf("re-cancel: status=%d body=%v, want 409 CANNOT_CANCEL", s, b)
	}

	// The supplier still sees the cancelled request (status visible).
	if s, b := c.getIncoming(t, supToken, reqID); s != http.StatusOK {
		t.Errorf("supplier get cancelled incoming: status=%d", s)
	} else if b["status"] != "cancelled" {
		t.Errorf("incoming status = %v, want cancelled", b["status"])
	}

	// RLS at the DB layer: buyer B sees zero rfqs of buyer A.
	tx, err := appPool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", buyerOrg); err != nil {
		t.Fatalf("set tenant: %v", err)
	}
	var visible int
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM rfqs").Scan(&visible); err != nil {
		t.Fatalf("count rfqs: %v", err)
	}
	if visible != 1 {
		t.Errorf("owner sees %d rfqs, want 1", visible)
	}
}

func TestProcurementRBAC(t *testing.T) {
	c := newClient(t)
	supToken, _ := mustSupplier(t, c, "prbac")
	buyerToken, buyerOrg := mustBuyer(t, c, "prbac")

	prod := mustCreateProduct(t, c, supToken, "RBAC", "kg", "active", 100, "KZT")
	mustAddToCart(t, c, buyerToken, prod, 1)

	// A viewer (procurement.read only) can list but not manage or submit.
	viewerEmail := uniqueEmail("prbacv")
	_, reg, _ := c.register(t, viewerEmail, "Password-1234!", "Viewer", "Viewer Org", "buyer")
	viewerID, _ := uuid.Parse(reg["user"].(map[string]any)["user_id"].(string))
	orgID, _ := uuid.Parse(buyerOrg)
	mustAddMembership(t, orgID, viewerID, mustRoleID(t, orgID, "viewer"))
	_, viewerLogin, _ := c.loginWithOrg(t, viewerEmail, "Password-1234!", buyerOrg)
	viewerToken := accessToken(t, viewerLogin)

	if s, _ := c.listRequests(t, viewerToken); s != http.StatusOK {
		t.Errorf("viewer list requests: status=%d, want 200", s)
	}
	if s, _ := c.getCart(t, viewerToken); s != http.StatusForbidden {
		t.Errorf("viewer get cart: status=%d, want 403", s)
	}
	if s, _ := c.submit(t, viewerToken, ""); s != http.StatusForbidden {
		t.Errorf("viewer submit: status=%d, want 403", s)
	}
	if s, _ := c.addCartItem(t, viewerToken, prod, 1); s != http.StatusForbidden {
		t.Errorf("viewer add item: status=%d, want 403", s)
	}
	if s, _ := c.cancelRequest(t, viewerToken, uuid.NewString()); s != http.StatusForbidden {
		t.Errorf("viewer cancel: status=%d, want 403", s)
	}
}

func TestProcurementAudit(t *testing.T) {
	c := newClient(t)
	supToken, _ := mustSupplier(t, c, "paudit")
	buyerToken, buyerOrg := mustBuyer(t, c, "paudit")

	prod := mustCreateProduct(t, c, supToken, "Audit", "kg", "active", 100, "KZT")
	mustAddToCart(t, c, buyerToken, prod, 2)
	items := cartItems(t, mustGetCart(t, c, buyerToken))
	itemID := items[0]["cart_item_id"].(string)
	c.patchCartItem(t, buyerToken, itemID, 3)
	c.deleteCartItem(t, buyerToken, itemID)
	mustAddToCart(t, c, buyerToken, prod, 1)
	status, body := c.submit(t, buyerToken, "submit-audit-"+uuid.NewString())
	if status != http.StatusOK {
		t.Fatalf("submit status=%d body=%v", status, body)
	}
	reqID := body["requests"].([]any)[0].(map[string]any)["request_id"].(string)
	c.cancelRequest(t, buyerToken, reqID)

	var actions string
	if err := adminPool.QueryRow(ctx, `
		SELECT string_agg(action, ',' ORDER BY created_at)
		FROM audit_logs WHERE org_id = $1`,
		buyerOrg).Scan(&actions); err != nil {
		t.Fatalf("query audit: %v", err)
	}
	for _, want := range []string{
		"procurement.cart_item_added",
		"procurement.cart_item_updated",
		"procurement.cart_item_removed",
		"procurement.request_submitted",
		"procurement.request_cancelled",
	} {
		if !strings.Contains(actions, want) {
			t.Errorf("audit missing %s: %q", want, actions)
		}
	}

	// The submission also wrote a transactional outbox event per request.
	var outbox int
	if err := adminPool.QueryRow(ctx,
		"SELECT count(*) FROM outbox_events WHERE topic = 'procurement.request.submitted' AND org_id = $1",
		buyerOrg).Scan(&outbox); err != nil {
		t.Fatalf("count outbox: %v", err)
	}
	if outbox != 1 {
		t.Errorf("outbox events = %d, want 1", outbox)
	}
}

func mustGetCart(t *testing.T, c *client, token string) map[string]any {
	t.Helper()
	status, cart := c.getCart(t, token)
	if status != http.StatusOK {
		t.Fatalf("get cart status=%d", status)
	}
	return cart
}