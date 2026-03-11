// Shared API helpers used across all pages.
// Superset of the helpers previously duplicated in VpnSetup, Tunnels, and Routing.

export async function apiGet<T = Record<string, unknown>>(url: string): Promise<{ ok: boolean; data: T }> {
  const res = await fetch(url);
  const data = await res.json().catch(() => ({ error: `Non-JSON response (${res.status})` }) as T);
  return { ok: res.ok, data };
}

export async function apiPost<T = Record<string, unknown>>(url: string, body?: unknown): Promise<{ ok: boolean; data: T }> {
  const opts: RequestInit = { method: "POST" };
  if (body !== undefined) {
    opts.headers = { "Content-Type": "application/json" };
    opts.body = JSON.stringify(body);
  }
  const res = await fetch(url, opts);
  const data = await res.json().catch(() => ({ error: `Non-JSON response (${res.status})` }) as T);
  return { ok: res.ok, data };
}

export async function apiPut<T = Record<string, unknown>>(url: string, body: unknown): Promise<{ ok: boolean; data: T }> {
  const res = await fetch(url, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  const data = await res.json().catch(() => ({ error: `Non-JSON response (${res.status})` }) as T);
  return { ok: res.ok, data };
}

export async function apiDelete<T = Record<string, unknown>>(url: string): Promise<{ ok: boolean; data: T }> {
  const res = await fetch(url, { method: "DELETE" });
  const data = await res.json().catch(() => ({ error: `Non-JSON response (${res.status})` }) as T);
  return { ok: res.ok, data };
}
