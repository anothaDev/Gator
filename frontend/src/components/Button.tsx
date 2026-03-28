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
  const variantStyles = {
    primary: [
      "bg-accent text-surface",
      "hover:brightness-110",
      "active:scale-[0.98]",
    ].join(" "),
    secondary: [
      "bg-elevated text-fg border border-line-strong",
      "hover:bg-hover hover:border-line-focus",
      "active:scale-[0.98]",
    ].join(" "),
    ghost: [
      "bg-transparent text-fg-secondary",
      "hover:bg-hover hover:text-fg",
      "active:scale-[0.98]",
    ].join(" "),
    danger: [
      "bg-error text-white",
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
      title={props.title}
      class={[
        "inline-flex items-center justify-center gap-1.5 rounded-md font-medium",
        "transition-all duration-fast",
        "disabled:opacity-50 disabled:cursor-not-allowed",
        variantStyles[props.variant ?? "secondary"],
        sizeStyles[props.size ?? "md"],
        props.class ?? "",
      ].join(" ")}
    >
      {props.loading && <Spinner size="xs" />}
      {props.children}
    </button>
  );
}
