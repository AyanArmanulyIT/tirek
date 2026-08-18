"use client";

import { FormEvent, useEffect, useState } from "react";
import {
  PRODUCT_UNITS,
  createCategory,
  createProduct,
  formatMinor,
  listCategories,
  problemMessage,
  updateProduct,
  type Category,
  type Product,
} from "@/lib/api";
import { useSession } from "@/lib/session";

const STATUSES = ["draft", "active", "archived"];
const CURRENCIES = ["KZT", "USD", "EUR"];

interface Props {
  initial?: Product;
  onDone: (p: Product) => void;
}

function toMajor(amountMinor: number): string {
  return (amountMinor / 100).toFixed(2);
}

export function ProductForm({ initial, onDone }: Props) {
  const { token } = useSession();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [categories, setCategories] = useState<Category[]>([]);
  const [newCategory, setNewCategory] = useState("");
  const [form, setForm] = useState({
    name: initial?.name ?? "",
    sku: initial?.sku ?? "",
    description: initial?.description ?? "",
    unit: initial?.unit ?? "piece",
    status: (initial?.status ?? "active") as string,
    category_id: initial?.category_id ?? "",
    image_url: initial?.image_url ?? "",
    min_order_qty: String(initial?.min_order_qty ?? 1),
    price: initial?.price ? toMajor(initial.price.amount_minor) : "0.00",
    currency: initial?.price?.currency ?? "KZT",
  });

  useEffect(() => {
    if (!token) {
      return;
    }
    listCategories(token)
      .then((res) => setCategories(res.data))
      .catch(() => setCategories([]));
  }, [token]);

  function set<K extends keyof typeof form>(key: K, value: (typeof form)[K]) {
    setForm((f) => ({ ...f, [key]: value }));
  }

  async function addCategory() {
    const name = newCategory.trim();
    if (!name || !token) {
      return;
    }
    setBusy(true);
    setError("");
    try {
      const cat = await createCategory(token, name);
      setCategories((cs) => [...cs, cat]);
      set("category_id", cat.category_id);
      setNewCategory("");
    } catch (err) {
      setError(problemMessage(err));
    } finally {
      setBusy(false);
    }
  }

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError("");
    setBusy(true);
    try {
      const priceMinor = Math.round(parseFloat(form.price) * 100);
      const payload = {
        name: form.name,
        sku: form.sku || undefined,
        description: form.description || undefined,
        unit: form.unit,
        status: form.status,
        category_id: form.category_id || undefined,
        image_url: form.image_url || undefined,
        min_order_qty: parseInt(form.min_order_qty, 10) || 1,
        price_minor: Number.isNaN(priceMinor) ? 0 : priceMinor,
        currency: form.currency,
      };
      const res = initial
        ? await updateProduct(token, initial.product_id, payload)
        : await createProduct(token, payload);
      onDone(res);
    } catch (err) {
      setError(problemMessage(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <form onSubmit={onSubmit} className="space-y-4">
      {error && <p className="rounded bg-red-50 p-3 text-sm text-red-700">{error}</p>}

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <div className="space-y-1">
          <label htmlFor="name" className="text-sm font-medium">Name *</label>
          <input
            id="name"
            required
            maxLength={200}
            value={form.name}
            onChange={(e) => set("name", e.target.value)}
            className="w-full rounded border border-gray-300 px-3 py-2"
          />
        </div>
        <div className="space-y-1">
          <label htmlFor="sku" className="text-sm font-medium">SKU</label>
          <input
            id="sku"
            maxLength={100}
            value={form.sku}
            onChange={(e) => set("sku", e.target.value)}
            className="w-full rounded border border-gray-300 px-3 py-2"
          />
        </div>
      </div>

      <div className="space-y-1">
        <label htmlFor="description" className="text-sm font-medium">Description</label>
        <textarea
          id="description"
          rows={3}
          maxLength={1000}
          value={form.description}
          onChange={(e) => set("description", e.target.value)}
          className="w-full rounded border border-gray-300 px-3 py-2"
        />
      </div>

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
        <div className="space-y-1">
          <label htmlFor="unit" className="text-sm font-medium">Unit *</label>
          <select
            id="unit"
            value={form.unit}
            onChange={(e) => set("unit", e.target.value)}
            className="w-full rounded border border-gray-300 px-3 py-2"
          >
            {PRODUCT_UNITS.map((u) => (
              <option key={u} value={u}>{u}</option>
            ))}
          </select>
        </div>
        <div className="space-y-1">
          <label htmlFor="status" className="text-sm font-medium">Status</label>
          <select
            id="status"
            value={form.status}
            onChange={(e) => set("status", e.target.value)}
            className="w-full rounded border border-gray-300 px-3 py-2"
          >
            {STATUSES.map((s) => (
              <option key={s} value={s}>{s}</option>
            ))}
          </select>
        </div>
        <div className="space-y-1">
          <label htmlFor="min_order_qty" className="text-sm font-medium">Min order qty</label>
          <input
            id="min_order_qty"
            type="number"
            min={1}
            value={form.min_order_qty}
            onChange={(e) => set("min_order_qty", e.target.value)}
            className="w-full rounded border border-gray-300 px-3 py-2"
          />
        </div>
      </div>

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <div className="space-y-1">
          <label htmlFor="price" className="text-sm font-medium">Price per unit ({form.currency})</label>
          <input
            id="price"
            type="number"
            step="0.01"
            min="0"
            value={form.price}
            onChange={(e) => set("price", e.target.value)}
            className="w-full rounded border border-gray-300 px-3 py-2"
          />
          {initial?.price && (
            <p className="text-xs text-gray-500">
              Current: {formatMinor(initial.price.amount_minor, initial.price.currency)}
            </p>
          )}
        </div>
        <div className="space-y-1">
          <label htmlFor="currency" className="text-sm font-medium">Currency</label>
          <select
            id="currency"
            value={form.currency}
            onChange={(e) => set("currency", e.target.value)}
            className="w-full rounded border border-gray-300 px-3 py-2"
          >
            {CURRENCIES.map((c) => (
              <option key={c} value={c}>{c}</option>
            ))}
          </select>
        </div>
      </div>

      <div className="space-y-1">
        <label htmlFor="category_id" className="text-sm font-medium">Category</label>
        <div className="flex gap-2">
          <select
            id="category_id"
            value={form.category_id}
            onChange={(e) => set("category_id", e.target.value)}
            className="w-full rounded border border-gray-300 px-3 py-2"
          >
            <option value="">— None —</option>
            {categories.map((cat) => (
              <option key={cat.category_id} value={cat.category_id}>{cat.name}</option>
            ))}
          </select>
          <input
            placeholder="New category…"
            value={newCategory}
            onChange={(e) => setNewCategory(e.target.value)}
            className="w-48 rounded border border-gray-300 px-3 py-2"
          />
          <button
            type="button"
            onClick={addCategory}
            disabled={busy || !newCategory.trim()}
            className="rounded border border-gray-300 px-3 py-2 text-sm hover:bg-gray-50 disabled:opacity-50"
          >
            Add
          </button>
        </div>
      </div>

      <div className="space-y-1">
        <label htmlFor="image_url" className="text-sm font-medium">Image URL</label>
        <input
          id="image_url"
          type="url"
          maxLength={2048}
          value={form.image_url}
          onChange={(e) => set("image_url", e.target.value)}
          className="w-full rounded border border-gray-300 px-3 py-2"
        />
      </div>

      <div className="flex gap-3">
        <button
          type="submit"
          disabled={busy}
          className="rounded bg-emerald-600 px-4 py-2 text-sm text-white hover:bg-emerald-700 disabled:opacity-50"
        >
          {busy ? "Saving…" : initial ? "Save changes" : "Create product"}
        </button>
      </div>
    </form>
  );
}