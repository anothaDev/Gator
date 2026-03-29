import { Show, For, type JSX, onMount, onCleanup } from "solid-js";
import { Transition } from "solid-transition-group";

interface AnimatedViewProps {
  children: JSX.Element;
  visible: boolean;
  class?: string;
  animation?: "fade" | "slide-up" | "scale";
  duration?: number;
  delay?: number;
}

export function AnimatedView(props: AnimatedViewProps) {
  const animationClass = () => {
    switch (props.animation ?? "fade") {
      case "slide-up":
        return "view-enter view-enter-active";
      case "scale":
        return "modal-enter modal-enter-active";
      default:
        return "view-enter view-enter-active";
    }
  };

  return (
    <Transition
      enterActiveClass="view-enter-active"
      exitActiveClass="view-exit-active"
      enterClass="view-enter"
      exitClass="view-exit"
      onEnter={(el, done) => {
        if (props.delay) {
          setTimeout(done, props.delay);
        }
      }}
      onExit={(el, done) => {
        setTimeout(done, props.duration ?? 200);
      }}
    >
      <Show when={props.visible}>
        <div class={props.class} style={{ "animation-delay": `${props.delay ?? 0}ms` }}>
          {props.children}
        </div>
      </Show>
    </Transition>
  );
}

interface AnimatedListProps<T> {
  items: T[];
  keyExtractor: (item: T, index: number) => string | number;
  renderItem: (item: T, index: number) => JSX.Element;
  class?: string;
  itemClass?: string;
  staggerDelay?: number;
  animation?: "fade" | "slide-up" | "scale";
}

export function AnimatedList<T>(props: AnimatedListProps<T>) {
  const getAnimationClass = (index: number) => {
    const delay = (props.staggerDelay ?? 50) * index;
    const baseClass = "animate-fade-in";
    
    switch (props.animation ?? "slide-up") {
      case "slide-up":
        return `${baseClass} animate-slide-up stagger-${Math.min(index + 1, 6)}`;
      case "scale":
        return `${baseClass} animate-scale-up stagger-${Math.min(index + 1, 6)}`;
      default:
        return baseClass;
    }
  };

  return (
    <div class={props.class}>
      <For each={props.items}>
        {(item, index) => (
          <div
            class={getAnimationClass(index())}
            style={{ "animation-delay": `${(props.staggerDelay ?? 50) * index()}ms` }}
          >
            {props.renderItem(item, index())}
          </div>
        )}
      </For>
    </div>
  );
}

interface AnimatedCardProps {
  children: JSX.Element;
  class?: string;
  interactive?: boolean;
  onClick?: () => void;
}

export function AnimatedCard(props: AnimatedCardProps) {
  return (
    <div
      class={[
        "transition-all duration-base",
        props.interactive && "hover-lift cursor-pointer active-scale",
        props.class ?? "",
      ].join(" ")}
      onClick={props.onClick}
    >
      {props.children}
    </div>
  );
}

interface StaggerContainerProps {
  children: JSX.Element;
  class?: string;
  staggerDelay?: number;
}

export function StaggerContainer(props: StaggerContainerProps) {
  return (
    <div 
      class={props.class}
      style={{ "--stagger-delay": `${props.staggerDelay ?? 50}ms` }}
    >
      {props.children}
    </div>
  );
}

interface FadeInProps {
  children: JSX.Element;
  class?: string;
  delay?: number;
  duration?: number;
}

export function FadeIn(props: FadeInProps) {
  return (
    <div
      class={["animate-fade-in", props.class ?? ""].join(" ")}
      style={{
        "animation-delay": `${props.delay ?? 0}ms`,
        "animation-duration": `${props.duration ?? 250}ms`,
      }}
    >
      {props.children}
    </div>
  );
}

interface SlideUpProps {
  children: JSX.Element;
  class?: string;
  delay?: number;
  duration?: number;
}

export function SlideUp(props: SlideUpProps) {
  return (
    <div
      class={["animate-slide-up", props.class ?? ""].join(" ")}
      style={{
        "animation-delay": `${props.delay ?? 0}ms`,
        "animation-duration": `${props.duration ?? 350}ms`,
      }}
    >
      {props.children}
    </div>
  );
}

interface PageTransitionProps {
  children: JSX.Element;
  class?: string;
}

export function PageTransition(props: PageTransitionProps) {
  return (
    <Transition
      enterActiveClass="view-enter-active"
      exitActiveClass="view-exit-active"
      enterClass="view-enter"
      exitClass="view-exit"
      mode="outin"
    >
      {props.children}
    </Transition>
  );
}
