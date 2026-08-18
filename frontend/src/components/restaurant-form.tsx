"use client";

import { FormEvent, useState } from "react";
import { createRestaurant, updateRestaurant, problemMessage, type Restaurant } from "@/lib/api";
import { useSession } from "@/lib/session";

const STATUSES = ["active", "suspended", "closed"];

interface Props {
  initial?: Restaurant;
  onDone: (r: Restaurant) => void;
}

export function RestaurantForm({ initial, onDone }: Props) {
  const { token } = useSession();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [form, setForm] = useState({
    name: initial?.name ?? "",
    legal_name: initial?.legal_name ?? "",
    status: (initial?.status ?? "active") as string,
    country: initial?.country ?? "KZ",
    default_currency: initial?.default_currency ?? "KZT",
    phone: initial?.phone ?? "",
    email: initial?.email ?? "",
  });

  function set<K extends keyof typeof form>(key: K, value: (typeof form)[K]) {
    setForm((f) => ({ ...f, [key]: value }));
  }

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError("");
    setBusy(true);
    try {
      const payload = {
        name: form.name,
        legal_name: form.legal_name || undefined,
        status: form.status,
        country: form.country,
        default_currency: form.default_currency,
        phone: form.phone || undefined,
        email: form.email || undefined,
      };
      const res = initial
        ? await updateRestaurant(token, initial.restaurant_id, payload)
        : await createRestaurant(token, payload);
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
        <label htmlFor="legal_name" className="text-sm font-medium">Legal name</label>
        <input
          id="legal_name"
          maxLength={200}
          value={form.legal_name}
          onChange={(e) => set("legal_name", e.target.value)}
          className="w-full rounded border border-gray-300 px-3 py-2"
        />
      </div>
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
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
          <label htmlFor="country" className="text-sm font-medium">Country (ISO)</label>
          <input
            id="country"
            maxLength={2}
            value={form.country}
            onChange={(e) => set("country", e.target.value.toUpperCase())}
            className="w-full rounded border border-gray-300 px-3 py-2"
          />
        </div>
        <div className="space-y-1">
          <label htmlFor="default_currency" className="text-sm font-medium">Currency (ISO)</label>
          <input
            id="default_currency"
            maxLength={3}
            value={form.default_currency}
            onChange={(e) => set("default_currency", e.target.value.toUpperCase())}
            className="w-full rounded border border-gray-300 px-3 py-2"
          />
        </div>
      </div>
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <div className="space-y-1">
          <label htmlFor="phone" className="text-sm font-medium">Phone</label>
          <input
            id="phone"
            maxLength={32}
            value={form.phone}
            onChange={(e) => set("phone", e.target.value)}
            className="w-full rounded border border-gray-300 px-3 py-2"
          />
        </div>
        <div className="space-y-1">
          <label htmlFor="email" className="text-sm font-medium">Email</label>
          <input
            id="email"
            type="email"
            value={form.email}
            onChange={(e) => set("email", e.target.value)}
            className="w-full rounded border border-gray-300 px-3 py-2"
          />
        </div>
      </div>
      <div className="flex gap-3">
        <button
          type="submit"
          disabled={busy}
          className="rounded bg-emerald-600 px-4 py-2 text-sm text-white hover:bg-emerald-700 disabled:opacity-50"
        >
          {busy ? "Saving…" : initial ? "Save changes" : "Create restaurant"}
        </button>
      </div>
    </form>
  );
}