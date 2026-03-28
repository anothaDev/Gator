import { Show, type JSX } from "solid-js";

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
      <Show when={props.label}>
        <label class="mb-1.5 block text-sm font-medium text-fg-secondary">
          {props.label}
        </label>
      </Show>
      <input
        type={props.type ?? "text"}
        value={props.value}
        onInput={handleInput}
        placeholder={props.placeholder}
        disabled={props.disabled}
        readOnly={props.readOnly}
        class={[
          "w-full rounded-lg border bg-surface-secondary px-3 py-2",
          "font-mono text-base text-fg",
          "placeholder:text-fg-muted",
          "transition-all duration-base",
          "focus:outline-none focus:ring-2 focus:ring-accent/50 focus:border-accent",
          props.error
            ? "border-error focus:border-error focus:ring-error/30"
            : "border-line hover:border-line-strong",
          props.disabled && "cursor-not-allowed opacity-60",
          props.readOnly && "cursor-default bg-surface-tertiary",
        ].join(" ")}
      />
      <Show when={props.error}>
        <p class="mt-1.5 text-xs text-error">
          {props.error}
        </p>
      </Show>
      <Show when={props.hint && !props.error}>
        <p class="mt-1.5 text-xs text-fg-tertiary">
          {props.hint}
        </p>
      </Show>
    </div>
  );
}
