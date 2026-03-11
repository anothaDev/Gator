import type { JSX } from "solid-js";

interface InputProps {
  label?: string;
  value: string;
  onInput: (value: string) => void;
  type?: "text" | "password" | "email" | "number" | "url";
  placeholder?: string;
  disabled?: boolean;
  readOnly?: boolean;
  error?: string;
  hint?: string;
  class?: string;
}

export default function Input(props: InputProps) {
  const handleInput = (e: InputEvent) => {
    const target = e.target as HTMLInputElement;
    props.onInput(target.value);
  };

  return (
    <div class={["w-full", props.class ?? ""].join(" ")}>
      {props.label && (
        <label class="mb-1.5 block text-[var(--text-sm)] font-medium text-[var(--text-secondary)]">
          {props.label}
        </label>
      )}
      <input
        type={props.type ?? "text"}
        value={props.value}
        onInput={handleInput}
        placeholder={props.placeholder}
        disabled={props.disabled}
        readOnly={props.readOnly}
        class={[
          "w-full rounded-[var(--radius-lg)] border bg-[var(--bg-secondary)] px-3 py-2",
          "font-mono text-[var(--text-base)] text-[var(--text-primary)]",
          "placeholder:text-[var(--text-muted)]",
          "transition-all duration-[var(--transition-base)]",
          "focus:outline-none focus:ring-2 focus:ring-[var(--accent-primary)]/50 focus:border-[var(--accent-primary)]",
          props.error
            ? "border-[var(--status-error)] focus:border-[var(--status-error)] focus:ring-[var(--status-error)]/30"
            : "border-[var(--border-default)] hover:border-[var(--border-strong)]",
          props.disabled && "cursor-not-allowed opacity-60",
          props.readOnly && "cursor-default bg-[var(--bg-tertiary)]",
        ].join(" ")}
      />
      {props.error && (
        <p class="mt-1.5 text-[var(--text-xs)] text-[var(--status-error)]">
          {props.error}
        </p>
      )}
      {props.hint && !props.error && (
        <p class="mt-1.5 text-[var(--text-xs)] text-[var(--text-tertiary)]">
          {props.hint}
        </p>
      )}
    </div>
  );
}
