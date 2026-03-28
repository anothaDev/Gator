import { createSignal, For, Show, onCleanup } from "solid-js";

interface SelectOption {
  value: string;
  label: string;
}

interface SelectProps {
  label?: string;
  value: string;
  options: SelectOption[];
  onChange: (value: string) => void;
  disabled?: boolean;
  placeholder?: string;
  class?: string;
}

export default function Select(props: SelectProps) {
  const [open, setOpen] = createSignal(false);
  let ref: HTMLDivElement | undefined;

  const handleClickOutside = (e: MouseEvent) => {
    if (ref && !ref.contains(e.target as Node)) setOpen(false);
  };
  if (typeof document !== "undefined") {
    document.addEventListener("mousedown", handleClickOutside);
    onCleanup(() => document.removeEventListener("mousedown", handleClickOutside));
  }

  const selectedLabel = () => {
    const opt = props.options.find((o) => o.value === props.value);
    return opt?.label ?? props.placeholder ?? "";
  };

  return (
    <div class={["w-full", props.class ?? ""].join(" ")} ref={ref}>
      {props.label && (
        <label class="mb-1.5 block text-sm font-medium text-fg-secondary">
          {props.label}
        </label>
      )}
      <div class="relative">
        <button
          type="button"
          disabled={props.disabled}
          onClick={() => setOpen((v) => !v)}
          class={[
            "flex w-full items-center justify-between rounded-lg border bg-surface-secondary px-3 py-2 text-left",
            "text-sm text-fg",
            "transition-all duration-base",
            open()
              ? "border-accent ring-2 ring-accent/50"
              : "border-line hover:border-line-strong",
            props.disabled ? "cursor-not-allowed opacity-60" : "",
          ].join(" ")}
        >
          <span class={props.value ? "" : "text-fg-muted"}>
            {selectedLabel()}
          </span>
          <svg
            class={["h-4 w-4 shrink-0 text-fg-tertiary transition-transform", open() ? "rotate-180" : ""].join(" ")}
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
            stroke-width="2"
          >
            <path stroke-linecap="round" stroke-linejoin="round" d="M19 9l-7 7-7-7" />
          </svg>
        </button>
        <Show when={open()}>
          <div class="absolute left-0 top-full z-50 mt-1 max-h-60 w-full overflow-auto rounded-lg border border-line bg-elevated py-1 shadow-xl shadow-black/40">
            <For each={props.options}>
              {(option) => (
                <button
                  type="button"
                  onClick={() => {
                    props.onChange(option.value);
                    setOpen(false);
                  }}
                  class={[
                    "flex w-full items-center px-3 py-2 text-left text-sm transition-colors",
                    option.value === props.value
                      ? "bg-accent/10 font-medium text-accent"
                      : "text-fg hover:bg-hover",
                  ].join(" ")}
                >
                  {option.label}
                </button>
              )}
            </For>
          </div>
        </Show>
      </div>
    </div>
  );
}
