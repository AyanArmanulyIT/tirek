"use client";

import { useEffect, useState } from "react";
import { formatMinor, problemMessage, type PurchaseRequest } from "@/lib/api";

interface Props {
  token: string;
  requestId: string;
  fetchFn: (token: string, id: string) => Promise<PurchaseRequest>;
  onCancel?: (requestId: string) => Promise<unknown>;
  showBuyer?: boolean;
}

export function RequestDetail({ token, requestId, fetchFn, onCancel, showBuyer }: Props) {
  const [req, setReq] = useState<PurchaseRequest | null>(null);
  const [error, setError] = useState("");
  const [cancelling, setCancelling] = useState(false);

  useEffect(() => {
    fetchFn(token, requestId)
      .then(setReq)
      .catch((err) => setError(problemMessage(err)));
  }, [token, requestId, fetchFn]);

  if (error) {
    return <p className="rounded bg-red-50 p-3 text-sm text-red-700">{error}</p>;
  }

  if (!req) {
    return <p className="text-sm text-gray-500">Loading…</p>;
  }

  const requestIdConst = req.request_id;

  async function handleCancel() {
    if (!onCancel) {
      return;
    }
    setCancelling(true);
    setError("");
    try {
      await onCancel(requestIdConst);
      setReq((r) => (r ? { ...r, status: "cancelled" } : r));
    } catch (err) {
      setError(problemMessage(err));
    } finally {
      setCancelling(false);
    }
  }

  return (
    <div className="space-y-6">
      {error && <p className="rounded bg-red-50 p-3 text-sm text-red-700">{error}</p>}

      <section className="rounded-lg border border-gray-200 bg-white p-5">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-xl font-bold">{req.number}</h1>
            <p className="text-sm text-gray-600">
              {showBuyer ? req.buyer_name : req.supplier_name} · {new Date(req.created_at).toLocaleString()}
            </p>
          </div>
          <div className="flex items-center gap-3">
            <span
              className={`rounded-full px-2 py-0.5 text-xs ${
                req.status === "submitted"
                  ? "bg-emerald-100 text-emerald-700"
                  : "bg-gray-100 text-gray-500"
              }`}
            >
              {req.status}
            </span>
            {onCancel && req.status === "submitted" && (
              <button
                onClick={handleCancel}
                disabled={cancelling}
                className="rounded border border-red-200 px-3 py-1.5 text-sm text-red-700 hover:bg-red-50 disabled:opacity-50"
              >
                {cancelling ? "Cancelling…" : "Cancel request"}
              </button>
            )}
          </div>
        </div>
      </section>

      <section className="rounded-lg border border-gray-200 bg-white p-5">
        <h2 className="mb-3 font-semibold">Items</h2>
        <table className="w-full text-left text-sm">
          <thead>
            <tr className="border-b border-gray-100 text-xs uppercase text-gray-500">
              <th className="pb-2 font-medium">Product</th>
              <th className="pb-2 font-medium">Qty</th>
              <th className="pb-2 font-medium">Unit price</th>
              <th className="pb-2 text-right font-medium">Line total</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-gray-100">
            {req.items.map((item) => (
              <tr key={item.request_item_id}>
                <td className="py-2">
                  <p className="font-medium">{item.product_name}</p>
                  {item.sku && <p className="text-xs text-gray-500">SKU {item.sku}</p>}
                </td>
                <td className="py-2">
                  {item.quantity} {item.unit}
                </td>
                <td className="py-2">{formatMinor(item.unit_price_minor, item.currency)}</td>
                <td className="py-2 text-right">{formatMinor(item.line_total_minor, item.currency)}</td>
              </tr>
            ))}
          </tbody>
          <tfoot>
            <tr className="border-t border-gray-200">
              <td colSpan={3} className="py-2 text-right font-medium">
                Total
              </td>
              <td className="py-2 text-right font-semibold">
                {formatMinor(req.total_minor, req.currency)}
              </td>
            </tr>
          </tfoot>
        </table>
      </section>
    </div>
  );
}