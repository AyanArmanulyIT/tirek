"use client";

import { FormEvent, useEffect, useState } from "react";
import Link from "next/link";
import { useParams } from "next/navigation";
import { AppShell } from "@/components/app-shell";
import { SessionProvider, useSession } from "@/lib/session";
import {
  can,
  createLocation,
  getRestaurant,
  listLocations,
  problemMessage,
  updateLocation,
  type Location,
  type Restaurant,
} from "@/lib/api";

export default function LocationsPage() {
  return (
    <SessionProvider>
      <AppShell>
        <Locations />
      </AppShell>
    </SessionProvider>
  );
}

function Locations() {
  const params = useParams<{ id: string }>();
  const { state, token } = useSession();
  const [restaurant, setRestaurant] = useState<Restaurant | null>(null);
  const [locations, setLocations] = useState<Location[]>([]);
  const [editing, setEditing] = useState<string | null>(null);
  const [error, setError] = useState("");

  async function refresh() {
    try {
      const res = await listLocations(token, params.id);
      setLocations(res.data);
    } catch (err) {
      setError(problemMessage(err));
    }
  }

  useEffect(() => {
    if (state.kind !== "loaded") {
      return;
    }
    getRestaurant(token, params.id)
      .then((r) => {
        setRestaurant(r);
        return listLocations(token, r.restaurant_id);
      })
      .then((res) => setLocations(res.data))
      .catch((err) => setError(problemMessage(err)));
  }, [state.kind, token, params.id]);

  if (state.kind !== "loaded") {
    return null;
  }
  const canWrite = can(state.data, "restaurants", "write");

  if (!restaurant) {
    return <p className="text-gray-500">Loading…</p>;
  }

  return (
    <div className="max-w-3xl space-y-6">
      <div>
        <Link href={`/restaurants/${restaurant.restaurant_id}`} className="text-sm text-emerald-700 hover:underline">
          ← {restaurant.name}
        </Link>
        <h1 className="mt-1 text-2xl font-bold">Locations</h1>
      </div>

      {error && <p className="rounded bg-red-50 p-3 text-sm text-red-700">{error}</p>}

      {canWrite && (
        <section className="rounded-lg border border-gray-200 bg-white p-6">
          <h2 className="text-lg font-semibold">Add location</h2>
          <LocationForm
            key="new"
            restaurantId={restaurant.restaurant_id}
            onSubmit={async (payload) => {
              await createLocation(token, restaurant.restaurant_id, payload);
              await refresh();
            }}
          />
        </section>
      )}

      <section className="rounded-lg border border-gray-200 bg-white p-6">
        <h2 className="text-lg font-semibold">All locations</h2>
        {locations.length === 0 ? (
          <p className="mt-3 text-sm text-gray-600">No locations yet.</p>
        ) : (
          <ul className="mt-3 divide-y divide-gray-100">
            {locations.map((l) => (
              <li key={l.location_id} className="py-3">
                <div className="flex items-center justify-between">
                  <div>
                    <span className="font-medium">{l.name}</span>
                    <span className="ml-2 text-gray-500">
                      {l.address}, {l.city} ({l.country})
                    </span>
                  </div>
                  <div className="flex items-center gap-3">
                    <span className={`rounded-full px-2 py-0.5 text-xs ${l.status === "active" ? "bg-emerald-50 text-emerald-800" : "bg-gray-100 text-gray-600"}`}>
                      {l.status}
                    </span>
                    {canWrite && (
                      <button
                        onClick={() => setEditing(editing === l.location_id ? null : l.location_id)}
                        className="text-sm text-emerald-700 hover:underline"
                      >
                        {editing === l.location_id ? "Cancel" : "Edit"}
                      </button>
                    )}
                  </div>
                </div>
                {canWrite && editing === l.location_id && (
                  <div className="mt-3 rounded bg-gray-50 p-4">
                    <LocationForm
                      key={l.location_id}
                      restaurantId={restaurant.restaurant_id}
                      initial={l}
                      onSubmit={async (payload) => {
                        await updateLocation(token, restaurant.restaurant_id, l.location_id, payload);
                        setEditing(null);
                        await refresh();
                      }}
                    />
                  </div>
                )}
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  );
}

interface LocationFormProps {
  restaurantId: string;
  initial?: Location;
  onSubmit: (payload: {
    name: string;
    address: string;
    city: string;
    country: string;
    status: string;
    phone?: string;
    email?: string;
  }) => Promise<void>;
}

function LocationForm({ initial, onSubmit }: LocationFormProps) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [form, setForm] = useState({
    name: initial?.name ?? "",
    address: initial?.address ?? "",
    city: initial?.city ?? "",
    country: initial?.country ?? "KZ",
    status: (initial?.status ?? "active") as string,
    phone: initial?.phone ?? "",
    email: initial?.email ?? "",
  });

  function set<K extends keyof typeof form>(key: K, value: (typeof form)[K]) {
    setForm((f) => ({ ...f, [key]: value }));
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setError("");
    setBusy(true);
    try {
      await onSubmit({
        name: form.name,
        address: form.address,
        city: form.city,
        country: form.country,
        status: form.status,
        phone: form.phone || undefined,
        email: form.email || undefined,
      });
      if (!initial) {
        setForm({ name: "", address: "", city: "", country: "KZ", status: "active", phone: "", email: "" });
      }
    } catch (err) {
      setError(problemMessage(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-3">
      {error && <p className="rounded bg-red-50 p-3 text-sm text-red-700">{error}</p>}
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
        <div className="space-y-1">
          <label className="text-sm font-medium">Name *</label>
          <input
            required
            data-testid="location-name"
            value={form.name}
            onChange={(e) => set("name", e.target.value)}
            className="w-full rounded border border-gray-300 px-3 py-2"
          />
        </div>
        <div className="space-y-1">
          <label className="text-sm font-medium">City *</label>
          <input
            required
            data-testid="location-city"
            value={form.city}
            onChange={(e) => set("city", e.target.value)}
            className="w-full rounded border border-gray-300 px-3 py-2"
          />
        </div>
        <div className="space-y-1 sm:col-span-2">
          <label className="text-sm font-medium">Address *</label>
          <input
            required
            data-testid="location-address"
            value={form.address}
            onChange={(e) => set("address", e.target.value)}
            className="w-full rounded border border-gray-300 px-3 py-2"
          />
        </div>
        <div className="space-y-1">
          <label className="text-sm font-medium">Country (ISO)</label>
          <input
            maxLength={2}
            value={form.country}
            onChange={(e) => set("country", e.target.value.toUpperCase())}
            className="w-full rounded border border-gray-300 px-3 py-2"
          />
        </div>
        <div className="space-y-1">
          <label className="text-sm font-medium">Status</label>
          <select
            value={form.status}
            onChange={(e) => set("status", e.target.value)}
            className="w-full rounded border border-gray-300 px-3 py-2"
          >
            <option value="active">active</option>
            <option value="inactive">inactive</option>
          </select>
        </div>
        <div className="space-y-1">
          <label className="text-sm font-medium">Phone</label>
          <input
            data-testid="location-phone"
            value={form.phone}
            onChange={(e) => set("phone", e.target.value)}
            className="w-full rounded border border-gray-300 px-3 py-2"
          />
        </div>
        <div className="space-y-1">
          <label className="text-sm font-medium">Email</label>
          <input
            type="email"
            value={form.email}
            onChange={(e) => set("email", e.target.value)}
            className="w-full rounded border border-gray-300 px-3 py-2"
          />
        </div>
      </div>
      <button
        type="submit"
        disabled={busy}
        className="rounded bg-emerald-600 px-4 py-2 text-sm text-white hover:bg-emerald-700 disabled:opacity-50"
      >
        {busy ? "Saving…" : initial ? "Save" : "Add location"}
      </button>
    </form>
  );
}