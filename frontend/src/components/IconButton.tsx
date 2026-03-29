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
      "bg-surface-raised text-fg-secondary",
      "hover:bg-hover hover:text-fg",
      "active:bg-active",
    ].join(" "),
    ghost: [
      "bg-transparent text-fg-muted",
      "hover:bg-hover hover:text-fg",
      "active:bg-active",
    ].join(" "),
    subtle: [
      "bg-transparent text-fg-muted",
      "hover:text-fg-secondary",
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
        "inline-flex items-center justify-center rounded-md",
        "transition-all duration-base",
        "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand focus-visible:ring-offset-2 focus-visible:ring-offset-surface",
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
