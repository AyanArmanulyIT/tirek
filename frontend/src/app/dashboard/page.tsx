"use client";

import Link from "next/link";
import { AppShell } from "@/components/app-shell";
import { SessionProvider, useSession } from "@/lib/session";
import { orgTypeLabel } from "@/lib/api";

export default function DashboardPage() {
  return (
    <SessionProvider>
      <AppShell>
        <Dashboard />
      </AppShell>
    </SessionProvider>
  );
}

function Dashboard() {
  const { state } = useSession();
  if (state.kind !== "loaded") {
    return null;
  }
  const { user, org, role, permissions } = state.data;
  const isBuyer = org.type === "buyer";

  return (
    <div className="space-y-6">
      <section className="rounded-lg border border-gray-200 bg-white p-6">
        <h1 className="text-2xl font-bold">Welcome, {user.full_name}</h1>
        <p className="mt-1 text-sm text-gray-600">
          {org.name} · {orgTypeLabel(org.type)} · {role}
        </p>
        <div className="mt-4 flex flex-wrap gap-2">
          {isBuyer ? (
            <Link
              href="/restaurants"
              className="rounded bg-emerald-600 px-4 py-2 text-sm text-white hover:bg-emerald-700"
            >
              Manage restaurants
            </Link>
          ) : (
            <Link
              href="/supplier"
              className="rounded bg-emerald-600 px-4 py-2 text-sm text-white hover:bg-emerald-700"
            >
              Manage supplier profile
            </Link>
          )}
        </div>
      </section>

      <section className="rounded-lg border border-gray-200 bg-white p-6">
        <h2 className="text-lg font-semibold">Your organization</h2>
        <dl className="mt-3 grid grid-cols-1 gap-2 text-sm sm:grid-cols-2">
          <dt className="text-gray-500">Country</dt>
          <dd>{org.country}</dd>
          <dt className="text-gray-500">Default currency</dt>
          <dd>{org.default_currency}</dd>
          <dt className="text-gray-500">Org ID</dt>
          <dd className="font-mono text-xs">{org.org_id}</dd>
        </dl>
      </section>

      <section className="rounded-lg border border-gray-200 bg-white p-6">
        <h2 className="text-lg font-semibold">Permissions</h2>
        <div className="mt-3 flex flex-wrap gap-2">
          {permissions.map((p) => (
            <span
              key={p}
              className="rounded-full bg-emerald-50 px-3 py-1 text-xs text-emerald-800"
            >
              {p}
            </span>
          ))}
        </div>
      </section>
    </div>
  );
}