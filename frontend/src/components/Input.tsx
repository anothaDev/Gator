import { Show, type JSX } from "solid-js";

interface InputProps {
  label?: string;
  value: string;
  onInput: (value: string) => void;
  type?: string;
  placeholder?: string;
  disabled?: boolean;
  readOnly?: boolean;
  error?: string;
  hint?: string;
  class?: string;
}

export default function Input(props: InputProps) {
  const handleInput: JSX.EventHandler<HTMLInputElement, InputEvent> = (e) => {
    props.onInput(e.currentTarget.value);
  };

  return (
    <div class={["w-full", props.class ?? ""].join(" ")}>
      <Show when={props.label}>
        <label class="text-label-sm text-fg-secondary mb-1.5 block">
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
          "w-full rounded-lg border bg-surface px-3 py-2",
          "text-body-md text-fg",
          "placeholder:text-fg-muted",
          "transition-colors duration-150",
          "focus:outline-none focus:ring-2 focus:ring-brand/30 focus:border-brand",
          props.error
            ? "border-error focus:border-error focus:ring-error/20"
            : "border-border hover:border-border-strong",
          props.disabled ? "cursor-not-allowed opacity-50" : "",
          props.readOnly ? "cursor-default bg-hover" : "",
        ].join(" ")}
      />
      <Show when={props.error}>
        <p class="mt-1.5 text-body-xs text-error">{props.error}</p>
      </Show>
      <Show when={props.hint && !props.error}>
        <p class="mt-1.5 text-body-xs text-fg-muted">{props.hint}</p>
      </Show>
    </div>
  );
}
