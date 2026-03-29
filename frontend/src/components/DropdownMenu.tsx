import { createSignal, For, Show, onCleanup } from "solid-js";
import type { JSX } from "solid-js";
import Spinner from "./Spinner";

export type MenuEntry =
  | { label: string; icon?: JSX.Element; onClick?: () => void; disabled?: boolean; loading?: boolean; danger?: boolean }
  | { label: string; icon?: JSX.Element; href: string; external?: boolean }
  | { divider: true };

export default function DropdownMenu(props: {
  items: MenuEntry[];
  align?: "left" | "right";
}) {
  const [open, setOpen] = createSignal(false);
  let ref: HTMLDivElement | undefined;

  const handleClickOutside = (e: MouseEvent) => {
    if (ref && !ref.contains(e.target as Node)) setOpen(false);
  };
  if (typeof document !== "undefined") {
    document.addEventListener("mousedown", handleClickOutside);
    onCleanup(() => document.removeEventListener("mousedown", handleClickOutside));
  }

  return (
    <div class="relative" ref={ref}>
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        class={[
          "inline-flex h-8 w-8 items-center justify-center rounded-md",
          "text-fg-muted transition-all duration-150",
          "hover:bg-hover hover:text-fg",
          open() ? "bg-hover text-fg" : "",
        ].join(" ")}
        title="More options"
      >
        <svg class="h-4 w-4" viewBox="0 0 24 24" fill="currentColor">
          <circle cx="12" cy="5" r="2" />
          <circle cx="12" cy="12" r="2" />
          <circle cx="12" cy="19" r="2" />
        </svg>
      </button>
      <Show when={open()}>
        <div
          class={[
            "absolute top-full z-50 mt-1 min-w-[180px] overflow-hidden rounded-lg",
            "border border-border bg-surface-raised py-1 shadow-lg",
            "animate-scale-in",
            props.align === "left" ? "left-0 origin-top-left" : "right-0 origin-top-right",
          ].join(" ")}
        >
          <For each={props.items}>
            {(item) => {
              if ("divider" in item) {
                return <div class="my-1 border-t border-border-faint" />;
              }
              if ("href" in item) {
                return (
                  <a
                    href={item.href}
                    target={item.external ? "_blank" : undefined}
                    rel={item.external ? "noopener noreferrer" : undefined}
                    onClick={() => setOpen(false)}
                    class="flex w-full items-center gap-2.5 px-3 py-2 text-left text-sm text-fg-secondary transition-colors hover:bg-hover hover:text-fg"
                  >
                    <Show when={item.icon}>
                      <span class="shrink-0 text-fg-muted">{item.icon}</span>
                    </Show>
                    <span class="flex-1">{item.label}</span>
                    <Show when={item.external}>
                      <svg class="h-3 w-3 shrink-0 text-fg-muted" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                        <path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6" />
                        <polyline points="15 3 21 3 21 9" />
                        <line x1="10" y1="14" x2="21" y2="3" />
                      </svg>
                    </Show>
                  </a>
                );
              }
              return (
                <button
                  type="button"
                  disabled={item.disabled}
                  onClick={() => {
                    setOpen(false);
                    item.onClick?.();
                  }}
                  class={[
                    "flex w-full items-center gap-2.5 px-3 py-2 text-left text-sm transition-colors",
                    item.danger
                      ? "text-error hover:bg-error-subtle"
                      : "text-fg-secondary hover:bg-hover hover:text-fg",
                    "disabled:opacity-40 disabled:cursor-not-allowed",
                  ].join(" ")}
                >
                  <Show when={item.loading}>
                    <Spinner size="xs" />
                  </Show>
                  <Show when={item.icon && !item.loading}>
                    <span class="shrink-0 text-fg-muted">{item.icon}</span>
                  </Show>
                  {item.label}
                </button>
              );
            }}
          </For>
        </div>
      </Show>
    </div>
  );
}
