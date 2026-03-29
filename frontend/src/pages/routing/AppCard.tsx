import { Show, For } from "solid-js";
import Badge from "../../components/Badge";
import type { AppProfile } from "./types";

// ─── App Card ────────────────────────────────────────────────────

function AppCard(props: {
  app: AppProfile;
  enabled: boolean;
  applied: boolean;
  busy: boolean;
  toggleDisabled: boolean;
  onToggle: () => void;
  onDelete: () => void;
}) {
  const protocolColors: Record<string, string> = {
    tcp: "text-info",
    udp: "text-warning",
    both: "text-brand",
  };

  return (
    <div
      class={`group relative flex items-center gap-3 rounded-lg border px-4 py-3.5 transition-all ${
        props.enabled
          ? "border-success/20 bg-success-subtle"
          : "border-border-faint bg-surface hover:border-border hover:bg-hover"
      }`}
    >
      <Show when={props.enabled}>
        <div class="absolute left-0 top-0 bottom-0 w-0.5 bg-success rounded-l-lg" />
      </Show>

      <div class="min-w-0 flex-1">
        <div class="flex items-center gap-2">
          <span class="text-body-md font-medium text-fg truncate">
            {props.app.name}
          </span>
          <Show when={props.app.is_custom}>
            <Badge variant="info" size="sm">custom</Badge>
          </Show>
          <Show when={props.enabled && props.applied}>
            <Badge variant="success" size="sm">routed</Badge>
          </Show>
        </div>

        <div class="mt-1.5 flex flex-wrap items-center gap-1.5">
          <For each={props.app.rules}>
            {(rule) => (
              <span class="inline-flex items-center gap-1 rounded bg-hover px-1.5 py-0.5 text-label-xs font-mono">
                <span class={protocolColors[rule.protocol] || "text-fg-muted"}>
                  {rule.protocol.toUpperCase()}
                </span>
                <span class="text-fg-secondary">{rule.ports}</span>
              </span>
            )}
          </For>
          <Show when={props.app.asns && props.app.asns.length > 0}>
            <span class="inline-flex items-center gap-1 rounded bg-brand/10 px-1.5 py-0.5 text-label-xs text-brand">
              {props.app.asns!.length} ASN{props.app.asns!.length > 1 ? "s" : ""}
            </span>
          </Show>
          <Show when={props.app.url_table_hint}>
            <span class="inline-flex items-center gap-1 rounded bg-info/10 px-1.5 py-0.5 text-label-xs text-info">
              IP ranges
            </span>
          </Show>
        </div>
      </div>

      <Show when={props.app.is_custom}>
        <button
          type="button"
          onClick={props.onDelete}
          class="shrink-0 rounded p-1.5 text-fg-muted opacity-0 transition-all hover:bg-error/10 hover:text-error group-hover:opacity-100"
          title="Delete custom profile"
        >
          <svg class="h-4 w-4" viewBox="0 0 20 20" fill="currentColor">
            <path fill-rule="evenodd" d="M8.75 1A2.75 2.75 0 006 3.75v.443c-.795.077-1.584.176-2.365.298a.75.75 0 10.23 1.482l.149-.022.841 10.518A2.75 2.75 0 007.596 19h4.807a2.75 2.75 0 002.742-2.53l.841-10.52.149.023a.75.75 0 00.23-1.482A41.03 41.03 0 0014 4.193V3.75A2.75 2.75 0 0011.25 1h-2.5zM10 4c.84 0 1.673.025 2.5.075V3.75c0-.69-.56-1.25-1.25-1.25h-2.5c-.69 0-1.25.56-1.25 1.25v.325C8.327 4.025 9.16 4 10 4zM8.58 7.72a.75.75 0 00-1.5.06l.3 7.5a.75.75 0 101.5-.06l-.3-7.5zm4.34.06a.75.75 0 10-1.5-.06l-.3 7.5a.75.75 0 101.5.06l.3-7.5z" clip-rule="evenodd"/>
          </svg>
        </button>
      </Show>

      <button
        type="button"
        aria-label={props.enabled ? `Disable ${props.app.name}` : `Enable ${props.app.name}`}
        onClick={props.onToggle}
        disabled={props.toggleDisabled}
        class={`relative inline-flex h-6 w-11 shrink-0 items-center rounded-full transition-all duration-200 disabled:opacity-50 ${
          props.enabled 
            ? "bg-success shadow-[0_0_10px_rgba(34,197,94,0.3)]" 
            : "bg-active"
        }`}
      >
        <Show when={props.busy}>
          <span class="absolute inset-0 flex items-center justify-center">
            <span class="h-3.5 w-3.5 animate-spin rounded-full border-2 border-white/60 border-t-transparent" />
          </span>
        </Show>
        <Show when={!props.busy}>
          <span
            class={`inline-block h-4 w-4 transform rounded-full bg-white shadow-md transition-transform duration-200 ${
              props.enabled ? "translate-x-6" : "translate-x-1"
            }`}
          />
        </Show>
      </button>
    </div>
  );
}

export default AppCard;
