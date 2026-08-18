"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { AppShell } from "@/components/app-shell";
import { SessionProvider, useSession } from "@/lib/session";
import {
  can,
  formatMinor,
  listProducts,
  problemMessage,
  type Product,
} from "@/lib/api";

export default function ProductsPage() {
  return (
    <SessionProvider>
      <AppShell>
        <ProductsList />
      </AppShell>
    </SessionProvider>
  );
}

function statusBadge(status: string) {
  const styles: Record<string, string> = {
    draft: "bg-gray-100 text-gray-600",
    active: "bg-emerald-100 text-emerald-700",
    archived: "bg-red-100 text-red-700",
  };
  return (
    <span className={`rounded-full px-2 py-0.5 text-xs font-medium ${styles[status] ?? "bg-gray-100 text-gray-600"}`}>
      {status}
    </span>
  );
}

function ProductsList() {
  const { state, token } = useSession();
  const [items, setItems] = useState<Product[]>([]);
  const [error, setError] = useState("");

  useEffect(() => {
    if (state.kind !== "loaded") {
      return;
    }
    listProducts(token)
      .then((res) => setItems(res.data))
      .catch((err) => setError(problemMessage(err)));
  }, [state.kind, token]);

  if (state.kind !== "loaded") {
    return null;
  }
  const canManage = can(state.data, "catalog", "manage");

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">Products</h1>
        {canManage && (
          <Link
            href="/products/new"
            className="rounded bg-emerald-600 px-4 py-2 text-sm text-white hover:bg-emerald-700"
          >
            New product
          </Link>
        )}
      </div>

      {error && <p className="rounded bg-red-50 p-3 text-sm text-red-700">{error}</p>}

      {items.length === 0 ? (
        <section className="rounded-lg border border-gray-200 bg-white p-6">
          <p className="text-sm text-gray-600">No products yet.</p>
          {canManage && (
            <Link href="/products/new" className="mt-2 inline-block text-sm text-emerald-700 underline">
              Create your first product
            </Link>
          )}
        </section>
      ) : (
        <div className="overflow-hidden rounded-lg border border-gray-200 bg-white">
          <table className="w-full text-left text-sm">
            <thead className="bg-gray-50 text-xs uppercase text-gray-500">
              <tr>
                <th className="px-4 py-3">Name</th>
                <th className="px-4 py-3">SKU</th>
                <th className="px-4 py-3">Unit</th>
                <th className="px-4 py-3">Status</th>
                <th className="px-4 py-3">Price</th>
                <th className="px-4 py-3" />
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100">
              {items.map((p) => (
                <tr key={p.product_id} className="hover:bg-gray-50">
                  <td className="px-4 py-3">
                    <Link href={`/products/${p.product_id}`} className="font-medium text-emerald-700 hover:underline">
                      {p.name}
                    </Link>
                  </td>
                  <td className="px-4 py-3 text-gray-600">{p.sku ?? "—"}</td>
                  <td className="px-4 py-3 text-gray-600">{p.unit}</td>
                  <td className="px-4 py-3">{statusBadge(p.status)}</td>
                  <td className="px-4 py-3 text-gray-700">
                    {p.price ? formatMinor(p.price.amount_minor, p.price.currency) : "—"}
                  </td>
                  <td className="px-4 py-3 text-right">
                    <Link href={`/products/${p.product_id}/edit`} className="text-sm text-emerald-700 hover:underline">
                      Edit
                    </Link>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}