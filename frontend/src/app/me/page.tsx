"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { logout, me, orgTypeLabel, problemMessage, type MeResponse } from "@/lib/api";

export default function MePage() {
  const router = useRouter();
  const [state, setState] = useState<
    | { kind: "loading" }
    | { kind: "loaded"; data: MeResponse }
    | { kind: "error"; message: string }
  >({ kind: "loading" });

  useEffect(() => {
    const token = sessionStorage.getItem("tirek_access_token");
    if (!token) {
      router.replace("/login");
      return;
    }
    me(token)
      .then((data) => setState({ kind: "loaded", data }))
      .catch((err) => setState({ kind: "error", message: problemMessage(err) }));
  }, [router]);

  async function signOut() {
    try {
      await logout();
    } finally {
      sessionStorage.removeItem("tirek_access_token");
      router.push("/");
    }
  }

  if (state.kind === "loading") {
    return (
      <main className="min-h-screen flex items-center justify-center">
        <p className="text-gray-500">Loading…</p>
      </main>
    );
  }

  if (state.kind === "error") {
    return (
      <main className="min-h-screen flex flex-col items-center justify-center gap-4 p-8">
        <p className="rounded bg-red-50 p-3 text-sm text-red-700">{state.message}</p>
        <Link href="/login" className="text-emerald-700 underline">
          Sign in again
        </Link>
      </main>
    );
  }

  const { user, org, role, permissions } = state.data;

  return (
    <main className="min-h-screen p-8">
      <div className="mx-auto max-w-2xl space-y-6">
        <div className="flex items-center justify-between">
          <h1 className="text-2xl font-bold">Welcome, {user.full_name}</h1>
          <button
            onClick={signOut}
            className="rounded border border-gray-300 px-4 py-2 text-sm hover:bg-gray-50"
          >
            Sign out
          </button>
        </div>

        <section className="rounded-lg border border-gray-200 p-6">
          <h2 className="text-lg font-semibold">Your profile</h2>
          <dl className="mt-3 grid grid-cols-1 gap-2 text-sm sm:grid-cols-2">
            <dt className="text-gray-500">Email</dt>
            <dd>{user.email}</dd>
            <dt className="text-gray-500">User ID</dt>
            <dd className="font-mono text-xs">{user.user_id}</dd>
            <dt className="text-gray-500">Status</dt>
            <dd>{user.status}</dd>
          </dl>
        </section>

        <section className="rounded-lg border border-gray-200 p-6">
          <h2 className="text-lg font-semibold">Your organization</h2>
          <dl className="mt-3 grid grid-cols-1 gap-2 text-sm sm:grid-cols-2">
            <dt className="text-gray-500">Name</dt>
            <dd>{org.name}</dd>
            <dt className="text-gray-500">Type</dt>
            <dd>{orgTypeLabel(org.type)}</dd>
            <dt className="text-gray-500">Country</dt>
            <dd>{org.country}</dd>
            <dt className="text-gray-500">Default currency</dt>
            <dd>{org.default_currency}</dd>
            <dt className="text-gray-500">Org ID</dt>
            <dd className="font-mono text-xs">{org.org_id}</dd>
            <dt className="text-gray-500">Your role</dt>
            <dd>{role}</dd>
          </dl>
        </section>

        <section className="rounded-lg border border-gray-200 p-6">
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
    </main>
  );
}