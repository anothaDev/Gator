import type { JSX } from "solid-js";

interface BadgeProps {
  children: JSX.Element;
  variant?: "default" | "success" | "warning" | "error" | "info" | "muted";
  size?: "sm" | "md";
  class?: string;
}

export default function Badge(props: BadgeProps) {
  const variantStyles = {
    default: "bg-accent-subtle text-accent",
    success: "bg-success-subtle text-success",
    warning: "bg-warning-subtle text-warning",
    error: "bg-error-subtle text-error",
    info: "bg-info-subtle text-info",
    muted: "bg-active text-fg-tertiary",
  };

  const sizeStyles = {
    sm: "text-[10px] px-1.5 py-0.5",
    md: "text-[11px] px-2 py-0.5",
  };

  return (
    <span
      class={[
        "inline-flex items-center gap-1 rounded-sm font-medium",
        variantStyles[props.variant ?? "default"],
        sizeStyles[props.size ?? "md"],
        props.class ?? "",
      ].join(" ")}
    >
      {props.children}
    </span>
  );
}
