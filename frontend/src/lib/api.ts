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

export interface Restaurant {
  restaurant_id: string;
  org_id: string;
  name: string;
  legal_name?: string;
  status: "active" | "suspended" | "closed";
  country: string;
  default_currency: string;
  phone?: string;
  email?: string;
  created_at: string;
  updated_at: string;
}

export interface RestaurantInput {
  name: string;
  legal_name?: string;
  status: string;
  country: string;
  default_currency: string;
  phone?: string;
  email?: string;
}

export interface Location {
  location_id: string;
  restaurant_id: string;
  org_id: string;
  name: string;
  address: string;
  city: string;
  country: string;
  status: "active" | "inactive";
  phone?: string;
  email?: string;
  created_at: string;
  updated_at: string;
}

export interface LocationInput {
  name: string;
  address: string;
  city: string;
  country: string;
  status: string;
  phone?: string;
  email?: string;
}

export interface Supplier {
  supplier_id: string;
  org_id: string;
  name: string;
  legal_name?: string;
  status: "active" | "suspended" | "closed";
  country: string;
  default_currency: string;
  phone?: string;
  email?: string;
  rating?: number;
  created_at: string;
  updated_at: string;
}

export interface SupplierInput {
  name: string;
  legal_name?: string;
  status: string;
  country: string;
  default_currency: string;
  phone?: string;
  email?: string;
}

export interface ListResponse<T> {
  data: T[];
  limit: number;
  offset: number;
  total: number;
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

export function listRestaurants(accessToken: string) {
  return request<ListResponse<Restaurant>>("/api/v1/restaurants", {}, accessToken);
}

export function getRestaurant(accessToken: string, id: string) {
  return request<Restaurant>(`/api/v1/restaurants/${id}`, {}, accessToken);
}

export function createRestaurant(accessToken: string, input: RestaurantInput) {
  return request<Restaurant>("/api/v1/restaurants", {
    method: "POST",
    body: JSON.stringify(input),
  }, accessToken);
}

export function updateRestaurant(accessToken: string, id: string, input: RestaurantInput) {
  return request<Restaurant>(`/api/v1/restaurants/${id}`, {
    method: "PATCH",
    body: JSON.stringify(input),
  }, accessToken);
}

export function listLocations(accessToken: string, restaurantId: string) {
  return request<ListResponse<Location>>(
    `/api/v1/restaurants/${restaurantId}/locations`,
    {},
    accessToken,
  );
}

export function createLocation(accessToken: string, restaurantId: string, input: LocationInput) {
  return request<Location>(`/api/v1/restaurants/${restaurantId}/locations`, {
    method: "POST",
    body: JSON.stringify(input),
  }, accessToken);
}

export function updateLocation(
  accessToken: string,
  restaurantId: string,
  locationId: string,
  input: LocationInput,
) {
  return request<Location>(`/api/v1/restaurants/${restaurantId}/locations/${locationId}`, {
    method: "PATCH",
    body: JSON.stringify(input),
  }, accessToken);
}

export function listSuppliers(accessToken: string) {
  return request<ListResponse<Supplier>>("/api/v1/suppliers", {}, accessToken);
}

export function getSupplier(accessToken: string, id: string) {
  return request<Supplier>(`/api/v1/suppliers/${id}`, {}, accessToken);
}

export function createSupplier(accessToken: string, input: SupplierInput) {
  return request<Supplier>("/api/v1/suppliers", {
    method: "POST",
    body: JSON.stringify(input),
  }, accessToken);
}

export function updateSupplier(accessToken: string, id: string, input: SupplierInput) {
  return request<Supplier>(`/api/v1/suppliers/${id}`, {
    method: "PATCH",
    body: JSON.stringify(input),
  }, accessToken);
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

// hasPermission mirrors the backend wildcard: the owner role (and any role
// holding org.owner) implicitly holds every permission. /me only returns the
// role's explicit permission list, so owner must be treated specially here.
export function hasPermission(me: Pick<MeResponse, "role" | "permissions">, perm: string): boolean {
  if (me.role === "owner" || me.permissions.includes("org.owner")) {
    return true;
  }
  return me.permissions.includes(perm);
}

export function can(
  me: Pick<MeResponse, "role" | "permissions">,
  prefix: "restaurants" | "suppliers",
  level: "read" | "write" | "manage",
): boolean {
  if (hasPermission(me, `${prefix}.manage`)) {
    return true;
  }
  if (level === "read") {
    return hasPermission(me, `${prefix}.read`);
  }
  return hasPermission(me, `${prefix}.write`);
}

export function problemMessage(err: unknown): string {
  if (err instanceof ApiError) {
    const p = err.problem as { title?: string; detail?: string } | null;
    return p?.detail ?? p?.title ?? `Request failed (${err.status})`;
  }
  return err instanceof Error ? err.message : "Unexpected error";
}