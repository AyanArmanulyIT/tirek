"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { AppShell } from "@/components/app-shell";
import { SessionProvider, useSession } from "@/lib/session";
import {
  cancelRequest,
  formatMinor,
  listRequests,
  problemMessage,
  type PurchaseRequest,
} from "@/lib/api";

const PAGE_SIZE = 20;

export default function ProcurementRequestsPage() {
  return (
    <SessionProvider>
      <AppShell>
        <RequestsView />
      </AppShell>
    </SessionProvider>
  );
}

function RequestsView() {
  const { state, token } = useSession();
  const searchParams = useSearchParams();
  const [requests, setRequests] = useState<PurchaseRequest[]>([]);
  const [total, setTotal] = useState(0);
  const [offset, setOffset] = useState(0);
  const [error, setError] = useState("");
  const [cancelling, setCancelling] = useState<string | null>(null);
  const justSubmitted = Number(searchParams.get("just_submitted") ?? 0);

  useEffect(() => {
    if (state.kind !== "loaded") {
      return;
    }
    listRequests(token, offset, PAGE_SIZE)
      .then((res) => {
        setRequests(res.data);
        setTotal(res.total);
      })
      .catch((err) => setError(problemMessage(err)));
  }, [state.kind, token, offset]);

  if (state.kind !== "loaded") {
    return null;
  }

  async function handleCancel(id: string) {
    setCancelling(id);
    setError("");
    try {
      await cancelRequest(token, id);
      setRequests((rs) => rs.map((r) => (r.request_id === id ? { ...r, status: "cancelled" } : r)));
    } catch (err) {
      setError(problemMessage(err));
    } finally {
      setCancelling(null);
    }
  }

  const page = Math.floor(offset / PAGE_SIZE);
  const pages = Math.max(1, Math.ceil(total / PAGE_SIZE));

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">Purchase requests</h1>
        <Link href="/procurement/cart" className="text-sm text-emerald-700 underline">
          Back to cart
        </Link>
      </div>

      {justSubmitted > 0 && (
        <p className="rounded bg-emerald-50 p-3 text-sm text-emerald-700">
          Submitted {justSubmitted} purchase request{justSubmitted === 1 ? "" : "s"}. Suppliers have
          been notified.
        </p>
      )}

      {error && <p className="rounded bg-red-50 p-3 text-sm text-red-700">{error}</p>}

      {requests.length === 0 ? (
        <section className="rounded-lg border border-gray-200 bg-white p-6">
          <p className="text-sm text-gray-600">
            No purchase requests yet.{" "}
            <Link href="/procurement/cart" className="text-emerald-700 underline">
              Build a cart
            </Link>{" "}
            to send requests to suppliers.
          </p>
        </section>
      ) : (
        <ul className="space-y-3">
          {requests.map((req) => (
            <li key={req.request_id} className="rounded-lg border border-gray-200 bg-white p-4">
              <div className="flex items-center justify-between gap-4">
                <div>
                  <Link
                    href={`/procurement/requests/${req.request_id}`}
                    className="font-semibold text-emerald-700 hover:underline"
                  >
                    {req.number}
                  </Link>
                  <p className="text-xs text-gray-500">
                    {req.supplier_name} · {req.items.length} item{req.items.length === 1 ? "" : "s"} ·{" "}
                    {new Date(req.created_at).toLocaleString()}
                  </p>
                </div>
                <div className="flex items-center gap-3">
                  <span className="font-medium">
                    {formatMinor(req.total_minor, req.currency)}
                  </span>
                  <span
                    className={`rounded-full px-2 py-0.5 text-xs ${
                      req.status === "submitted"
                        ? "bg-emerald-100 text-emerald-700"
                        : "bg-gray-100 text-gray-500"
                    }`}
                  >
                    {req.status}
                  </span>
                  {req.status === "submitted" && (
                    <button
                      onClick={() => handleCancel(req.request_id)}
                      disabled={cancelling === req.request_id}
                      className="rounded border border-red-200 px-2 py-1 text-xs text-red-700 hover:bg-red-50 disabled:opacity-50"
                    >
                      {cancelling === req.request_id ? "…" : "Cancel"}
                    </button>
                  )}
                </div>
              </div>
            </li>
          ))}
        </ul>
      )}

      {pages > 1 && (
        <div className="flex items-center gap-3">
          <button
            onClick={() => setOffset((o) => Math.max(0, o - PAGE_SIZE))}
            disabled={page === 0}
            className="rounded border border-gray-300 px-3 py-1.5 text-sm hover:bg-gray-50 disabled:opacity-50"
          >
            Previous
          </button>
          <span className="text-sm text-gray-600">Page {page + 1} of {pages}</span>
          <button
            onClick={() => setOffset((o) => o + PAGE_SIZE)}
            disabled={page + 1 >= pages}
            className="rounded border border-gray-300 px-3 py-1.5 text-sm hover:bg-gray-50 disabled:opacity-50"
          >
            Next
          </button>
        </div>
      )}
    </div>
  );
}