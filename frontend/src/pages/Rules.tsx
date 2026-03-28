import { createSignal, For, Show, onMount } from "solid-js";
import Card from "../components/Card";
import Badge from "../components/Badge";
import Button from "../components/Button";
import IconButton from "../components/IconButton";
import OpnsenseLink from "../components/OpnsenseLink";
import LegacyRulesWarning from "../components/LegacyRulesWarning";
import { EmptyStateCard, ErrorStateCard, LoadingStateCard } from "../components/PageState";
import { apiDelete, apiGet } from "../lib/api";
import { createLegacyCheck } from "../lib/legacy";
import Spinner from "../components/Spinner";

type FilterRule = {
  uuid: string;
  enabled: string;
  action: string;
  quick: string;
  interface: string;
  direction: string;
  ipprotocol: string;
  protocol: string;
  source_net: string;
  source_not: string;
  destination_net: string;
  destination_not: string;
  destination_port: string;
  gateway: string;
  description: string;
  is_gator: boolean;
};

async function fetchFilterRules(): Promise<FilterRule[]> {
  const { ok, data } = await apiGet<{ rules?: FilterRule[]; error?: string }>("/api/opnsense/rules");
  if (!ok) throw new Error(data.error ?? "Failed to load filter rules");
  return data.rules ?? [];
}

function getActionBadge(action: string) {
  switch (action.toLowerCase()) {
    case "pass":
      return (
        <Badge variant="success" size="sm">
          <span class="h-1.5 w-1.5 rounded-full bg-current" />
          {action}
        </Badge>
      );
    case "block":
      return (
        <Badge variant="error" size="sm">
          <span class="h-1.5 w-1.5 rounded-full bg-current" />
          {action}
        </Badge>
      );
    case "reject":
      return (
        <Badge variant="warning" size="sm">
          <span class="h-1.5 w-1.5 rounded-full bg-current" />
          {action}
        </Badge>
      );
    default:
      return <Badge variant="muted" size="sm">{action || "-"}</Badge>;
  }
}

