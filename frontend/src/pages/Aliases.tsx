import { createSignal, For, Show, onMount } from "solid-js";
import Card from "../components/Card";
import Badge from "../components/Badge";
import DropdownMenu from "../components/DropdownMenu";
import type { MenuEntry } from "../components/DropdownMenu";
import { EmptyStateCard, ErrorStateCard, LoadingStateCard } from "../components/PageState";
import { apiGet, getOpnsenseHost } from "../lib/api";

type Alias = {
  uuid: string;
  enabled: string;
  name: string;
  type: string;
  content: string;
  description: string;
  is_gator: boolean;
};

async function fetchAliases(): Promise<Alias[]> {
  const { ok, data } = await apiGet<{ aliases?: Alias[]; error?: string }>("/api/opnsense/aliases");
  if (!ok) throw new Error(data.error ?? "Failed to load aliases");
  return data.aliases ?? [];
}

function getTypeBadge(type: string) {
  switch (type.toLowerCase()) {
    case "network":
      return <Badge variant="info" size="sm">{type}</Badge>;
    case "port":
      return <Badge variant="default" size="sm">{type}</Badge>;
    case "host":
      return <Badge variant="success" size="sm">{type}</Badge>;
    case "urltable":
      return <Badge variant="warning" size="sm">{type}</Badge>;
    case "geoip":
      return <Badge variant="success" size="sm">{type}</Badge>;
    case "asn":
      return <Badge variant="error" size="sm">{type}</Badge>;
    default:
      return <Badge variant="muted" size="sm">{type || "-"}</Badge>;
  }
}

function formatContent(content: string): string {
  if (!content) return "-";
  const items = content.split(/[\n,]+/).filter((s) => s.trim());
  if (items.length <= 3) return items.join(", ");
  return `${items.slice(0, 3).join(", ")} (+${items.length - 3} more)`;
}

export default function Aliases() {
  const [aliases, setAliases] = createSignal<Alias[]>([]);
  const [loading, setLoading] = createSignal(true);
  const [loadError, setLoadError] = createSignal("");
  const [opnHost, setOpnHost] = createSignal("");

  const loadAliases = async () => {
    setLoading(true);
    setLoadError("");
    try {
      const data = await fetchAliases();
      setAliases(data);
    } catch (err) {
      setLoadError(err instanceof Error ? err.message : "Failed to load aliases");
    } finally {
      setLoading(false);
    }
  };

  onMount(() => {
    void loadAliases();
    void getOpnsenseHost().then(setOpnHost);
  });

  const gatorCount = () => aliases().filter((a) => a.is_gator).length;

  return (
    <div class="space-y-5">
      {/* Header */}
      <div class="flex items-start justify-between gap-4">
        <div>
          <h1 class="text-title-h2 font-semibold tracking-tight text-fg">
            Aliases
          </h1>
          <p class="mt-1 text-body-sm text-fg-muted">
            Firewall aliases for IP ranges, port groups, and network lists.
            <Show when={gatorCount() > 0}>
              <span class="ml-2 text-success">{gatorCount()} managed by Gator</span>
            </Show>
          </p>
        </div>
        <DropdownMenu items={(() => {
          const items: MenuEntry[] = [
            { label: "Refresh", onClick: () => void loadAliases(), loading: loading() },
          ];
          if (opnHost()) {
            items.push({ divider: true as const });
            items.push({ label: "Open in OPNsense", href: `${opnHost()}/ui/firewall/alias`, external: true });
          }
          return items;
        })()} />
      </div>

      {/* Loading state */}
      <Show when={loading()}>
        <LoadingStateCard message="Loading aliases..." />
      </Show>

      {/* Error state */}
      <Show when={loadError() !== ""}>
        <ErrorStateCard message={loadError()} />
      </Show>

      {/* Table */}
      <Show when={!loading() && loadError() === ""}>
        <Show when={aliases().length === 0}>
          <EmptyStateCard message="No aliases found">
            <svg class="mx-auto h-12 w-12 text-fg-muted" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
              <path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71" />
              <path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71" />
            </svg>
          </EmptyStateCard>
        </Show>

        <Show when={aliases().length > 0}>
          <Card padding="none" class="overflow-hidden">
              <table class="w-full table-fixed">
                <colgroup>
                  <col class="w-[28%]" />
                  <col class="w-[10%]" />
                  <col class="w-[25%]" />
                  <col class="w-[27%]" />
                  <col class="w-[10%]" />
                </colgroup>
                <thead>
                  <tr class="border-b border-border-faint">
                    <th class="px-4 py-3 text-left text-label-xs font-semibold uppercase tracking-wider text-fg-muted">
                      Name
                    </th>
                    <th class="px-4 py-3 text-left text-label-xs font-semibold uppercase tracking-wider text-fg-muted">
                      Type
                    </th>
                    <th class="px-4 py-3 text-left text-label-xs font-semibold uppercase tracking-wider text-fg-muted">
                      Content
                    </th>
                    <th class="px-4 py-3 text-left text-label-xs font-semibold uppercase tracking-wider text-fg-muted">
                      Description
                    </th>
                    <th class="px-4 py-3 text-left text-label-xs font-semibold uppercase tracking-wider text-fg-muted">
                      Status
                    </th>
                  </tr>
                </thead>
                <tbody>
                  <For each={aliases()}>
                    {(alias) => (
                      <tr
                        class={[
                          "border-b border-border-faint transition-colors duration-fast hover:bg-hover",
                          alias.is_gator ? "bg-success-subtle/30" : "",
                        ].join(" ")}
                      >
                        <td class="px-4 py-3 overflow-hidden">
                          <div class="flex items-center gap-2 min-w-0">
                            <span class="text-mono-md text-fg truncate">{alias.name}</span>
                            <Show when={alias.is_gator}>
                              <Badge variant="success" size="sm" class="shrink-0">Gator</Badge>
                            </Show>
                          </div>
                        </td>
                        <td class="px-4 py-3">{getTypeBadge(alias.type)}</td>
                        <td
                          class="px-4 py-3 text-mono-sm text-fg-muted truncate overflow-hidden"
                          title={alias.content}
                        >
                          {formatContent(alias.content)}
                        </td>
                        <td
                          class="px-4 py-3 text-body-xs text-fg-muted truncate overflow-hidden"
                          title={alias.description}
                        >
                          {alias.description || "-"}
                        </td>
                        <td class="px-4 py-3">
                          {alias.enabled === "1" ? (
                            <Badge variant="success" size="sm">enabled</Badge>
                          ) : (
                            <Badge variant="muted" size="sm">disabled</Badge>
                          )}
                        </td>
                      </tr>
                    )}
                  </For>
                </tbody>
              </table>
          </Card>
        </Show>
      </Show>
    </div>
  );
}
