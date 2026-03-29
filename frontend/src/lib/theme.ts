import { createSignal, createEffect, onMount } from "solid-js";

export type Theme = "dark" | "light" | "system";

const STORAGE_KEY = "gator-theme";

function getSystemTheme(): "dark" | "light" {
  return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
}

function applyTheme(theme: Theme) {
  const root = document.documentElement;
  const resolvedTheme = theme === "system" ? getSystemTheme() : theme;
  
  // Add transition class for smooth theme switch
  root.classList.add("theme-transition");
  
  root.dataset.theme = resolvedTheme;
  root.style.colorScheme = resolvedTheme;
  
  // Remove transition class after animation completes
  setTimeout(() => {
    root.classList.remove("theme-transition");
  }, 250);
}

export function createTheme() {
  const [theme, setThemeSignal] = createSignal<Theme>("system");
  const [resolvedTheme, setResolvedTheme] = createSignal<"dark" | "light">("dark");

  onMount(() => {
    // Load saved preference
    const saved = localStorage.getItem(STORAGE_KEY) as Theme | null;
    const initialTheme = saved ?? "system";
    setThemeSignal(initialTheme);
    
    const resolved = initialTheme === "system" ? getSystemTheme() : initialTheme;
    setResolvedTheme(resolved);
    applyTheme(initialTheme);

    // Listen for system theme changes
    const mediaQuery = window.matchMedia("(prefers-color-scheme: dark)");
    const handleChange = () => {
      if (theme() === "system") {
        const newTheme = getSystemTheme();
        setResolvedTheme(newTheme);
        applyTheme("system");
      }
    };

    mediaQuery.addEventListener("change", handleChange);
  });

  createEffect(() => {
    const currentTheme = theme();
    localStorage.setItem(STORAGE_KEY, currentTheme);
    
    const resolved = currentTheme === "system" ? getSystemTheme() : currentTheme;
    setResolvedTheme(resolved);
    applyTheme(currentTheme);
  });

  const setTheme = (newTheme: Theme) => {
    setThemeSignal(newTheme);
  };

  const toggleTheme = () => {
    const current = resolvedTheme();
    const newTheme = current === "dark" ? "light" : "dark";
    setThemeSignal(newTheme);
  };

  return {
    theme,
    resolvedTheme,
    setTheme,
    toggleTheme,
  };
}

// Convenience hook for components that just need to know the current theme
export function useTheme() {
  return createTheme();
}
