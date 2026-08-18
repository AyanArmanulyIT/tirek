"use client";

import { useEffect, useState } from "react";
import { useParams, useRouter } from "next/navigation";
import { AppShell } from "@/components/app-shell";
import { RestaurantForm } from "@/components/restaurant-form";
import { SessionProvider, useSession } from "@/lib/session";
import { getRestaurant, problemMessage, type Restaurant } from "@/lib/api";

export default function EditRestaurantPage() {
  return (
    <SessionProvider>
      <AppShell>
        <EditRestaurant />
      </AppShell>
    </SessionProvider>
  );
}

function EditRestaurant() {
  const params = useParams<{ id: string }>();
  const router = useRouter();
  const { state, token } = useSession();
  const [restaurant, setRestaurant] = useState<Restaurant | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    if (state.kind !== "loaded") {
      return;
    }
    getRestaurant(token, params.id)
      .then(setRestaurant)
      .catch((err) => setError(problemMessage(err)));
  }, [state.kind, token, params.id]);

  if (state.kind !== "loaded") {
    return null;
  }

  return (
    <div className="max-w-2xl space-y-6">
      <h1 className="text-2xl font-bold">Edit restaurant</h1>
      {error && <p className="rounded bg-red-50 p-3 text-sm text-red-700">{error}</p>}
      {restaurant && (
        <section className="rounded-lg border border-gray-200 bg-white p-6">
          <RestaurantForm
            initial={restaurant}
            onDone={(r) => router.push(`/restaurants/${r.restaurant_id}`)}
          />
        </section>
      )}
    </div>
  );
}