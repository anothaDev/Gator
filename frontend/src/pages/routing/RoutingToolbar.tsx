import { For, Show } from "solid-js";
import Button from "../../components/Button";
import type { AppPreset, CategoryInfo } from "./types";

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
    <div class="rounded-lg border border-border-faint bg-surface-raised p-4">
      {/* Presets row */}
      <div class="flex flex-wrap items-center gap-2">
        <span class="text-xs font-semibold uppercase tracking-wider text-fg-muted mr-1">Presets</span>
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
            class="w-full rounded-lg border border-border bg-surface px-3 py-2 pl-8 text-sm text-fg placeholder-fg-muted focus:border-brand focus:outline-none"
          />
          <svg class="absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-fg-muted" viewBox="0 0 20 20" fill="currentColor">
            <path fill-rule="evenodd" d="M9 3.5a5.5 5.5 0 100 11 5.5 5.5 0 000-11zM2 9a7 7 0 1112.452 4.391l3.328 3.329a.75.75 0 11-1.06 1.06l-3.329-3.328A7 7 0 012 9z" clip-rule="evenodd"/>
          </svg>
        </div>
        <div class="flex items-center gap-1.5">
          <button
            type="button"
            onClick={() => props.onFilterStatusChange(props.filterStatus === "enabled" ? "all" : "enabled")}
            class={`rounded-md px-2.5 py-1.5 text-xs font-medium transition-all ${
              props.filterStatus === "enabled"
                ? "border border-success/40 bg-success-subtle text-success"
                : "border-transparent bg-surface text-fg-muted hover:text-fg"
            }`}
          >
            Active ({props.enabledCount})
          </button>
          <button
            type="button"
            onClick={() => props.onFilterStatusChange(props.filterStatus === "disabled" ? "all" : "disabled")}
            class={`rounded-md px-2.5 py-1.5 text-xs font-medium transition-all ${
              props.filterStatus === "disabled"
                ? "border border-fg-secondary/40 bg-hover text-fg-secondary"
                : "border-transparent bg-surface text-fg-muted hover:text-fg"
            }`}
          >
            Inactive ({props.totalCount - props.enabledCount})
          </button>
          <button
            type="button"
            onClick={props.onAddCustom}
            class="rounded-md border border-dashed border-transparent px-2.5 py-1.5 text-xs font-medium text-fg-muted transition-all hover:border-border-strong hover:text-fg-secondary"
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
          class={`rounded-md px-2.5 py-1 text-xs font-medium transition-all ${
            props.filterCategory === null
              ? "border border-success/40 bg-success-subtle text-success"
              : "border-transparent text-fg-muted hover:text-fg-secondary"
          }`}
        >
          All
        </button>
        <For each={props.categories}>
          {(cat) => (
            <button
              type="button"
              onClick={() => props.onFilterCategoryChange(props.filterCategory === cat.key ? null : cat.key)}
              class={`rounded-md px-2.5 py-1 text-xs font-medium transition-all ${
                props.filterCategory === cat.key
                  ? "border border-success/40 bg-success-subtle text-success"
                  : "border-transparent text-fg-muted hover:text-fg-secondary"
              }`}
            >
              {cat.label}
              <Show when={cat.enabledCount > 0}>
                <span class="ml-1 text-success">{cat.enabledCount}</span>
              </Show>
            </button>
          )}
        </For>
      </div>
    </div>
  );
}

export default RoutingToolbar;
