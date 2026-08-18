"use client";

import { FormEvent, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { register, orgTypeLabel, problemMessage } from "@/lib/api";

const ORG_TYPES = ["buyer", "supplier", "marketplace"] as const;

export default function RegisterPage() {
  const router = useRouter();
  const [form, setForm] = useState({
    email: "",
    password: "",
    full_name: "",
    org_name: "",
    org_type: "buyer" as (typeof ORG_TYPES)[number],
  });
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  function set<K extends keyof typeof form>(key: K, value: (typeof form)[K]) {
    setForm((f) => ({ ...f, [key]: value }));
  }

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError("");
    setBusy(true);
    try {
      const res = await register(form);
      sessionStorage.setItem("tirek_access_token", res.access_token);
      router.push("/dashboard");
    } catch (err) {
      setError(problemMessage(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <main className="min-h-screen flex items-center justify-center p-8">
      <form
        onSubmit={onSubmit}
        className="w-full max-w-sm space-y-4 rounded-lg border border-gray-200 p-8 shadow-sm"
      >
        <h1 className="text-2xl font-bold">Create an organization</h1>
        {error && (
          <p className="rounded bg-red-50 p-3 text-sm text-red-700">{error}</p>
        )}
        <div className="space-y-1">
          <label htmlFor="full_name" className="text-sm font-medium">
            Full name
          </label>
          <input
            id="full_name"
            required
            value={form.full_name}
            onChange={(e) => set("full_name", e.target.value)}
            className="w-full rounded border border-gray-300 px-3 py-2"
          />
        </div>
        <div className="space-y-1">
          <label htmlFor="org_name" className="text-sm font-medium">
            Organization name
          </label>
          <input
            id="org_name"
            required
            value={form.org_name}
            onChange={(e) => set("org_name", e.target.value)}
            className="w-full rounded border border-gray-300 px-3 py-2"
          />
        </div>
        <div className="space-y-1">
          <label htmlFor="org_type" className="text-sm font-medium">
            Organization type
          </label>
          <select
            id="org_type"
            value={form.org_type}
            onChange={(e) => set("org_type", e.target.value as typeof form.org_type)}
            className="w-full rounded border border-gray-300 px-3 py-2"
          >
            {ORG_TYPES.map((t) => (
              <option key={t} value={t}>
                {orgTypeLabel(t)}
              </option>
            ))}
          </select>
        </div>
        <div className="space-y-1">
          <label htmlFor="email" className="text-sm font-medium">
            Email
          </label>
          <input
            id="email"
            type="email"
            required
            value={form.email}
            onChange={(e) => set("email", e.target.value)}
            className="w-full rounded border border-gray-300 px-3 py-2"
          />
        </div>
        <div className="space-y-1">
          <label htmlFor="password" className="text-sm font-medium">
            Password (min 12 chars)
          </label>
          <input
            id="password"
            type="password"
            required
            minLength={12}
            value={form.password}
            onChange={(e) => set("password", e.target.value)}
            className="w-full rounded border border-gray-300 px-3 py-2"
          />
        </div>
        <button
          type="submit"
          disabled={busy}
          className="w-full rounded bg-emerald-600 px-4 py-2 text-white hover:bg-emerald-700 disabled:opacity-50"
        >
          {busy ? "Creating…" : "Create organization"}
        </button>
        <p className="text-sm text-gray-600">
          Already registered?{" "}
          <Link href="/login" className="text-emerald-700 underline">
            Sign in
          </Link>
        </p>
      </form>
    </main>
  );
}