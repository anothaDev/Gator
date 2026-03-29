import { createSignal, For, Show, onMount } from "solid-js";
import Card from "../components/Card";
import Badge from "../components/Badge";
import IconButton from "../components/IconButton";
import DropdownMenu from "../components/DropdownMenu";
import type { MenuEntry } from "../components/DropdownMenu";
import { EmptyStateCard, ErrorStateCard, LoadingStateCard } from "../components/PageState";
import { apiDelete, apiGet, getOpnsenseHost } from "../lib/api";
import Spinner from "../components/Spinner";

type Gateway = {
  uuid: string;
  name: string;
  interface: string;
  gateway: string;
  ipprotocol: string;
  disabled: string;
  defaultgw: string;
  descr: string;
  status: string;
};

async function fetchGateways(): Promise<Gateway[]> {
  const { ok, data } = await apiGet<{ gateways?: Gateway[]; error?: string }>("/api/opnsense/gateways");
  if (!ok) throw new Error(data.error ?? "Failed to load gateways");
  return data.gateways ?? [];
}

function getStatusBadge(status: string) {
  const lower = status.toLowerCase();
  if (lower.includes("online") || lower.includes("active") || lower === "up") {
    return (
      <Badge variant="success" size="sm">
        <span class="h-1.5 w-1.5 rounded-full bg-current" />
        {status}
      </Badge>
    );
  }
  if (lower.includes("offline") || lower.includes("down")) {
    return (
      <Badge variant="error" size="sm">
        <span class="h-1.5 w-1.5 rounded-full bg-current" />
        {status}
      </Badge>
    );
  }
  return <Badge variant="muted" size="sm">{status}</Badge>;
}

