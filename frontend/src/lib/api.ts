// Shared API helpers used across all pages.
// Superset of the helpers previously duplicated in VpnSetup, Tunnels, and Routing.

function handleAuthFailure(url: string, status: number) {
  if (status !== 401) {
    return;
  }
  if (url.startsWith("/api/auth/")) {
    return;
  }
  if (typeof window === "undefined") {
    return;
  }
  if (window.location.pathname === "/login") {
    return;
  }
  window.location.assign("/login");
}

export async function apiGet<T = Record<string, unknown>>(url: string): Promise<{ ok: boolean; data: T }> {
  const res = await fetch(url);
  handleAuthFailure(url, res.status);
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
  handleAuthFailure(url, res.status);
  const data = await res.json().catch(() => ({ error: `Non-JSON response (${res.status})` }) as T);
  return { ok: res.ok, data };
}

export async function apiPut<T = Record<string, unknown>>(url: string, body: unknown): Promise<{ ok: boolean; data: T }> {
  const res = await fetch(url, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  handleAuthFailure(url, res.status);
  const data = await res.json().catch(() => ({ error: `Non-JSON response (${res.status})` }) as T);
  return { ok: res.ok, data };
}

export async function apiDelete<T = Record<string, unknown>>(url: string): Promise<{ ok: boolean; data: T }> {
  const res = await fetch(url, { method: "DELETE" });
  handleAuthFailure(url, res.status);
  const data = await res.json().catch(() => ({ error: `Non-JSON response (${res.status})` }) as T);
  return { ok: res.ok, data };
}

// Cached OPNsense host URL for external links.
let _opnsenseHost: string | null = null;

export async function getOpnsenseHost(): Promise<string> {
  if (_opnsenseHost !== null) return _opnsenseHost;
  try {
    const { ok, data } = await apiGet<{ instances?: Array<{ host: string; active: boolean }> }>("/api/instances");
    if (ok) {
      const active = data.instances?.find((i) => i.active);
      if (active) {
        _opnsenseHost = active.host.replace(/\/$/, "");
        return _opnsenseHost;
      }
    }
  } catch {}
  _opnsenseHost = "";
  return "";
}

export function clearOpnsenseHostCache() {
  _opnsenseHost = null;
}
