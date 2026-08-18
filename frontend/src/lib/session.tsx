"use client";

import { createContext, useContext, useEffect, useState, type ReactNode } from "react";
import { useRouter } from "next/navigation";
import { logout, me, problemMessage, type MeResponse } from "@/lib/api";

export type SessionState =
  | { kind: "loading" }
  | { kind: "loaded"; data: MeResponse; token: string }
  | { kind: "error"; message: string };

interface SessionValue {
  state: SessionState;
  token: string;
  signOut: () => Promise<void>;
}

const SessionContext = createContext<SessionValue | null>(null);

export function SessionProvider({ children }: { children: ReactNode }) {
  const router = useRouter();
  const [state, setState] = useState<SessionState>({ kind: "loading" });

  useEffect(() => {
    const token = sessionStorage.getItem("tirek_access_token");
    if (!token) {
      router.replace("/login");
      return;
    }
    me(token)
      .then((data) => setState({ kind: "loaded", data, token }))
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

  return (
    <SessionContext.Provider
      value={{ state, token: state.kind === "loaded" ? state.token : "", signOut }}
    >
      {children}
    </SessionContext.Provider>
  );
}

export function useSession(): SessionValue {
  const ctx = useContext(SessionContext);
  if (!ctx) {
    throw new Error("useSession must be used within a SessionProvider");
  }
  return ctx;
}