export default function Gateways() {
  const [gateways, setGateways] = createSignal<Gateway[]>([]);
  const [loading, setLoading] = createSignal(true);
  const [loadError, setLoadError] = createSignal("");
  const [deleting, setDeleting] = createSignal<string | null>(null);
  const [opnHost, setOpnHost] = createSignal("");
  const [actionMsg, setActionMsg] = createSignal("");
  const [actionErr, setActionErr] = createSignal("");

  const loadGateways = async () => {
    setLoading(true);
    setLoadError("");
    try {
      const data = await fetchGateways();
      setGateways(data);
    } catch (err) {
      setLoadError(err instanceof Error ? err.message : "Failed to load gateways");
    } finally {
      setLoading(false);
    }
  };

  onMount(() => {
    void loadGateways();
    void getOpnsenseHost().then(setOpnHost);
  });

  const deleteGateway = async (uuid: string, name: string) => {
    setActionMsg("");
    setActionErr("");
    setDeleting(uuid);

    try {
      const { ok, data } = await apiDelete<{ error?: string }>(`/api/opnsense/gateways/${uuid}`);
      if (!ok) {
        setActionErr(data.error ?? "Failed to delete gateway.");
        return;
      }

      setActionMsg(`Gateway "${name}" deleted.`);
      void loadGateways();
    } catch {
      setActionErr("Failed to delete gateway. Check backend connectivity.");
    } finally {
      setDeleting(null);
    }
  };

  return (
    <div class="space-y-5">
      {/* Header */}
      <div class="flex items-start justify-between gap-4">
        <div>
          <h1 class="text-title-h2 font-semibold tracking-tight text-fg">
            Gateways
          </h1>
          <p class="mt-1 text-body-sm text-fg-muted">
            Network gateways. VPN routing creates gateways automatically.
          </p>
        </div>
        <DropdownMenu items={(() => {
          const items: MenuEntry[] = [
            { label: "Refresh", onClick: () => void loadGateways(), loading: loading() },
          ];
          if (opnHost()) {
            items.push({ divider: true as const });
            items.push({ label: "Open in OPNsense", href: `${opnHost()}/ui/routes/gateway`, external: true });
          }
          return items;
        })()} />
      </div>

      {/* Action messages */}
      <Show when={actionMsg() !== ""}>
        <Card variant="elevated" class="border-l-4 border-l-success">
          <div class="flex items-center gap-3 text-success">
            <svg class="h-5 w-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M22 11.08V12a10 10 0 1 1-5.93-9.14" />
              <polyline points="22 4 12 14.01 9 11.01" />
            </svg>
            <span class="text-sm">{actionMsg()}</span>
          </div>
        </Card>
      </Show>

      <Show when={actionErr() !== ""}>
        <Card variant="elevated" class="border-l-4 border-l-error">
          <div class="flex items-center gap-3 text-error">
            <svg class="h-5 w-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <circle cx="12" cy="12" r="10" />
              <line x1="12" y1="8" x2="12" y2="12" />
              <line x1="12" y1="16" x2="12.01" y2="16" />
            </svg>
            <span class="text-sm">{actionErr()}</span>
          </div>
        </Card>
      </Show>

      {/* Loading state */}
      <Show when={loading()}>
        <LoadingStateCard message="Loading gateways..." />
      </Show>

      {/* Error state */}
      <Show when={loadError() !== ""}>
        <ErrorStateCard message={loadError()} />
      </Show>

      {/* Table */}
      <Show when={!loading() && loadError() === ""}>
        <Show when={gateways().length === 0}>
          <EmptyStateCard message="No gateways found">
            <svg class="mx-auto h-12 w-12 text-fg-muted" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
              <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z" />
              <circle cx="12" cy="12" r="3" />
            </svg>
          </EmptyStateCard>
        </Show>

        <Show when={gateways().length > 0}>
          <Card padding="none" class="overflow-hidden">
            <div class="overflow-x-auto">
              <table class="w-full">
                <thead>
                  <tr class="border-b border-border-faint">
                    <th class="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wider text-fg-muted">
                      Name
                    </th>
                    <th class="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wider text-fg-muted">
                      Interface
                    </th>
                    <th class="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wider text-fg-muted">
                      Gateway
                    </th>
                    <th class="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wider text-fg-muted">
                      Protocol
                    </th>
                    <th class="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wider text-fg-muted">
                      Status
                    </th>
                    <th class="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wider text-fg-muted">
                      Default
                    </th>
                    <th class="px-4 py-3 text-right text-xs font-semibold uppercase tracking-wider text-fg-muted">
                      Actions
                    </th>
                  </tr>
                </thead>
                <tbody>
                  <For each={gateways()}>
                    {(gw) => (
                      <tr class="border-b border-border-faint transition-colors duration-fast hover:bg-hover">
                        <td class="px-4 py-3">
                          <div>
                            <span class="font-medium text-fg">{gw.name}</span>
                            <Show when={gw.descr}>
                              <p class="text-xs text-fg-muted">{gw.descr}</p>
                            </Show>
                          </div>
                        </td>
                        <td class="px-4 py-3 text-sm text-fg-secondary">{gw.interface}</td>
                        <td class="px-4 py-3 font-mono text-sm text-fg-secondary">{gw.gateway || "-"}</td>
                        <td class="px-4 py-3 text-sm text-fg-muted">
                          {gw.ipprotocol === "inet" ? "IPv4" : gw.ipprotocol === "inet6" ? "IPv6" : gw.ipprotocol}
                        </td>
                        <td class="px-4 py-3">{getStatusBadge(gw.status)}</td>
                        <td class="px-4 py-3 text-sm text-fg-muted">
                          {gw.defaultgw === "1" ? (
                            <span class="text-success">Yes</span>
                          ) : (
                            "-"
                          )}
                        </td>
                        <td class="px-4 py-3 text-right">
                          <IconButton
                            variant="ghost"
                            size="sm"
                            title={`Delete ${gw.name}`}
                            onClick={() => void deleteGateway(gw.uuid, gw.name)}
                            disabled={deleting() === gw.uuid}
                          >
                            <Show
                              when={deleting() === gw.uuid}
                              fallback={
                                <svg class="h-4 w-4 text-error" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                                  <polyline points="3 6 5 6 21 6" />
                                  <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" />
                                </svg>
                              }
                            >
                              <Spinner />
                            </Show>
                          </IconButton>
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
