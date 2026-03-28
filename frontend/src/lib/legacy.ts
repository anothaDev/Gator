import { createSignal } from "solid-js";
import { apiGet } from "./api";

/**
 * SolidJS hook for the legacy firewall rules migration check.
 * Returns reactive signals and an async checker function.
 *
 * Usage:
 *   const { legacyCount, legacyChecked, checkLegacyRules } = createLegacyCheck();
 *   onMount(() => void checkLegacyRules());
 */
export function createLegacyCheck() {
  const [legacyCount, setLegacyCount] = createSignal(0);
  const [legacyChecked, setLegacyChecked] = createSignal(false);

  const checkLegacyRules = async () => {
    try {
      const { ok, data } = await apiGet<{ legacy_count?: number; legacy_available?: boolean }>(
        "/api/opnsense/migration/status",
      );
      if (ok && data.legacy_available && (data.legacy_count ?? 0) > 0) {
        setLegacyCount(data.legacy_count ?? 0);
      }
    } catch {
      // Silent — best-effort check.
    } finally {
      setLegacyChecked(true);
    }
  };

  return { legacyCount, legacyChecked, checkLegacyRules } as const;
}
