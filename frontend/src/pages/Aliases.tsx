import { createSignal, For, Show, onMount } from "solid-js";
import Card from "../components/Card";
import Badge from "../components/Badge";
import Button from "../components/Button";
import { EmptyStateCard, ErrorStateCard, LoadingStateCard } from "../components/PageState";
import { apiGet } from "../lib/api";

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
  });

  const gatorCount = () => aliases().filter((a) => a.is_gator).length;

  return (
    <div class="space-y-5">
      {/* Header */}
      <div class="flex items-center justify-between">
        <div>
          <h1 class="text-[var(--text-2xl)] font-semibold tracking-tight text-[var(--text-primary)]">
            Aliases
          </h1>
          <p class="mt-1 text-[var(--text-sm)] text-[var(--text-tertiary)]">
            Firewall aliases for IP ranges, port groups, and network lists.
            {gatorCount() > 0 && (
              <span class="ml-2 text-[var(--status-success)]">{gatorCount()} managed by Gator</span>
            )}
          </p>
        </div>
        <Button variant="secondary" size="md" onClick={() => void loadAliases()} loading={loading()}>
          <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M23 4v6h-6M1 20v-6h6M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15" />
          </svg>
          Refresh
        </Button>
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
            <svg class="mx-auto h-12 w-12 text-[var(--text-muted)]" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
              <path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71" />
              <path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71" />
            </svg>
          </EmptyStateCard>
        </Show>

        <Show when={aliases().length > 0}>
          <Card padding="none" class="overflow-hidden">
            <div class="overflow-x-auto">
              <table class="w-full">
                <thead>
                  <tr class="border-b border-[var(--border-strong)]">
                    <th class="px-4 py-3 text-left text-[var(--text-xs)] font-semibold uppercase tracking-wider text-[var(--text-tertiary)]">
                      Name
                    </th>
                    <th class="px-4 py-3 text-left text-[var(--text-xs)] font-semibold uppercase tracking-wider text-[var(--text-tertiary)]">
                      Type
                    </th>
                    <th class="px-4 py-3 text-left text-[var(--text-xs)] font-semibold uppercase tracking-wider text-[var(--text-tertiary)]">
                      Content
                    </th>
                    <th class="px-4 py-3 text-left text-[var(--text-xs)] font-semibold uppercase tracking-wider text-[var(--text-tertiary)]">
                      Description
                    </th>
                    <th class="px-4 py-3 text-left text-[var(--text-xs)] font-semibold uppercase tracking-wider text-[var(--text-tertiary)]">
                      Status
                    </th>
                  </tr>
                </thead>
                <tbody>
                  <For each={aliases()}>
                    {(alias) => (
                      <tr
                        class={[
                          "border-b border-[var(--border-subtle)] transition-colors duration-[var(--transition-fast)] hover:bg-[var(--bg-hover)]",
                          alias.is_gator ? "bg-[var(--success-subtle)]/30" : "",
                        ].join(" ")}
                      >
                        <td class="px-4 py-3">
                          <div class="flex items-center gap-2">
                            <span class="font-mono text-[var(--text-sm)] text-[var(--text-primary)]">{alias.name}</span>
                            <Show when={alias.is_gator}>
                              <Badge variant="success" size="sm">Gator</Badge>
                            </Show>
                          </div>
                        </td>
                        <td class="px-4 py-3">{getTypeBadge(alias.type)}</td>
                        <td
                          class="px-4 py-3 font-mono text-[var(--text-xs)] text-[var(--text-tertiary)] max-w-xs truncate"
                          title={alias.content}
                        >
                          {formatContent(alias.content)}
                        </td>
                        <td
                          class="px-4 py-3 text-[var(--text-xs)] text-[var(--text-tertiary)] max-w-xs truncate"
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
            </div>
          </Card>
        </Show>
      </Show>
    </div>
  );
}
