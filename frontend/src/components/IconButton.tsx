import type { JSX } from "solid-js";

interface IconButtonProps {
  children: JSX.Element;
  variant?: "default" | "ghost" | "subtle";
  size?: "sm" | "md" | "lg";
  disabled?: boolean;
  title?: string;
  class?: string;
  onClick?: (e: MouseEvent) => void;
}

export default function IconButton(props: IconButtonProps) {
  const variantStyles = {
    default: [
      "bg-[var(--bg-elevated)] text-[var(--text-secondary)]",
      "hover:bg-[var(--bg-hover)] hover:text-[var(--text-primary)]",
      "active:bg-[var(--bg-active)]",
    ].join(" "),
    ghost: [
      "bg-transparent text-[var(--text-tertiary)]",
      "hover:bg-[var(--bg-hover)] hover:text-[var(--text-primary)]",
      "active:bg-[var(--bg-active)]",
    ].join(" "),
    subtle: [
      "bg-transparent text-[var(--text-muted)]",
      "hover:text-[var(--text-secondary)]",
    ].join(" "),
  };

  const sizeStyles = {
    sm: "h-7 w-7",
    md: "h-8 w-8",
    lg: "h-9 w-9",
  };

  return (
    <button
      type="button"
      disabled={props.disabled}
      title={props.title}
      onClick={props.onClick}
      class={[
        "inline-flex items-center justify-center rounded-[var(--radius-md)]",
        "transition-all duration-[var(--transition-base)]",
        "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent-primary)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--bg-primary)]",
        "disabled:opacity-50 disabled:cursor-not-allowed",
        variantStyles[props.variant ?? "ghost"],
        sizeStyles[props.size ?? "md"],
        props.class ?? "",
      ].join(" ")}
    >
      {props.children}
    </button>
  );
}
