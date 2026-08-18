"use client";

import { useEffect, useState } from "react";
import { useParams, useRouter } from "next/navigation";
import { AppShell } from "@/components/app-shell";
import { ProductForm } from "@/components/product-form";
import { SessionProvider, useSession } from "@/lib/session";
import { getProduct, problemMessage, type Product } from "@/lib/api";

export default function EditProductPage() {
  return (
    <SessionProvider>
      <AppShell>
        <EditProduct />
      </AppShell>
    </SessionProvider>
  );
}

function EditProduct() {
  const params = useParams<{ id: string }>();
  const router = useRouter();
  const { state, token } = useSession();
  const [product, setProduct] = useState<Product | null>(null);
  const [error, setError] = useState("");

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

  return (
    <div className="max-w-2xl space-y-6">
      <h1 className="text-2xl font-bold">Edit product</h1>
      {error && <p className="rounded bg-red-50 p-3 text-sm text-red-700">{error}</p>}
      {product && (
        <section className="rounded-lg border border-gray-200 bg-white p-6">
          <ProductForm
            initial={product}
            onDone={(p) => router.push(`/products/${p.product_id}`)}
          />
        </section>
      )}
    </div>
  );
}