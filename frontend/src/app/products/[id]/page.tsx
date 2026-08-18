"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { AppShell } from "@/components/app-shell";
import { SessionProvider, useSession } from "@/lib/session";
import {
  can,
  formatMinor,
  getProduct,
  problemMessage,
  updateProduct,
  type Product,
} from "@/lib/api";

export default function ProductDetailPage() {
  return (
    <SessionProvider>
      <AppShell>
        <ProductDetail />
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

function ProductDetail() {
  const params = useParams<{ id: string }>();
  const router = useRouter();
  const { state, token } = useSession();
  const [product, setProduct] = useState<Product | null>(null);
  const [error, setError] = useState("");
  const [archiving, setArchiving] = useState(false);

  useEffect(() => {
    if (state.kind !== "loaded") {
      return;
    }
    getProduct(token, params.id)
      .then(setProduct)
      .catch((err) => setError(problemMessage(err)));
  }, [state.kind, token, params.id]);

  if (state.kind !== "loaded") {
    return null;
  }
  const canManage = can(state.data, "catalog", "manage");

  async function archive() {
    if (!product) {
      return;
    }
    setArchiving(true);
    setError("");
    try {
      const updated = await updateProduct(token, product.product_id, {
        name: product.name,
        sku: product.sku || undefined,
        description: product.description || undefined,
        unit: product.unit,
        status: "archived",
        category_id: product.category_id || undefined,
        image_url: product.image_url || undefined,
        min_order_qty: product.min_order_qty,
        price_minor: product.price?.amount_minor ?? 0,
        currency: product.price?.currency ?? "KZT",
      });
      setProduct(updated);
    } catch (err) {
      setError(problemMessage(err));
    } finally {
      setArchiving(false);
    }
  }

  return (
    <div className="max-w-2xl space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">{product?.name ?? "Product"}</h1>
        {canManage && product?.status !== "archived" && (
          <div className="flex gap-2">
            <Link
              href={`/products/${params.id}/edit`}
              className="rounded bg-emerald-600 px-4 py-2 text-sm text-white hover:bg-emerald-700"
            >
              Edit
            </Link>
            <button
              onClick={archive}
              disabled={archiving}
              className="rounded border border-red-300 px-4 py-2 text-sm text-red-700 hover:bg-red-50 disabled:opacity-50"
            >
              {archiving ? "Archiving…" : "Archive"}
            </button>
          </div>
        )}
      </div>

      {error && <p className="rounded bg-red-50 p-3 text-sm text-red-700">{error}</p>}

      {product && (
        <section className="rounded-lg border border-gray-200 bg-white p-6">
          <dl className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <div>
              <dt className="text-xs uppercase text-gray-500">Status</dt>
              <dd className="mt-1">{statusBadge(product.status)}</dd>
            </div>
            <div>
              <dt className="text-xs uppercase text-gray-500">SKU</dt>
              <dd className="mt-1 text-sm">{product.sku ?? "—"}</dd>
            </div>
            <div>
              <dt className="text-xs uppercase text-gray-500">Unit</dt>
              <dd className="mt-1 text-sm">{product.unit}</dd>
            </div>
            <div>
              <dt className="text-xs uppercase text-gray-500">Min order quantity</dt>
              <dd className="mt-1 text-sm">{product.min_order_qty}</dd>
            </div>
            <div>
              <dt className="text-xs uppercase text-gray-500">Price per unit</dt>
              <dd className="mt-1 text-sm">
                {product.price ? formatMinor(product.price.amount_minor, product.price.currency) : "—"}
              </dd>
            </div>
            <div>
              <dt className="text-xs uppercase text-gray-500">VAT</dt>
              <dd className="mt-1 text-sm">{(product.vat_rate_bps / 100).toFixed(2)}%</dd>
            </div>
          </dl>
          {product.description && (
            <div className="mt-4">
              <dt className="text-xs uppercase text-gray-500">Description</dt>
              <dd className="mt-1 text-sm text-gray-700">{product.description}</dd>
            </div>
          )}
          {product.image_url && (
            <div className="mt-4">
              {/* eslint-disable-next-line @next/next/no-img-element */}
              <img
                src={product.image_url}
                alt={product.name}
                className="h-32 w-32 rounded-lg object-cover"
              />
            </div>
          )}
          <div className="mt-6">
            <Link href="/products" className="text-sm text-emerald-700 hover:underline">
              ← Back to products
            </Link>
          </div>
        </section>
      )}
    </div>
  );
}