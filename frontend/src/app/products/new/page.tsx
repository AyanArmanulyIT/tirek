"use client";

import { useRouter } from "next/navigation";
import { AppShell } from "@/components/app-shell";
import { ProductForm } from "@/components/product-form";
import { SessionProvider } from "@/lib/session";

export default function NewProductPage() {
  return (
    <SessionProvider>
      <AppShell>
        <NewProduct />
      </AppShell>
    </SessionProvider>
  );
}

function NewProduct() {
  const router = useRouter();
  return (
    <div className="max-w-2xl space-y-6">
      <h1 className="text-2xl font-bold">New product</h1>
      <section className="rounded-lg border border-gray-200 bg-white p-6">
        <ProductForm onDone={(p) => router.push(`/products/${p.product_id}`)} />
      </section>
    </div>
  );
}