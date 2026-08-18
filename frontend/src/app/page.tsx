import Link from "next/link";

export default function HomePage() {
  return (
    <main className="min-h-screen flex flex-col items-center justify-center gap-6 p-8">
      <h1 className="text-4xl font-bold">Tirek</h1>
      <p className="text-lg text-gray-600">
        Procurement, orders, payments and financing for restaurants and
        suppliers in Kazakhstan.
      </p>
      <div className="flex gap-4">
        <Link
          href="/login"
          className="rounded bg-emerald-600 px-4 py-2 text-white hover:bg-emerald-700"
        >
          Sign in
        </Link>
        <Link
          href="/register"
          className="rounded border border-gray-300 px-4 py-2 hover:bg-gray-50"
        >
          Create organization
        </Link>
      </div>
    </main>
  );
}