import type { JSX } from "solid-js";

interface BadgeProps {
  children: JSX.Element;
  variant?: "default" | "success" | "warning" | "error" | "info" | "muted";
  size?: "sm" | "md";
  class?: string;
}

export default function Badge(props: BadgeProps) {
  const variantStyles = {
    default: "bg-[var(--accent-primary-subtle)] text-[var(--accent-primary)]",
    success: "bg-[var(--success-subtle)] text-[var(--status-success)]",
    warning: "bg-[var(--warning-subtle)] text-[var(--status-warning)]",
    error: "bg-[var(--error-subtle)] text-[var(--status-error)]",
    info: "bg-[var(--info-subtle)] text-[var(--status-info)]",
    muted: "bg-[var(--bg-active)] text-[var(--text-tertiary)]",
  };

  const sizeStyles = {
    sm: "text-[10px] px-1.5 py-0.5",
    md: "text-[11px] px-2 py-0.5",
  };

  return (
    <span
      class={[
        "inline-flex items-center gap-1 rounded-[var(--radius-sm)] font-medium",
        variantStyles[props.variant ?? "default"],
        sizeStyles[props.size ?? "md"],
        props.class ?? "",
      ].join(" ")}
    >
      {props.children}
    </span>
  );
}
