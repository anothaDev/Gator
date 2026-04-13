import { createSignal, For, Show, onMount } from "solid-js";
import Card from "../components/Card";
import Badge from "../components/Badge";
import DropdownMenu from "../components/DropdownMenu";
import type { MenuEntry } from "../components/DropdownMenu";
import { EmptyStateCard, ErrorStateCard, LoadingStateCard } from "../components/PageState";
import { apiGet, getOpnsenseHost } from "../lib/api";

type OPNInterface = {
  identifier: string;
  device: string;
  description: string;
  status: string;
  enabled: string;
  type: string;
  is_wireguard: boolean;
  assigned: boolean;
  addresses: string[];
  macaddr: string;
};

async function fetchInterfaces(): Promise<OPNInterface[]> {
  const { ok, data } = await apiGet<{ interfaces?: OPNInterface[]; error?: string }>("/api/opnsense/interfaces");
  if (!ok) throw new Error(data.error ?? "Failed to load interfaces");
  return data.interfaces ?? [];
}

function getStatusBadge(iface: OPNInterface) {
  const hasAddrs = iface.addresses.length > 0;
  const isEnabled = iface.enabled === "1" || hasAddrs;

  if (!isEnabled) {
    return <Badge variant="muted" size="sm">disabled</Badge>;
  }

  const lower = (iface.status || "").toLowerCase();
  if (lower.includes("up") || lower.includes("associated") || lower.includes("active")) {
    return (
      <Badge variant="success" size="sm">
        <span class="h-1.5 w-1.5 rounded-full bg-current" />
        {iface.status}
      </Badge>
    );
  }
  if (lower.includes("down") || lower.includes("no carrier")) {
    return (
      <Badge variant="error" size="sm">
        <span class="h-1.5 w-1.5 rounded-full bg-current" />
        {iface.status}
      </Badge>
    );
  }

  if (hasAddrs) {
    return (
      <Badge variant="success" size="sm">
        <span class="h-1.5 w-1.5 rounded-full bg-current" />
        up
      </Badge>
    );
  }

  return <Badge variant="info" size="sm">enabled</Badge>;
}

