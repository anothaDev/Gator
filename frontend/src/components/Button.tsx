import type { JSX } from "solid-js";
import Spinner from "./Spinner";

interface ButtonProps {
  children: JSX.Element;
  variant?: "primary" | "secondary" | "ghost" | "danger";
  size?: "sm" | "md" | "lg";
  disabled?: boolean;
  loading?: boolean;
  type?: "button" | "submit" | "reset";
  class?: string;
  title?: string;
  onClick?: (e: MouseEvent) => void;
}

export default function Button(props: ButtonProps) {
  const variants: Record<string, string> = {
    primary: [
      "bg-brand text-white",
      "hover:bg-brand-hover",
      "active:scale-[0.995]",
      "focus-visible:ring-2 focus-visible:ring-brand/40 focus-visible:ring-offset-2",
    ].join(" "),
    secondary: [
      "border border-border bg-surface text-fg-secondary",
      "hover:bg-hover hover:text-fg hover:border-border-strong",
      "active:scale-[0.99]",
      "focus-visible:ring-2 focus-visible:ring-border-strong",
    ].join(" "),
    ghost: [
      "bg-transparent text-fg-secondary",
      "hover:bg-hover hover:text-fg",
      "active:scale-[0.99]",
      "focus-visible:ring-2 focus-visible:ring-border-strong",
    ].join(" "),
    danger: [
      "bg-error text-white",
      "hover:bg-error/90",
      "active:scale-[0.995]",
      "focus-visible:ring-2 focus-visible:ring-error/40 focus-visible:ring-offset-2",
    ].join(" "),
  };

  const sizes: Record<string, string> = {
    sm: "h-7 px-2.5 text-label-sm rounded-md gap-1.5",
    md: "h-8 px-3 text-label-md rounded-md gap-2",
    lg: "h-9 px-4 text-label-md rounded-lg gap-2",
  };

  return (
    <button
      type={props.type ?? "button"}
      disabled={props.disabled || props.loading}
      onClick={props.onClick}
      title={props.title}
      class={[
        "inline-flex items-center justify-center font-medium",
        "transition-all duration-150",
        "disabled:opacity-50 disabled:pointer-events-none",
        "focus-visible:outline-none",
        variants[props.variant ?? "secondary"],
        sizes[props.size ?? "md"],
        props.class ?? "",
      ].join(" ")}
    >
      {props.loading && <Spinner size="xs" />}
      {props.children}
    </button>
  );
}
