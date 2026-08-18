"use client";

import { FormEvent, Suspense, useEffect, useState } from "react";
import { useSearchParams } from "next/navigation";
import { AppShell } from "@/components/app-shell";
import { SessionProvider, useSession } from "@/lib/session";
import {
  can,
  createSupplier,
  listSuppliers,
  problemMessage,
  updateSupplier,
  type Supplier,
} from "@/lib/api";

export default function SupplierPage() {
  return (
    <SessionProvider>
      <AppShell>
        <Suspense fallback={<p className="text-gray-500">Loading…</p>}>
          <SupplierProfile />
        </Suspense>
      </AppShell>
    </SessionProvider>
  );
}

function SupplierProfile() {
  const searchParams = useSearchParams();
  const { state, token } = useSession();
  const [supplier, setSupplier] = useState<Supplier | null>(null);
  const [loaded, setLoaded] = useState(false);
  const [editing, setEditing] = useState(searchParams.get("edit") === "1");
  const [error, setError] = useState("");

  useEffect(() => {
    if (state.kind !== "loaded") {
      return;
    }
    listSuppliers(token)
      .then((res) => {
        setSupplier(res.data[0] ?? null);
        setLoaded(true);
      })
      .catch((err) => {
        setError(problemMessage(err));
        setLoaded(true);
      });
  }, [state.kind, token]);

  if (state.kind !== "loaded" || !loaded) {
    return <p className="text-gray-500">Loading…</p>;
  }
  const canWrite = can(state.data, "suppliers", "write");

  if (!supplier) {
    return (
      <div className="max-w-2xl space-y-6">
        <h1 className="text-2xl font-bold">Supplier profile</h1>
        <p className="text-sm text-gray-600">
          Your organization does not have a supplier profile yet. Create one to become visible
          to buyers on the platform.
        </p>
        {canWrite && (
          <section className="rounded-lg border border-gray-200 bg-white p-6">
            <SupplierForm
              onSubmit={async (payload) => {
                const res = await createSupplier(token, payload);
                setSupplier(res);
                setEditing(false);
              }}
            />
          </section>
        )}
        {!canWrite && (
          <p className="text-sm text-gray-600">Contact an administrator of your organization to create it.</p>
        )}
      </div>
    );
  }

  return (
    <div className="max-w-2xl space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">{supplier.name}</h1>
          <p className="text-sm text-gray-500">{supplier.legal_name}</p>
        </div>
        {canWrite && (
          <button
            onClick={() => setEditing((e) => !e)}
            className="rounded border border-gray-300 bg-white px-4 py-2 text-sm hover:bg-gray-50"
          >
            {editing ? "Cancel" : "Edit"}
          </button>
        )}
      </div>

      {error && <p className="rounded bg-red-50 p-3 text-sm text-red-700">{error}</p>}

      {editing && canWrite ? (
        <section className="rounded-lg border border-gray-200 bg-white p-6">
          <SupplierForm
            initial={supplier}
            onSubmit={async (payload) => {
              const res = await updateSupplier(token, supplier.supplier_id, payload);
              setSupplier(res);
              setEditing(false);
            }}
          />
        </section>
      ) : (
        <section className="rounded-lg border border-gray-200 bg-white p-6">
          <dl className="grid grid-cols-1 gap-2 text-sm sm:grid-cols-2">
            <dt className="text-gray-500">Status</dt>
            <dd>{supplier.status}</dd>
            <dt className="text-gray-500">Country</dt>
            <dd>{supplier.country}</dd>
            <dt className="text-gray-500">Default currency</dt>
            <dd>{supplier.default_currency}</dd>
            <dt className="text-gray-500">Phone</dt>
            <dd>{supplier.phone || "—"}</dd>
            <dt className="text-gray-500">Email</dt>
            <dd>{supplier.email || "—"}</dd>
            <dt className="text-gray-500">Rating</dt>
            <dd>{supplier.rating ?? "—"}</dd>
            <dt className="text-gray-500">Supplier ID</dt>
            <dd className="font-mono text-xs">{supplier.supplier_id}</dd>
          </dl>
        </section>
      )}
    </div>
  );
}

function SupplierForm({
  initial,
  onSubmit,
}: {
  initial?: Supplier;
  onSubmit: (payload: {
    name: string;
    legal_name?: string;
    status: string;
    country: string;
    default_currency: string;
    phone?: string;
    email?: string;
  }) => Promise<void>;
}) {
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

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setError("");
    setBusy(true);
    try {
      await onSubmit({
        name: form.name,
        legal_name: form.legal_name || undefined,
        status: form.status,
        country: form.country,
        default_currency: form.default_currency,
        phone: form.phone || undefined,
        email: form.email || undefined,
      });
    } catch (err) {
      setError(problemMessage(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      {error && <p className="rounded bg-red-50 p-3 text-sm text-red-700">{error}</p>}
      <div className="space-y-1">
        <label className="text-sm font-medium">Name *</label>
        <input
          required
          maxLength={200}
          value={form.name}
          onChange={(e) => set("name", e.target.value)}
          className="w-full rounded border border-gray-300 px-3 py-2"
        />
      </div>
      <div className="space-y-1">
        <label className="text-sm font-medium">Legal name</label>
        <input
          maxLength={200}
          value={form.legal_name}
          onChange={(e) => set("legal_name", e.target.value)}
          className="w-full rounded border border-gray-300 px-3 py-2"
        />
      </div>
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
        <div className="space-y-1">
          <label className="text-sm font-medium">Status</label>
          <select
            value={form.status}
            onChange={(e) => set("status", e.target.value)}
            className="w-full rounded border border-gray-300 px-3 py-2"
          >
            <option value="active">active</option>
            <option value="suspended">suspended</option>
            <option value="closed">closed</option>
          </select>
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
          <label className="text-sm font-medium">Currency (ISO)</label>
          <input
            maxLength={3}
            value={form.default_currency}
            onChange={(e) => set("default_currency", e.target.value.toUpperCase())}
            className="w-full rounded border border-gray-300 px-3 py-2"
          />
        </div>
      </div>
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <div className="space-y-1">
          <label className="text-sm font-medium">Phone</label>
          <input
            maxLength={32}
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
        {busy ? "Saving…" : initial ? "Save changes" : "Create profile"}
      </button>
    </form>
  );
}