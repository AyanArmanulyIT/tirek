"use client";

import { useEffect, useState } from "react";
import { AppShell } from "@/components/app-shell";
import { SessionProvider, useSession } from "@/lib/session";
import {
  browseCatalog,
  browseCategories,
  formatMinor,
  problemMessage,
  type CatalogProduct,
  type Category,
} from "@/lib/api";

const PAGE_SIZE = 20;

export default function CatalogPage() {
  return (
    <SessionProvider>
      <AppShell>
        <Catalog />
      </AppShell>
    </SessionProvider>
  );
}

function Catalog() {
  const { state, token } = useSession();
  const [items, setItems] = useState<CatalogProduct[]>([]);
  const [categories, setCategories] = useState<Category[]>([]);
  const [total, setTotal] = useState(0);
  const [q, setQ] = useState("");
  const [categoryId, setCategoryId] = useState("");
  const [offset, setOffset] = useState(0);
  const [error, setError] = useState("");

  useEffect(() => {
    if (state.kind !== "loaded") {
      return;
    }
    browseCategories(token)
      .then((res) => setCategories(res.data))
      .catch(() => setCategories([]));
  }, [state.kind, token]);

  useEffect(() => {
    if (state.kind !== "loaded") {
      return;
    }
    browseCatalog(token, {
      q: q || undefined,
      category_id: categoryId || undefined,
      limit: PAGE_SIZE,
      offset,
    })
      .then((res) => {
        setItems(res.data);
        setTotal(res.total);
      })
      .catch((err) => setError(problemMessage(err)));
  }, [state.kind, token, q, categoryId, offset]);

  if (state.kind !== "loaded") {
    return null;
  }

  const page = Math.floor(offset / PAGE_SIZE);
  const pages = Math.max(1, Math.ceil(total / PAGE_SIZE));

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">Marketplace</h1>
      </div>

      <div className="flex flex-wrap items-center gap-3">
        <input
          value={q}
          onChange={(e) => {
            setQ(e.target.value);
            setOffset(0);
          }}
          placeholder="Search products…"
          className="w-64 rounded border border-gray-300 px-3 py-2 text-sm"
        />
        <select
          value={categoryId}
          onChange={(e) => {
            setCategoryId(e.target.value);
            setOffset(0);
          }}
          className="rounded border border-gray-300 px-3 py-2 text-sm"
        >
          <option value="">All categories</option>
          {categories.map((cat) => (
            <option key={cat.category_id} value={cat.category_id}>{cat.name}</option>
          ))}
        </select>
        <span className="text-sm text-gray-500">{total} product{total === 1 ? "" : "s"}</span>
      </div>

      {error && <p className="rounded bg-red-50 p-3 text-sm text-red-700">{error}</p>}

      {items.length === 0 ? (
        <section className="rounded-lg border border-gray-200 bg-white p-6">
          <p className="text-sm text-gray-600">No products match your search.</p>
        </section>
      ) : (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {items.map((p) => (
            <article key={p.product_id} className="rounded-lg border border-gray-200 bg-white p-4">
              {p.image_url ? (
                // eslint-disable-next-line @next/next/no-img-element
                <img src={p.image_url} alt={p.name} className="mb-3 h-32 w-full rounded-md object-cover" />
              ) : (
                <div className="mb-3 flex h-32 items-center justify-center rounded-md bg-gray-100 text-3xl">
                  📦
                </div>
              )}
              <h2 className="font-semibold">{p.name}</h2>
              <p className="text-xs text-gray-500">
                {p.supplier_name}
                {p.category_name ? ` · ${p.category_name}` : ""}
              </p>
              <div className="mt-2 flex items-center justify-between">
                <span className="font-medium text-emerald-700">
                  {formatMinor(p.price.amount_minor, p.price.currency)}
                </span>
                <span className="text-xs text-gray-500">per {p.unit}</span>
              </div>
            </article>
          ))}
        </div>
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