"use client";

import { useRouter } from "next/navigation";
import { AppShell } from "@/components/app-shell";
import { RestaurantForm } from "@/components/restaurant-form";
import { SessionProvider } from "@/lib/session";

export default function NewRestaurantPage() {
  return (
    <SessionProvider>
      <AppShell>
        <NewRestaurant />
      </AppShell>
    </SessionProvider>
  );
}

function NewRestaurant() {
  const router = useRouter();
  return (
    <div className="max-w-2xl space-y-6">
      <h1 className="text-2xl font-bold">New restaurant</h1>
      <section className="rounded-lg border border-gray-200 bg-white p-6">
        <RestaurantForm onDone={(r) => router.push(`/restaurants/${r.restaurant_id}`)} />
      </section>
    </div>
  );
}