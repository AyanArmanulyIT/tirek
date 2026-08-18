"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { AppShell } from "@/components/app-shell";
import { SessionProvider, useSession } from "@/lib/session";
import {
  formatMinor,
  listIncoming,
  problemMessage,
  type PurchaseRequest,
} from "@/lib/api";

const PAGE_SIZE = 20;

export default function ProcurementIncomingPage() {
  return (
    <SessionProvider>
      <AppShell>
        <IncomingView />
      </AppShell>
    </SessionProvider>
  );
}

function IncomingView() {
  const { state, token } = useSession();
  const [requests, setRequests] = useState<PurchaseRequest[]>([]);
  const [total, setTotal] = useState(0);
  const [offset, setOffset] = useState(0);
  const [error, setError] = useState("");

  useEffect(() => {
    if (state.kind !== "loaded") {
      return;
    }
    listIncoming(token, offset, PAGE_SIZE)
      .then((res) => {
        setRequests(res.data);
        setTotal(res.total);
      })
      .catch((err) => setError(problemMessage(err)));
  }, [state.kind, token, offset]);

  if (state.kind !== "loaded") {
    return null;
  }

  const page = Math.floor(offset / PAGE_SIZE);
  const pages = Math.max(1, Math.ceil(total / PAGE_SIZE));

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">Incoming purchase requests</h1>
        <Link href="/products" className="text-sm text-emerald-700 underline">
          Manage products
        </Link>
      </div>

      {error && <p className="rounded bg-red-50 p-3 text-sm text-red-700">{error}</p>}

      {requests.length === 0 ? (
        <section className="rounded-lg border border-gray-200 bg-white p-6">
          <p className="text-sm text-gray-600">No incoming purchase requests yet.</p>
        </section>
      ) : (
        <ul className="space-y-3">
          {requests.map((req) => (
            <li key={req.request_id} className="rounded-lg border border-gray-200 bg-white p-4">
              <div className="flex items-center justify-between gap-4">
                <div>
                  <Link
                    href={`/procurement/incoming/${req.request_id}`}
                    className="font-semibold text-emerald-700 hover:underline"
                  >
                    {req.number}
                  </Link>
                  <p className="text-xs text-gray-500">
                    {req.buyer_name} · {req.items.length} item{req.items.length === 1 ? "" : "s"} ·{" "}
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