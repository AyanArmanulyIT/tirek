// BFF client for the Tirek API.
// The browser never holds the raw refresh token: it is kept in the HttpOnly
// cookie the API sets on login/refresh. The access token is short-lived and
// held in memory (see docs/architecture/security.md).

const API_URL =
  process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";

export class ApiError extends Error {
  constructor(
    public status: number,
    public problem?: unknown,
  ) {
    super(`API request failed with status ${status}`);
  }
}

export interface UserInfo {
  user_id: string;
  email: string;
  full_name: string;
  phone?: string;
  status: string;
}

export interface OrgInfo {
  org_id: string;
  name: string;
  type: string;
  country: string;
  default_currency: string;
  status: string;
}

export interface AuthResponse {
  access_token: string;
  token_type: string;
  expires_in: number;
  user: UserInfo;
  org: OrgInfo;
  role: string;
}

export interface MeResponse extends AuthResponse {
  permissions: string[];
}

async function request<T>(
  path: string,
  init: RequestInit = {},
  accessToken?: string,
): Promise<T> {
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    ...(init.headers as Record<string, string>),
  };
  if (accessToken) {
    headers.Authorization = `Bearer ${accessToken}`;
  }
  const res = await fetch(`${API_URL}${path}`, {
    ...init,
    headers,
    credentials: "include",
  });
  if (!res.ok) {
    throw new ApiError(res.status, await res.json().catch(() => null));
  }
  if (res.status === 204) {
    return undefined as T;
  }
  return res.json() as Promise<T>;
}

export function login(email: string, password: string) {
  return request<AuthResponse>("/api/v1/auth/login", {
    method: "POST",
    body: JSON.stringify({ email, password }),
  });
}

export function register(input: {
  email: string;
  password: string;
  full_name: string;
  org_name: string;
  org_type: string;
}) {
  return request<AuthResponse>("/api/v1/auth/register", {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export function logout() {
  return request<void>("/api/v1/auth/logout", { method: "POST" });
}

export function me(accessToken: string) {
  return request<MeResponse>("/api/v1/me", {}, accessToken);
}

export function orgTypeLabel(t: string): string {
  switch (t) {
    case "buyer":
      return "Restaurant / Buyer";
    case "supplier":
      return "Supplier";
    case "marketplace":
      return "Marketplace";
    default:
      return t;
  }
}

export function problemMessage(err: unknown): string {
  if (err instanceof ApiError) {
    const p = err.problem as { title?: string; detail?: string } | null;
    return p?.detail ?? p?.title ?? `Request failed (${err.status})`;
  }
  return err instanceof Error ? err.message : "Unexpected error";
}