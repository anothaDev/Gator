import type { JSX } from "solid-js";

interface CardProps {
  children: JSX.Element;
  class?: string;
  variant?: "default" | "elevated" | "ghost";
  padding?: "none" | "sm" | "md" | "lg";
  interactive?: boolean;
}

export default function Card(props: CardProps) {
  const variantStyles = {
    default: "bg-[var(--bg-tertiary)] border-[var(--border-default)]",
    elevated: "bg-[var(--bg-elevated)] border-[var(--border-strong)] shadow-[var(--shadow-md)]",
    ghost: "bg-transparent border-transparent",
  };

  const paddingStyles = {
    none: "",
    sm: "p-3",
    md: "p-4",
    lg: "p-5",
  };

  return (
    <div
      class={[
        "rounded-[var(--radius-lg)] border",
        "transition-all duration-[var(--transition-base)]",
        variantStyles[props.variant ?? "default"],
        paddingStyles[props.padding ?? "md"],
        props.interactive && "cursor-pointer hover:border-[var(--border-focus)] hover:shadow-[var(--shadow-md)]",
        props.class ?? "",
      ].join(" ")}
    >
      {props.children}
    </div>
  );
}
