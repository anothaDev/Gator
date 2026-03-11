import { createSignal, For, Show, onMount } from "solid-js";
import Card from "../components/Card";
import Badge from "../components/Badge";
import Button from "../components/Button";
import { EmptyStateCard, ErrorStateCard, LoadingStateCard } from "../components/PageState";
import { apiGet } from "../lib/api";

type NATRule = {
  uuid: string;
  enabled: string;
  interface: string;
  source_net: string;
  destination: string;
  protocol: string;
  target: string;
  description: string;
  is_gator: boolean;
};

async function fetchNATRules(): Promise<NATRule[]> {
  const { ok, data } = await apiGet<{ rules?: NATRule[]; error?: string }>("/api/opnsense/nat-rules");
  if (!ok) throw new Error(data.error ?? "Failed to load NAT rules");
  return data.rules ?? [];
}

export default function Nat() {
  const [rules, setRules] = createSignal<NATRule[]>([]);
  const [loading, setLoading] = createSignal(true);
  const [loadError, setLoadError] = createSignal("");

  const loadRules = async () => {
    setLoading(true);
    setLoadError("");
    try {
      const data = await fetchNATRules();
      setRules(data);
    } catch (err) {
      setLoadError(err instanceof Error ? err.message : "Failed to load NAT rules");
    } finally {
      setLoading(false);
    }
  };

  onMount(() => {
    void loadRules();
  });

  const gatorCount = () => rules().filter((r) => r.is_gator).length;

  return (
    <div class="space-y-5">
      {/* Header */}
      <div class="flex items-center justify-between">
        <div>
          <h1 class="text-[var(--text-2xl)] font-semibold tracking-tight text-[var(--text-primary)]">
            NAT
          </h1>
          <p class="mt-1 text-[var(--text-sm)] text-[var(--text-tertiary)]">
            Outbound NAT rules.
            {gatorCount() > 0 && (
              <span class="ml-2 text-[var(--status-success)]">{gatorCount()} managed by Gator</span>
            )}
          </p>
        </div>
        <Button variant="secondary" size="md" onClick={() => void loadRules()} loading={loading()}>
          <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M23 4v6h-6M1 20v-6h6M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15" />
          </svg>
          Refresh
        </Button>
      </div>

      {/* Loading state */}
      <Show when={loading()}>
        <LoadingStateCard message="Loading NAT rules..." />
      </Show>

      {/* Error state */}
      <Show when={loadError() !== ""}>
        <ErrorStateCard message={loadError()} />
      </Show>

      {/* Table */}
      <Show when={!loading() && loadError() === ""}>
        <Show when={rules().length === 0}>
          <EmptyStateCard message="No outbound NAT rules found. If OPNsense is in automatic or hybrid NAT mode, rules are generated dynamically.">
            <svg class="mx-auto h-12 w-12 text-[var(--text-muted)]" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
              <path d="M4 4h16c1.1 0 2 .9 2 2v12c0 1.1-.9 2-2 2H4c-1.1 0-2-.9-2-2V6c0-1.1.9-2 2-2z" />
              <polyline points="22,6 12,13 2,6" />
            </svg>
          </EmptyStateCard>
        </Show>

        <Show when={rules().length > 0}>
          <Card padding="none" class="overflow-hidden">
            <div class="overflow-x-auto">
              <table class="w-full">
                <thead>
                  <tr class="border-b border-[var(--border-strong)]">
                    <th class="px-4 py-3 text-left text-[var(--text-xs)] font-semibold uppercase tracking-wider text-[var(--text-tertiary)]">
                      Description
                    </th>
                    <th class="px-4 py-3 text-left text-[var(--text-xs)] font-semibold uppercase tracking-wider text-[var(--text-tertiary)]">
                      Interface
                    </th>
                    <th class="px-4 py-3 text-left text-[var(--text-xs)] font-semibold uppercase tracking-wider text-[var(--text-tertiary)]">
                      Source
                    </th>
                    <th class="px-4 py-3 text-left text-[var(--text-xs)] font-semibold uppercase tracking-wider text-[var(--text-tertiary)]">
                      Destination
                    </th>
                    <th class="px-4 py-3 text-left text-[var(--text-xs)] font-semibold uppercase tracking-wider text-[var(--text-tertiary)]">
                      Protocol
                    </th>
                    <th class="px-4 py-3 text-left text-[var(--text-xs)] font-semibold uppercase tracking-wider text-[var(--text-tertiary)]">
                      Target
                    </th>
                    <th class="px-4 py-3 text-left text-[var(--text-xs)] font-semibold uppercase tracking-wider text-[var(--text-tertiary)]">
                      Status
                    </th>
                  </tr>
                </thead>
                <tbody>
                  <For each={rules()}>
                    {(rule) => (
                      <tr
                        class={[
                          "border-b border-[var(--border-subtle)] transition-colors duration-[var(--transition-fast)] hover:bg-[var(--bg-hover)]",
                          rule.is_gator ? "bg-[var(--success-subtle)]/30" : "",
                        ].join(" ")}
                      >
                        <td class="px-4 py-3">
                          <div class="flex items-center gap-2">
                            <span class="font-medium text-[var(--text-sm)] text-[var(--text-primary)]">
                              {rule.description || "(no description)"}
                            </span>
                            <Show when={rule.is_gator}>
                              <Badge variant="success" size="sm">Gator</Badge>
                            </Show>
                          </div>
                        </td>
                        <td class="px-4 py-3 text-[var(--text-sm)] text-[var(--text-secondary)]">{rule.interface || "-"}</td>
                        <td class="px-4 py-3 font-mono text-[var(--text-xs)] text-[var(--text-tertiary)]">{rule.source_net || "any"}</td>
                        <td class="px-4 py-3 font-mono text-[var(--text-xs)] text-[var(--text-tertiary)]">{rule.destination || "any"}</td>
                        <td class="px-4 py-3 text-[var(--text-sm)] text-[var(--text-tertiary)]">{rule.protocol || "any"}</td>
                        <td class="px-4 py-3 font-mono text-[var(--text-xs)] text-[var(--text-tertiary)]">{rule.target || "-"}</td>
                        <td class="px-4 py-3">
                          {rule.enabled === "1" ? (
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
