import type { JSX } from "solid-js";

interface BadgeProps {
  children: JSX.Element;
  variant?: "default" | "success" | "warning" | "error" | "info" | "muted";
  size?: "sm" | "md";
  class?: string;
}

export default function Badge(props: BadgeProps) {
  const variants: Record<string, string> = {
    default: "bg-brand-subtle text-brand",
    success: "bg-success-subtle text-success",
    warning: "bg-warning-subtle text-warning",
    error: "bg-error-subtle text-error",
    info: "bg-info-subtle text-info",
    muted: "bg-hover text-fg-muted",
  };

  const sizes: Record<string, string> = {
    sm: "text-label-xs px-1.5 py-px rounded",
    md: "text-label-xs px-2 py-0.5 rounded-md",
  };

  return (
    <span
      class={[
        "inline-flex items-center gap-1 font-medium whitespace-nowrap",
        variants[props.variant ?? "default"],
        sizes[props.size ?? "md"],
        props.class ?? "",
      ].join(" ")}
    >
      {props.children}
    </span>
  );
}
