import { For, Show } from "solid-js";
import Button from "../../components/Button";

// ─── Types ───────────────────────────────────────────────────────

type AppPreset = {
  id: string;
  name: string;
  description: string;
  vpn_on?: string[];
  vpn_off?: string[];
};

type CategoryInfo = {
  key: string;
  label: string;
  enabledCount: number;
};

// ─── Routing Toolbar ─────────────────────────────────────────────

function RoutingToolbar(props: {
  presets: AppPreset[];
  categories: CategoryInfo[];
  search: string;
  filterCategory: string | null;
  filterStatus: "all" | "enabled" | "disabled";
  enabledCount: number;
  totalCount: number;
  isToggling: boolean;
  onApplyPreset: (preset: AppPreset) => void;
  onSearchChange: (value: string) => void;
  onFilterCategoryChange: (cat: string | null) => void;
  onFilterStatusChange: (status: "all" | "enabled" | "disabled") => void;
  onAddCustom: () => void;
}) {
  return (
    <div class="rounded-xl border border-[var(--border-default)] bg-[var(--bg-tertiary)] p-4">
      {/* Presets row */}
      <div class="flex flex-wrap items-center gap-2">
        <span class="text-[var(--text-xs)] font-semibold uppercase tracking-wider text-[var(--text-tertiary)] mr-1">Presets</span>
        <For each={props.presets}>
          {(preset) => (
            <Button
              variant="secondary"
              size="sm"
              onClick={() => props.onApplyPreset(preset)}
              disabled={props.isToggling}
              title={preset.description}
            >
              {preset.name}
            </Button>
          )}
        </For>
      </div>

      {/* Search + filters */}
      <div class="mt-3 flex flex-wrap items-center gap-3">
        <div class="relative flex-1 min-w-[200px]">
          <input
            type="text"
            placeholder="Search apps, ports, protocols..."
            value={props.search}
            onInput={(e) => props.onSearchChange(e.currentTarget.value)}
            class="w-full rounded-lg border border-[var(--border-default)] bg-[var(--bg-secondary)] px-3 py-2 pl-8 text-[var(--text-sm)] text-[var(--text-primary)] placeholder-[var(--text-muted)] focus:border-[var(--accent-primary)] focus:outline-none"
          />
          <svg class="absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-[var(--text-muted)]" viewBox="0 0 20 20" fill="currentColor">
            <path fill-rule="evenodd" d="M9 3.5a5.5 5.5 0 100 11 5.5 5.5 0 000-11zM2 9a7 7 0 1112.452 4.391l3.328 3.329a.75.75 0 11-1.06 1.06l-3.329-3.328A7 7 0 012 9z" clip-rule="evenodd"/>
          </svg>
        </div>
        <div class="flex items-center gap-1.5">
          <button
            type="button"
            onClick={() => props.onFilterStatusChange(props.filterStatus === "enabled" ? "all" : "enabled")}
            class={`rounded-md px-2.5 py-1.5 text-[var(--text-xs)] font-medium transition-all ${
              props.filterStatus === "enabled"
                ? "border border-[var(--status-success)]/40 bg-[var(--success-subtle)] text-[var(--status-success)]"
                : "border border-[var(--border-default)] bg-[var(--bg-secondary)] text-[var(--text-tertiary)] hover:text-[var(--text-primary)]"
            }`}
          >
            Active ({props.enabledCount})
          </button>
          <button
            type="button"
            onClick={() => props.onFilterStatusChange(props.filterStatus === "disabled" ? "all" : "disabled")}
            class={`rounded-md px-2.5 py-1.5 text-[var(--text-xs)] font-medium transition-all ${
              props.filterStatus === "disabled"
                ? "border border-[var(--text-secondary)]/40 bg-[var(--bg-hover)] text-[var(--text-secondary)]"
                : "border border-[var(--border-default)] bg-[var(--bg-secondary)] text-[var(--text-tertiary)] hover:text-[var(--text-primary)]"
            }`}
          >
            Inactive ({props.totalCount - props.enabledCount})
          </button>
          <button
            type="button"
            onClick={props.onAddCustom}
            class="rounded-md border border-dashed border-[var(--border-strong)] px-2.5 py-1.5 text-[var(--text-xs)] font-medium text-[var(--text-tertiary)] transition-all hover:border-[var(--border-focus)] hover:text-[var(--text-secondary)]"
          >
            + Custom
          </button>
        </div>
      </div>

      {/* Category chips */}
      <div class="mt-3 flex flex-wrap gap-1.5">
        <button
          type="button"
          onClick={() => props.onFilterCategoryChange(null)}
          class={`rounded-md px-2.5 py-1 text-[var(--text-xs)] font-medium transition-all ${
            props.filterCategory === null
              ? "border border-[var(--status-success)]/40 bg-[var(--success-subtle)] text-[var(--status-success)]"
              : "border border-[var(--border-default)] text-[var(--text-tertiary)] hover:text-[var(--text-secondary)]"
          }`}
        >
          All
        </button>
        <For each={props.categories}>
          {(cat) => (
            <button
              type="button"
              onClick={() => props.onFilterCategoryChange(props.filterCategory === cat.key ? null : cat.key)}
              class={`rounded-md px-2.5 py-1 text-[var(--text-xs)] font-medium transition-all ${
                props.filterCategory === cat.key
                  ? "border border-[var(--status-success)]/40 bg-[var(--success-subtle)] text-[var(--status-success)]"
                  : "border border-[var(--border-default)] text-[var(--text-tertiary)] hover:text-[var(--text-secondary)]"
              }`}
            >
              {cat.label}
              <Show when={cat.enabledCount > 0}>
                <span class="ml-1 text-[var(--status-success)]">{cat.enabledCount}</span>
              </Show>
            </button>
          )}
        </For>
      </div>
    </div>
  );
}

export default RoutingToolbar;
