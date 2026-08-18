"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { AppShell } from "@/components/app-shell";
import { SessionProvider, useSession } from "@/lib/session";
import { can, listRestaurants, problemMessage, type Restaurant } from "@/lib/api";

export default function RestaurantsPage() {
  return (
    <SessionProvider>
      <AppShell>
        <RestaurantsList />
      </AppShell>
    </SessionProvider>
  );
}

function RestaurantsList() {
  const { state, token } = useSession();
  const [items, setItems] = useState<Restaurant[]>([]);
  const [error, setError] = useState("");

  useEffect(() => {
    if (state.kind !== "loaded") {
      return;
    }
    listRestaurants(token)
      .then((res) => setItems(res.data))
      .catch((err) => setError(problemMessage(err)));
  }, [state.kind, token]);

  if (state.kind !== "loaded") {
    return null;
  }
  const canWrite = can(state.data, "restaurants", "write");

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">Restaurants</h1>
        {canWrite && (
          <Link
            href="/restaurants/new"
            className="rounded bg-emerald-600 px-4 py-2 text-sm text-white hover:bg-emerald-700"
          >
            New restaurant
          </Link>
        )}
      </div>

      {error && <p className="rounded bg-red-50 p-3 text-sm text-red-700">{error}</p>}

      {items.length === 0 ? (
        <section className="rounded-lg border border-gray-200 bg-white p-6">
          <p className="text-sm text-gray-600">No restaurants yet.</p>
          {canWrite && (
            <Link href="/restaurants/new" className="mt-2 inline-block text-sm text-emerald-700 underline">
              Create your first restaurant
            </Link>
          )}
        </section>
      ) : (
        <div className="overflow-hidden rounded-lg border border-gray-200 bg-white">
          <table className="w-full text-left text-sm">
            <thead className="bg-gray-50 text-xs uppercase text-gray-500">
              <tr>
                <th className="px-4 py-3">Name</th>
                <th className="px-4 py-3">Status</th>
                <th className="px-4 py-3">Country</th>
                <th className="px-4 py-3">Currency</th>
                <th className="px-4 py-3" />
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100">
              {items.map((r) => (
                <tr key={r.restaurant_id} className="hover:bg-gray-50">
                  <td className="px-4 py-3">
                    <Link href={`/restaurants/${r.restaurant_id}`} className="font-medium text-emerald-700 hover:underline">
                      {r.name}
                    </Link>
                    {r.legal_name && (
                      <div className="text-xs text-gray-500">{r.legal_name}</div>
                    )}
                  </td>
                  <td className="px-4 py-3">
                    <span className={`rounded-full px-2 py-0.5 text-xs ${r.status === "active" ? "bg-emerald-50 text-emerald-800" : "bg-gray-100 text-gray-600"}`}>
                      {r.status}
                    </span>
                  </td>
                  <td className="px-4 py-3">{r.country}</td>
                  <td className="px-4 py-3">{r.default_currency}</td>
                  <td className="px-4 py-3 text-right">
                    <Link href={`/restaurants/${r.restaurant_id}`} className="text-sm text-emerald-700 hover:underline">
                      View
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