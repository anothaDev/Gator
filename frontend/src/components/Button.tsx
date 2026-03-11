import type { JSX } from "solid-js";

interface ButtonProps {
  children: JSX.Element;
  variant?: "primary" | "secondary" | "ghost" | "danger";
  size?: "sm" | "md" | "lg";
  disabled?: boolean;
  loading?: boolean;
  type?: "button" | "submit" | "reset";
  class?: string;
  onClick?: (e: MouseEvent) => void;
}

export default function Button(props: ButtonProps) {
  const variantStyles = {
    primary: [
      "bg-[var(--accent-primary)] text-[var(--bg-primary)]",
      "hover:brightness-110",
      "active:scale-[0.98]",
    ].join(" "),
    secondary: [
      "bg-[var(--bg-elevated)] text-[var(--text-primary)] border border-[var(--border-strong)]",
      "hover:bg-[var(--bg-hover)] hover:border-[var(--border-focus)]",
      "active:scale-[0.98]",
    ].join(" "),
    ghost: [
      "bg-transparent text-[var(--text-secondary)]",
      "hover:bg-[var(--bg-hover)] hover:text-[var(--text-primary)]",
      "active:scale-[0.98]",
    ].join(" "),
    danger: [
      "bg-[var(--status-error)] text-white",
      "hover:brightness-110",
      "active:scale-[0.98]",
    ].join(" "),
  };

  const sizeStyles = {
    sm: "h-7 px-2.5 text-[12px]",
    md: "h-8 px-3 text-[13px]",
    lg: "h-9 px-4 text-[14px]",
  };

  return (
    <button
      type={props.type ?? "button"}
      disabled={props.disabled || props.loading}
      onClick={props.onClick}
      class={[
        "inline-flex items-center justify-center gap-1.5 rounded-[var(--radius-md)] font-medium",
        "transition-all duration-[var(--transition-fast)]",
        "disabled:opacity-50 disabled:cursor-not-allowed",
        variantStyles[props.variant ?? "secondary"],
        sizeStyles[props.size ?? "md"],
        props.class ?? "",
      ].join(" ")}
    >
      {props.loading && (
        <svg class="h-3.5 w-3.5 animate-spin" viewBox="0 0 24 24" fill="none">
          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="3" />
          <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
        </svg>
      )}
      {props.children}
    </button>
  );
}
