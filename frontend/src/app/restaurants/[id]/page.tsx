"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { notFound, useParams } from "next/navigation";
import { AppShell } from "@/components/app-shell";
import { SessionProvider, useSession } from "@/lib/session";
import { can, getRestaurant, listLocations, problemMessage, type Location, type Restaurant } from "@/lib/api";

export default function RestaurantDetailPage() {
  return (
    <SessionProvider>
      <AppShell>
        <RestaurantDetail />
      </AppShell>
    </SessionProvider>
  );
}

function RestaurantDetail() {
  const params = useParams<{ id: string }>();
  const { state, token } = useSession();
  const [restaurant, setRestaurant] = useState<Restaurant | null>(null);
  const [locations, setLocations] = useState<Location[]>([]);
  const [error, setError] = useState("");

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
      .catch((err) => {
        if (err instanceof Error && "status" in (err as { status?: number }) && (err as { status?: number }).status === 404) {
          notFound();
          return;
        }
        setError(problemMessage(err));
      });
  }, [state.kind, token, params.id]);

  if (state.kind !== "loaded") {
    return null;
  }
  const canWrite = can(state.data, "restaurants", "write");

  if (!restaurant) {
    return error ? <p className="rounded bg-red-50 p-3 text-sm text-red-700">{error}</p> : <p className="text-gray-500">Loading…</p>;
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">{restaurant.name}</h1>
          <p className="text-sm text-gray-500">{restaurant.legal_name}</p>
        </div>
        {canWrite && (
          <Link
            href={`/restaurants/${restaurant.restaurant_id}/edit`}
            className="rounded border border-gray-300 bg-white px-4 py-2 text-sm hover:bg-gray-50"
          >
            Edit
          </Link>
        )}
      </div>

      <section className="rounded-lg border border-gray-200 bg-white p-6">
        <dl className="grid grid-cols-1 gap-2 text-sm sm:grid-cols-2">
          <dt className="text-gray-500">Status</dt>
          <dd>{restaurant.status}</dd>
          <dt className="text-gray-500">Country</dt>
          <dd>{restaurant.country}</dd>
          <dt className="text-gray-500">Default currency</dt>
          <dd>{restaurant.default_currency}</dd>
          <dt className="text-gray-500">Phone</dt>
          <dd>{restaurant.phone || "—"}</dd>
          <dt className="text-gray-500">Email</dt>
          <dd>{restaurant.email || "—"}</dd>
          <dt className="text-gray-500">Restaurant ID</dt>
          <dd className="font-mono text-xs">{restaurant.restaurant_id}</dd>
        </dl>
      </section>

      <section className="rounded-lg border border-gray-200 bg-white p-6">
        <div className="flex items-center justify-between">
          <h2 className="text-lg font-semibold">Locations</h2>
          {canWrite && (
            <Link
              href={`/restaurants/${restaurant.restaurant_id}/locations`}
              className="rounded bg-emerald-600 px-3 py-1.5 text-sm text-white hover:bg-emerald-700"
            >
              Manage locations
            </Link>
          )}
        </div>
        {locations.length === 0 ? (
          <p className="mt-3 text-sm text-gray-600">No locations yet.</p>
        ) : (
          <ul className="mt-3 divide-y divide-gray-100">
            {locations.map((l) => (
              <li key={l.location_id} className="flex items-center justify-between py-2 text-sm">
                <div>
                  <span className="font-medium">{l.name}</span>
                  <span className="ml-2 text-gray-500">
                    {l.address}, {l.city} ({l.country})
                  </span>
                </div>
                <span className={`rounded-full px-2 py-0.5 text-xs ${l.status === "active" ? "bg-emerald-50 text-emerald-800" : "bg-gray-100 text-gray-600"}`}>
                  {l.status}
                </span>
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  );
}