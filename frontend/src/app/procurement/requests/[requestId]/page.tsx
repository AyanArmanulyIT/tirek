"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
import { AppShell } from "@/components/app-shell";
import { RequestDetail } from "@/components/request-detail";
import { SessionProvider, useSession } from "@/lib/session";
import { cancelRequest, getRequest } from "@/lib/api";

export default function ProcurementRequestDetailPage() {
  return (
    <SessionProvider>
      <AppShell>
        <RequestDetailPageInner />
      </AppShell>
    </SessionProvider>
  );
}

function RequestDetailPageInner() {
  const params = useParams<{ requestId: string }>();
  const { state, token } = useSession();
  if (state.kind !== "loaded") {
    return null;
  }
  return (
    <div className="space-y-4">
      <Link href="/procurement/requests" className="text-sm text-emerald-700 underline">
        ← All requests
      </Link>
      <RequestDetail
        token={token}
        requestId={params.requestId}
        fetchFn={getRequest}
        onCancel={(id) => cancelRequest(token, id)}
      />
    </div>
  );
}