import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "Tirek — food-service procurement platform",
  description:
    "Procurement, orders, payments and financing for restaurants and suppliers.",
};

export default function RootLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="en">
      <body className="bg-white text-gray-900 antialiased">{children}</body>
    </html>
  );
}