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
    default: "bg-surface-tertiary border-line",
    elevated: "bg-elevated border-line-strong shadow-md",
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
        "rounded-lg border",
        "transition-all duration-base",
        variantStyles[props.variant ?? "default"],
        paddingStyles[props.padding ?? "md"],
        props.interactive && "cursor-pointer hover:border-line-focus hover:shadow-md",
        props.class ?? "",
      ].join(" ")}
    >
      {props.children}
    </div>
  );
}
