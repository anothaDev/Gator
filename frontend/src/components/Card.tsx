import type { JSX } from "solid-js";

interface CardProps {
  children: JSX.Element;
  class?: string;
  variant?: "default" | "elevated" | "ghost";
  padding?: "none" | "sm" | "md" | "lg";
  interactive?: boolean;
}

export default function Card(props: CardProps) {
  const base = "rounded-lg border";

  const variants: Record<string, string> = {
    default: "bg-surface border-border-faint",
    elevated: "bg-surface-raised border-border shadow-sm",
    ghost: "bg-transparent border-transparent",
  };

  const paddings: Record<string, string> = {
    none: "",
    sm: "p-3",
    md: "p-4",
    lg: "p-5",
  };

  return (
    <div
      class={[
        base,
        variants[props.variant ?? "default"],
        paddings[props.padding ?? "md"],
        props.interactive ? "cursor-pointer transition-shadow duration-200 hover:shadow-md" : "",
        props.class ?? "",
      ].join(" ")}
    >
      {props.children}
    </div>
  );
}
