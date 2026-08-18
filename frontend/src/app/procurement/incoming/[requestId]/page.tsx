"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
import { AppShell } from "@/components/app-shell";
import { RequestDetail } from "@/components/request-detail";
import { SessionProvider, useSession } from "@/lib/session";
import { getIncoming } from "@/lib/api";

export default function ProcurementIncomingDetailPage() {
  return (
    <SessionProvider>
      <AppShell>
        <IncomingDetailPageInner />
      </AppShell>
    </SessionProvider>
  );
}

function IncomingDetailPageInner() {
  const params = useParams<{ requestId: string }>();
  const { state, token } = useSession();
  if (state.kind !== "loaded") {
    return null;
  }
  return (
    <div className="space-y-4">
      <Link href="/procurement/incoming" className="text-sm text-emerald-700 underline">
        ← All incoming requests
      </Link>
      <RequestDetail
        token={token}
        requestId={params.requestId}
        fetchFn={getIncoming}
        showBuyer
      />
    </div>
  );
}