export default function Rules(props: { onNavigate?: (section: string) => void }) {
  const [rules, setRules] = createSignal<FilterRule[]>([]);
  const [loading, setLoading] = createSignal(true);
  const [loadError, setLoadError] = createSignal("");
  const [deleting, setDeleting] = createSignal<string | null>(null);
  const [deleteErr, setDeleteErr] = createSignal("");
  const { legacyCount, legacyChecked, checkLegacyRules } = createLegacyCheck();

  const loadRules = async () => {
    setLoading(true);
    setLoadError("");
    try {
      const data = await fetchFilterRules();
      setRules(data);
    } catch (err) {
      setLoadError(err instanceof Error ? err.message : "Failed to load filter rules");
    } finally {
      setLoading(false);
    }
  };

  const deleteRule = async (uuid: string, description: string) => {
    if (!confirm(`Delete rule "${description}"?\n\nThis will remove it from OPNsense and apply firewall changes.`)) return;

    setDeleting(uuid);
    setDeleteErr("");
    try {
      const { ok, data } = await apiDelete<{ error?: string }>(`/api/opnsense/rules/${uuid}`);
      if (!ok) {
        setDeleteErr(data.error ?? "Failed to delete rule");
        return;
      }
      await loadRules();
    } catch {
      setDeleteErr("Failed to delete rule. Check backend connectivity.");
    } finally {
      setDeleting(null);
    }
  };

  onMount(() => {
    void loadRules();
    void checkLegacyRules();
  });

  const gatorCount = () => rules().filter((r) => r.is_gator).length;

  return (
    <div class="space-y-5">
      {/* Header */}
      <div class="flex items-center justify-between">
        <div>
          <h1 class="text-2xl font-semibold tracking-tight text-fg">
            Rules
          </h1>
          <p class="mt-1 text-sm text-fg-tertiary">
            Firewall filter rules.
            {gatorCount() > 0 && (
              <span class="ml-2 text-success">{gatorCount()} managed by Gator</span>
            )}
          </p>
        </div>
        <Button variant="secondary" size="md" onClick={() => void loadRules()} loading={loading()}>
          <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M23 4v6h-6M1 20v-6h6M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15" />
          </svg>
          Refresh
        </Button>
        <OpnsenseLink path="/ui/firewall/filter" label="Rules" />
      </div>

      {/* Error banner */}
      <Show when={deleteErr()}>
        <Card variant="elevated" class="border-l-4 border-l-error">
          <div class="flex items-center gap-3 text-error">
            <svg class="h-5 w-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <circle cx="12" cy="12" r="10" />
              <line x1="12" y1="8" x2="12" y2="12" />
              <line x1="12" y1="16" x2="12.01" y2="16" />
            </svg>
            <span class="text-sm">{deleteErr()}</span>
          </div>
        </Card>
      </Show>

      {/* Legacy rules migration banner */}
      <Show when={legacyChecked() && legacyCount() > 0}>
        <LegacyRulesWarning
          count={legacyCount()}
          detail="These rules are not visible here. Use the Migration assistant to convert them."
          onNavigate={props.onNavigate ? () => props.onNavigate!("migration") : undefined}
        />
      </Show>

      {/* Loading state */}
      <Show when={loading()}>
        <LoadingStateCard message="Loading filter rules..." />
      </Show>

      {/* Error state */}
      <Show when={loadError() !== ""}>
        <ErrorStateCard message={loadError()} />
      </Show>

      {/* Table */}
      <Show when={!loading() && loadError() === ""}>
        <Show when={rules().length === 0}>
          <EmptyStateCard message="No filter rules found">
            <svg class="mx-auto h-12 w-12 text-fg-muted" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
              <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" />
              <polyline points="14,2 14,8 20,8" />
              <line x1="16" y1="13" x2="8" y2="13" />
              <line x1="16" y1="17" x2="8" y2="17" />
              <polyline points="10,9 9,9 8,9" />
            </svg>
          </EmptyStateCard>
        </Show>

        <Show when={rules().length > 0}>
          <Card padding="none" class="overflow-hidden">
              <table class="w-full table-fixed">
                <colgroup>
                  <col class="w-[24%]" />
                  <col class="w-[8%]" />
                  <col class="w-[8%]" />
                  <col class="w-[10%]" />
                  <col class="w-[18%]" />
                  <col class="w-[10%]" />
                  <col class="w-[12%]" />
                  <col class="w-[6%]" />
                  <col class="w-[4%]" />
                </colgroup>
                <thead>
                  <tr class="border-b border-line-strong">
                    <th class="px-3 py-3 text-left text-xs font-semibold uppercase tracking-wider text-fg-tertiary">
                      Description
                    </th>
                    <th class="px-3 py-3 text-left text-xs font-semibold uppercase tracking-wider text-fg-tertiary">
                      Action
                    </th>
                    <th class="px-3 py-3 text-left text-xs font-semibold uppercase tracking-wider text-fg-tertiary">
                      Iface
                    </th>
                    <th class="px-3 py-3 text-left text-xs font-semibold uppercase tracking-wider text-fg-tertiary">
                      Source
                    </th>
                    <th class="px-3 py-3 text-left text-xs font-semibold uppercase tracking-wider text-fg-tertiary">
                      Destination
                    </th>
                    <th class="px-3 py-3 text-left text-xs font-semibold uppercase tracking-wider text-fg-tertiary">
                      Proto
                    </th>
                    <th class="px-3 py-3 text-left text-xs font-semibold uppercase tracking-wider text-fg-tertiary">
                      Gateway
                    </th>
                    <th class="px-3 py-3 text-left text-xs font-semibold uppercase tracking-wider text-fg-tertiary">
                      Status
                    </th>
                    <th class="px-3 py-3"></th>
                  </tr>
                </thead>
                <tbody>
                  <For each={rules()}>
                    {(rule) => {
                      const src = () => {
                        const s = rule.source_net || "any";
                        return rule.source_not === "1" ? `!${s}` : s;
                      };
                      const dest = () => {
                        let d = rule.destination_net || "any";
                        if (rule.destination_not === "1") d = `!${d}`;
                        if (rule.destination_port) d += `:${rule.destination_port}`;
                        return d;
                      };

                      return (
                        <tr
                          class={[
                            "border-b border-line-faint transition-colors duration-fast hover:bg-hover",
                            rule.is_gator ? "bg-success-subtle/30" : "",
                          ].join(" ")}
                        >
                          <td class="px-3 py-3 overflow-hidden">
                            <div class="flex items-center gap-1.5 min-w-0">
                              <span class="font-medium text-sm text-fg truncate" title={rule.description}>
                                {rule.description || "(no description)"}
                              </span>
                              <Show when={rule.is_gator}>
                                <Badge variant="success" size="sm" class="shrink-0">Gator</Badge>
                              </Show>
                            </div>
                          </td>
                          <td class="px-3 py-3">{getActionBadge(rule.action)}</td>
                          <td class="px-3 py-3 text-sm text-fg-secondary truncate overflow-hidden">{rule.interface || "-"}</td>
                          <td class="px-3 py-3 font-mono text-xs text-fg-tertiary truncate overflow-hidden" title={src()}>{src()}</td>
                          <td class="px-3 py-3 font-mono text-xs text-fg-tertiary truncate overflow-hidden" title={dest()}>{dest()}</td>
                          <td class="px-3 py-3 text-xs text-fg-tertiary truncate overflow-hidden">
                            {rule.protocol || "any"}
                            <Show when={rule.direction}>
                              <span class="ml-0.5 text-fg-muted">({rule.direction})</span>
                            </Show>
                          </td>
                          <td class="px-3 py-3 overflow-hidden">
                            <Show
                              when={rule.gateway}
                              fallback={<span class="text-xs text-fg-muted">default</span>}
                            >
                              <span class="font-mono text-xs text-accent truncate block" title={rule.gateway}>{rule.gateway}</span>
                            </Show>
                          </td>
                          <td class="px-3 py-3">
                            {rule.enabled === "1" ? (
                              <span class="h-2 w-2 rounded-full bg-success inline-block" title="enabled" />
                            ) : (
                              <span class="h-2 w-2 rounded-full bg-fg-muted inline-block" title="disabled" />
                            )}
                          </td>
                          <td class="px-3 py-3 text-right">
                            <Show when={rule.is_gator}>
                              <IconButton
                                variant="ghost"
                                size="sm"
                                title={`Delete ${rule.description}`}
                                onClick={() => void deleteRule(rule.uuid, rule.description)}
                                disabled={deleting() === rule.uuid}
                              >
                                <Show
                                  when={deleting() === rule.uuid}
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
                            </Show>
                          </td>
                        </tr>
                      );
                    }}
                  </For>
                </tbody>
              </table>
          </Card>
        </Show>
      </Show>
    </div>
  );
}
