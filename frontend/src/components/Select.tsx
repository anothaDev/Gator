import { For, type JSX } from "solid-js";

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
  const handleChange = (e: Event) => {
    const target = e.target as HTMLSelectElement;
    props.onChange(target.value);
  };

  return (
    <div class={["w-full", props.class ?? ""].join(" ")}>
      {props.label && (
        <label class="mb-1.5 block text-[var(--text-sm)] font-medium text-[var(--text-secondary)]">
          {props.label}
        </label>
      )}
      <div class="relative">
        <select
          value={props.value}
          onChange={handleChange}
          disabled={props.disabled}
          class={[
            "w-full appearance-none rounded-[var(--radius-lg)] border bg-[var(--bg-secondary)] px-3 py-2 pr-10",
            "text-[var(--text-base)] text-[var(--text-primary)]",
            "transition-all duration-[var(--transition-base)]",
            "focus:outline-none focus:ring-2 focus:ring-[var(--accent-primary)]/50 focus:border-[var(--accent-primary)]",
            "border-[var(--border-default)] hover:border-[var(--border-strong)]",
            props.disabled && "cursor-not-allowed opacity-60",
          ].join(" ")}
        >
          {props.placeholder && (
            <option value="" disabled>
              {props.placeholder}
            </option>
          )}
          <For each={props.options}>
            {(option) => (
              <option value={option.value}>{option.label}</option>
            )}
          </For>
        </select>
        <div class="pointer-events-none absolute inset-y-0 right-0 flex items-center px-3">
          <svg
            class="h-4 w-4 text-[var(--text-tertiary)]"
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
            stroke-width="2"
          >
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              d="M19 9l-7 7-7-7"
            />
          </svg>
        </div>
      </div>
    </div>
  );
}
