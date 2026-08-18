"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { can, orgTypeLabel, type MeResponse } from "@/lib/api";
import { useSession } from "@/lib/session";

export function AppShell({ children }: { children: React.ReactNode }) {
  const { state, signOut } = useSession();
  const pathname = usePathname();

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

  return (
    <div className="min-h-screen bg-gray-50">
      <Header data={state.data} pathname={pathname} onSignOut={signOut} />
      <main className="mx-auto max-w-5xl px-6 py-8">{children}</main>
    </div>
  );
}

function Header({
  data,
  pathname,
  onSignOut,
}: {
  data: MeResponse;
  pathname: string;
  onSignOut: () => void;
}) {
  const { user, org } = data;
  const canWriteRestaurants = can(data, "restaurants", "write");
  const canWriteSuppliers = can(data, "suppliers", "write");
  const canManageCatalog = can(data, "catalog", "manage");

  function navLink(href: string, label: string) {
    const active = pathname === href || pathname.startsWith(`${href}/`);
    return (
      <Link
        href={href}
        className={`rounded px-3 py-1.5 text-sm ${
          active ? "bg-emerald-600 text-white" : "text-gray-700 hover:bg-gray-100"
        }`}
      >
        {label}
      </Link>
    );
  }

  return (
    <header className="border-b border-gray-200 bg-white">
      <div className="mx-auto flex max-w-5xl items-center justify-between px-6 py-3">
        <div className="flex items-center gap-6">
          <Link href="/dashboard" className="text-lg font-bold text-emerald-700">
            Tirek
          </Link>
          <nav className="flex items-center gap-1">
            {navLink("/dashboard", "Dashboard")}
            {org.type === "buyer" && navLink("/restaurants", "Restaurants")}
            {org.type === "supplier" && navLink("/products", "Products")}
            {org.type === "buyer" && navLink("/catalog", "Browse products")}
            {org.type === "buyer" && can(data, "procurement", "write") && navLink("/procurement/cart", "Cart")}
            {org.type === "buyer" && can(data, "procurement", "read") && navLink("/procurement/requests", "Requests")}
            {org.type === "supplier" && can(data, "procurement", "read") && navLink("/procurement/incoming", "Incoming")}
            {org.type === "supplier" && navLink("/supplier", "Supplier profile")}
          </nav>
        </div>
        <div className="flex items-center gap-3 text-sm">
          <span className="hidden text-gray-600 sm:inline">{user.email}</span>
          <span className="rounded-full bg-gray-100 px-2 py-0.5 text-xs text-gray-600">
            {orgTypeLabel(org.type)}
          </span>
          {org.type === "buyer" && canWriteRestaurants && (
            <Link
              href="/restaurants/new"
              className="rounded bg-emerald-600 px-3 py-1.5 text-sm text-white hover:bg-emerald-700"
            >
              New restaurant
            </Link>
          )}
          {org.type === "supplier" && canWriteSuppliers && (
            <Link
              href="/supplier?edit=1"
              className="rounded bg-emerald-600 px-3 py-1.5 text-sm text-white hover:bg-emerald-700"
            >
              Edit profile
            </Link>
          )}
          {org.type === "supplier" && canManageCatalog && (
            <Link
              href="/products/new"
              className="rounded bg-emerald-600 px-3 py-1.5 text-sm text-white hover:bg-emerald-700"
            >
              New product
            </Link>
          )}
          <button
            onClick={onSignOut}
            className="rounded border border-gray-300 px-3 py-1.5 text-sm hover:bg-gray-50"
          >
            Sign out
          </button>
        </div>
      </div>
    </header>
  );
}