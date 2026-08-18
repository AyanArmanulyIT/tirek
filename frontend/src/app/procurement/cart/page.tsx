"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { AppShell } from "@/components/app-shell";
import { SessionProvider, useSession } from "@/lib/session";
import {
  formatMinor,
  getCart,
  problemMessage,
  removeCartItem,
  submitCart,
  updateCartItem,
  type Cart,
} from "@/lib/api";

export default function ProcurementCartPage() {
  return (
    <SessionProvider>
      <AppShell>
        <CartView />
      </AppShell>
    </SessionProvider>
  );
}

function CartView() {
  const { state, token } = useSession();
  const router = useRouter();
  const [cart, setCart] = useState<Cart | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [confirming, setConfirming] = useState(false);

  const load = useCallback(() => {
    if (state.kind !== "loaded") {
      return;
    }
    getCart(token)
      .then(setCart)
      .catch((err) => setError(problemMessage(err)));
  }, [state.kind, token]);

  useEffect(() => {
    load();
  }, [load]);

  if (state.kind !== "loaded") {
    return null;
  }

  async function changeQty(itemId: string, qty: number) {
    setBusy(true);
    setError("");
    try {
      setCart(await updateCartItem(token, itemId, qty));
    } catch (err) {
      setError(problemMessage(err));
    } finally {
      setBusy(false);
    }
  }

  async function handleRemove(itemId: string) {
    setBusy(true);
    setError("");
    try {
      setCart(await removeCartItem(token, itemId));
    } catch (err) {
      setError(problemMessage(err));
    } finally {
      setBusy(false);
    }
  }

  async function handleSubmit() {
    setBusy(true);
    setError("");
    try {
      const key =
        typeof crypto !== "undefined" && "randomUUID" in crypto
          ? crypto.randomUUID()
          : undefined;
      const res = await submitCart(token, key);
      setCart(null);
      setConfirming(false);
      router.push(`/procurement/requests?just_submitted=${res.requests.length}`);
    } catch (err) {
      setError(problemMessage(err));
    } finally {
      setBusy(false);
    }
  }

  const totalCount = cart?.groups.reduce(
    (sum, g) => sum + g.items.reduce((s, it) => s + it.quantity, 0),
    0,
  ) ?? 0;

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">Procurement cart</h1>
        <Link href="/catalog" className="text-sm text-emerald-700 underline">
          Browse products
        </Link>
      </div>

      {error && <p className="rounded bg-red-50 p-3 text-sm text-red-700">{error}</p>}

      {!cart || cart.groups.length === 0 ? (
        <section className="rounded-lg border border-gray-200 bg-white p-6">
          <p className="text-sm text-gray-600">
            Your cart is empty.{" "}
            <Link href="/catalog" className="text-emerald-700 underline">
              Browse the marketplace
            </Link>{" "}
            to add products.
          </p>
        </section>
      ) : (
        <>
          {cart.groups.map((group) => (
            <section key={group.supplier_org_id} className="rounded-lg border border-gray-200 bg-white p-5">
              <div className="mb-3 flex items-center justify-between">
                <h2 className="font-semibold">{group.supplier_name}</h2>
                <span className="text-sm text-gray-600">
                  Subtotal {formatMinor(group.total_minor, group.currency)}
                </span>
              </div>
              <ul className="divide-y divide-gray-100">
                {group.items.map((item) => (
                  <li key={item.cart_item_id} className="flex items-center justify-between gap-4 py-3">
                    <div className="min-w-0">
                      <p className="truncate font-medium">{item.product_name}</p>
                      <p className="text-xs text-gray-500">
                        {formatMinor(item.unit_price_minor, item.currency)} / {item.unit} · frozen at
                        add time
                      </p>
                    </div>
                    <div className="flex items-center gap-3">
                      <input
                        type="number"
                        min={1}
                        value={item.quantity}
                        disabled={busy}
                        onChange={(e) => changeQty(item.cart_item_id, Number(e.target.value))}
                        className="w-20 rounded border border-gray-300 px-2 py-1 text-sm"
                      />
                      <span className="w-24 text-right text-sm text-gray-700">
                        {formatMinor(item.line_total_minor, item.currency)}
                      </span>
                      <button
                        onClick={() => handleRemove(item.cart_item_id)}
                        disabled={busy}
                        className="rounded border border-red-200 px-2 py-1 text-xs text-red-700 hover:bg-red-50 disabled:opacity-50"
                      >
                        Remove
                      </button>
                    </div>
                  </li>
                ))}
              </ul>
            </section>
          ))}

          <section className="rounded-lg border border-gray-200 bg-white p-5">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm text-gray-600">
                  {cart.groups.length} supplier{cart.groups.length === 1 ? "" : "s"} · {totalCount} item{totalCount === 1 ? "" : "s"}
                </p>
                <p className="text-lg font-semibold">
                  Total {formatMinor(cart.total_minor, cart.currency)}
                </p>
              </div>
              <button
                onClick={() => setConfirming(true)}
                disabled={busy}
                className="rounded bg-emerald-600 px-4 py-2 text-sm text-white hover:bg-emerald-700 disabled:opacity-50"
              >
                Submit purchase requests
              </button>
            </div>
          </section>

          {confirming && (
            <section className="rounded-lg border border-amber-200 bg-amber-50 p-5">
              <p className="text-sm text-amber-900">
                Submitting the cart creates one purchase request per supplier. The quantities and
                prices are frozen and sent to each supplier. This cannot be undone — you can only
                cancel an unsent request afterwards.
              </p>
              <div className="mt-3 flex items-center gap-3">
                <button
                  onClick={handleSubmit}
                  disabled={busy}
                  className="rounded bg-emerald-600 px-4 py-2 text-sm text-white hover:bg-emerald-700 disabled:opacity-50"
                >
                  {busy ? "Submitting…" : "Confirm submission"}
                </button>
                <button
                  onClick={() => setConfirming(false)}
                  disabled={busy}
                  className="rounded border border-gray-300 px-3 py-2 text-sm hover:bg-gray-50"
                >
                  Go back
                </button>
              </div>
            </section>
          )}
        </>
      )}
    </div>
  );
}