export default function Interfaces() {
  const [interfaces, setInterfaces] = createSignal<OPNInterface[]>([]);
  const [loading, setLoading] = createSignal(true);
  const [loadError, setLoadError] = createSignal("");
  const [opnHost, setOpnHost] = createSignal("");

  const loadInterfaces = async () => {
    setLoading(true);
    setLoadError("");
    try {
      const data = await fetchInterfaces();
      // Sort: WireGuard first, then assigned, then alphabetical.
      data.sort((a, b) => {
        if (a.is_wireguard !== b.is_wireguard) return a.is_wireguard ? -1 : 1;
        if (a.assigned !== b.assigned) return a.assigned ? -1 : 1;
        return a.identifier.localeCompare(b.identifier);
      });
      setInterfaces(data);
    } catch (err) {
      setLoadError(err instanceof Error ? err.message : "Failed to load interfaces");
    } finally {
      setLoading(false);
    }
  };

  onMount(() => {
    void loadInterfaces();
    void getOpnsenseHost().then(setOpnHost);
  });

  const wgInterfaces = () => interfaces().filter((i) => i.is_wireguard);
  const hasUnassignedWG = () => wgInterfaces().some((i) => !i.assigned);

  return (
    <div class="space-y-5">
      {/* Header */}
      <div class="flex items-start justify-between gap-4">
        <div>
          <h1 class="text-title-h2 font-semibold tracking-tight text-fg">
            Interfaces
          </h1>
          <p class="mt-1 text-body-sm text-fg-muted">
            Network interfaces. WireGuard devices must be assigned before routing.
          </p>
        </div>
        <DropdownMenu items={(() => {
          const items: MenuEntry[] = [
            { label: "Refresh", onClick: () => void loadInterfaces(), loading: loading() },
          ];
          if (opnHost()) {
            items.push({ divider: true as const });
            items.push({ label: "Open in OPNsense", href: `${opnHost()}/ui/interfaces/overview`, external: true });
          }
          return items;
        })()} />
      </div>

      {/* Warning banner */}
      <Show when={hasUnassignedWG()}>
        <Card variant="elevated" class="border-l-4 border-l-warning">
          <div class="flex items-start gap-4">
            <div class="flex h-10 w-10 items-center justify-center rounded-lg bg-warning-subtle">
              <svg class="h-5 w-5 text-warning" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z" />
                <line x1="12" y1="9" x2="12" y2="13" />
                <line x1="12" y1="17" x2="12.01" y2="17" />
              </svg>
            </div>
            <div>
              <h3 class="font-medium text-fg">Unassigned WireGuard device detected</h3>
              <p class="mt-1 text-body-sm text-fg-secondary">
                To use a WireGuard tunnel for routing, it must be assigned as an OPNsense interface.
                Go to <span class="font-semibold">Interfaces &gt; Assignments</span> in OPNsense,
                select the WG device from the "New interface" dropdown, click <span class="font-semibold">+</span> to add it,
                then enable it. After that, click Refresh here.
              </p>
            </div>
          </div>
        </Card>
      </Show>

      {/* Loading state */}
      <Show when={loading()}>
        <LoadingStateCard message="Loading interfaces..." />
      </Show>

      {/* Error state */}
      <Show when={loadError() !== ""}>
        <ErrorStateCard message={loadError()} />
      </Show>

      {/* Table */}
      <Show when={!loading() && loadError() === ""}>
        <Show when={interfaces().length === 0}>
          <EmptyStateCard message="No interfaces found">
            <svg class="mx-auto h-12 w-12 text-fg-muted" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
              <rect x="2" y="3" width="20" height="14" rx="2" ry="2" />
              <line x1="8" y1="21" x2="16" y2="21" />
              <line x1="12" y1="17" x2="12" y2="21" />
            </svg>
          </EmptyStateCard>
        </Show>

        <Show when={interfaces().length > 0}>
          <Card padding="none" class="overflow-hidden">
            <div class="overflow-x-auto">
              <table class="w-full">
                <thead>
                  <tr class="border-b border-border-faint">
                    <th class="px-4 py-3 text-left text-label-xs font-semibold uppercase tracking-wider text-fg-muted">
                      Identifier
                    </th>
                    <th class="px-4 py-3 text-left text-label-xs font-semibold uppercase tracking-wider text-fg-muted">
                      Device
                    </th>
                    <th class="px-4 py-3 text-left text-label-xs font-semibold uppercase tracking-wider text-fg-muted">
                      Description
                    </th>
                    <th class="px-4 py-3 text-left text-label-xs font-semibold uppercase tracking-wider text-fg-muted">
                      Address
                    </th>
                    <th class="px-4 py-3 text-left text-label-xs font-semibold uppercase tracking-wider text-fg-muted">
                      Status
                    </th>
                    <th class="px-4 py-3 text-left text-label-xs font-semibold uppercase tracking-wider text-fg-muted">
                      Assigned
                    </th>
                  </tr>
                </thead>
                <tbody>
                  <For each={interfaces()}>
                    {(iface) => (
                      <tr
                        class={[
                          "border-b border-border-faint transition-colors duration-fast hover:bg-hover",
                          iface.is_wireguard ? "bg-info-subtle/30" : "",
                        ].join(" ")}
                      >
                        <td class="px-4 py-3">
                          <div class="flex items-center gap-2">
                            <span class="font-medium text-fg">{iface.identifier}</span>
                            <Show when={iface.is_wireguard}>
                              <Badge variant="info" size="sm">WG</Badge>
                            </Show>
                          </div>
                        </td>
                        <td class="px-4 py-3 text-mono-md text-fg-secondary">
                          {iface.device}
                        </td>
                        <td class="px-4 py-3 text-body-sm text-fg-secondary">
                          {iface.description || "-"}
                        </td>
                        <td class="px-4 py-3 text-mono-sm text-fg-muted">
                          <Show when={iface.addresses.length > 0} fallback="-">
                            <For each={iface.addresses}>
                              {(addr) => <div>{addr}</div>}
                            </For>
                          </Show>
                        </td>
                        <td class="px-4 py-3">{getStatusBadge(iface)}</td>
                        <td class="px-4 py-3">
                          <Show
                            when={iface.assigned}
                            fallback={<span class="text-body-xs text-warning">not assigned</span>}
                          >
                            <span class="text-body-xs text-success">yes</span>
                          </Show>
                        </td>
                      </tr>
                    )}
                  </For>
                </tbody>
              </table>
            </div>
          </Card>
        </Show>
      </Show>
    </div>
  );
}
