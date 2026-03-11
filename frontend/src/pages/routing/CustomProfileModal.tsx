import { createSignal, For, Show } from "solid-js";
import { apiPost } from "../../lib/api";

// ─── Types ───────────────────────────────────────────────────────

type PortRule = {
  protocol: string;
  ports: string;
};

// ─── Custom Profile Modal ────────────────────────────────────────

function CustomProfileModal(props: {
  onClose: () => void;
  onSaved: () => void;
}) {
  const [customName, setCustomName] = createSignal("");
  const [customCategory, setCustomCategory] = createSignal("custom");
  const [customRules, setCustomRules] = createSignal<PortRule[]>([{ protocol: "TCP", ports: "" }]);
  const [customASNs, setCustomASNs] = createSignal("");
  const [customNote, setCustomNote] = createSignal("");
  const [customSaving, setCustomSaving] = createSignal(false);
  const [customErr, setCustomErr] = createSignal("");

  const addRuleRow = () => {
    setCustomRules([...customRules(), { protocol: "TCP", ports: "" }]);
  };

  const removeRuleRow = (idx: number) => {
    setCustomRules(customRules().filter((_, i) => i !== idx));
  };

  const updateRuleRow = (idx: number, field: "protocol" | "ports", value: string) => {
    setCustomRules(customRules().map((r, i) => (i === idx ? { ...r, [field]: value } : r)));
  };

  const saveCustomProfile = async () => {
    setCustomErr("");
    const name = customName().trim();
    if (!name) { setCustomErr("Name is required"); return; }
    const rules = customRules().filter((r) => r.ports.trim() !== "");
    if (rules.length === 0) { setCustomErr("At least one port rule is required"); return; }

    // Parse ASN numbers from comma-separated input.
    const asnsParsed = customASNs()
      .split(/[,\s]+/)
      .map((s) => parseInt(s.trim(), 10))
      .filter((n) => !isNaN(n) && n > 0);

    setCustomSaving(true);
    try {
      const { ok, data } = await apiPost("/api/app-profiles", {
        name,
        category: customCategory(),
        rules: rules.map((r) => ({ protocol: r.protocol, ports: r.ports.trim() })),
        asns: asnsParsed.length > 0 ? asnsParsed : undefined,
        note: customNote().trim(),
      });
      if (!ok) {
        setCustomErr((data as { error?: string })?.error ?? "Failed to create profile");
        return;
      }
      props.onSaved();
    } catch {
      setCustomErr("Failed to create profile. Check backend connectivity.");
    } finally {
      setCustomSaving(false);
    }
  };

  return (
    <div class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm" onClick={(e) => { if (e.target === e.currentTarget) props.onClose(); }}>
      <div class="mx-4 w-full max-w-lg rounded-2xl border border-[var(--border-default)] bg-[var(--bg-secondary)] p-6 shadow-2xl">
        <h2 class="text-lg font-bold text-[var(--text-primary)]">Add Custom Service</h2>
        <p class="mt-1 text-xs text-[var(--text-tertiary)]">Define a custom app or service with its protocol and port rules.</p>

        <Show when={customErr()}>
          <div class="mt-3 rounded-lg border border-red-500/30 bg-red-500/10 px-3 py-2 text-xs text-red-300">
            {customErr()}
          </div>
        </Show>

        <div class="mt-4 space-y-3">
          <div>
            <label class="text-xs font-medium text-[var(--text-tertiary)]">Name</label>
            <input
              type="text"
              placeholder="e.g. My Game Server"
              value={customName()}
              onInput={(e) => setCustomName(e.currentTarget.value)}
              class="mt-1 w-full rounded-lg border border-[var(--border-default)] bg-[var(--bg-tertiary)]/50 px-3 py-2 text-sm text-[var(--text-primary)] placeholder-[var(--text-muted)] focus:border-[var(--accent-primary)] focus:outline-none"
            />
          </div>

          <div>
            <label class="text-xs font-medium text-[var(--text-tertiary)]">Category</label>
            <select
              value={customCategory()}
              onChange={(e) => setCustomCategory(e.currentTarget.value)}
              class="mt-1 w-full rounded-lg border border-[var(--border-default)] bg-[var(--bg-tertiary)]/50 px-3 py-2 text-sm text-[var(--text-primary)] focus:border-[var(--accent-primary)] focus:outline-none"
            >
              <option value="custom">Custom</option>
              <option value="gaming">Gaming</option>
              <option value="streaming">Streaming</option>
              <option value="communication">Communication</option>
              <option value="file_sharing">File Sharing</option>
              <option value="browsing">Browsing</option>
              <option value="remote_access">Remote Access</option>
              <option value="home_iot">Home & IoT</option>
              <option value="mail">Mail</option>
            </select>
          </div>

          <div>
            <div class="flex items-center justify-between">
              <label class="text-xs font-medium text-[var(--text-tertiary)]">Port Rules</label>
              <button
                type="button"
                onClick={addRuleRow}
                class="text-xs text-[var(--status-success)] hover:text-[var(--status-success)]"
              >
                + Add rule
              </button>
            </div>
            <div class="mt-2 space-y-2">
              <For each={customRules()}>
                {(rule, idx) => (
                  <div class="flex items-center gap-2">
                    <select
                      value={rule.protocol}
                      onChange={(e) => updateRuleRow(idx(), "protocol", e.currentTarget.value)}
                      class="w-28 rounded-lg border border-[var(--border-default)] bg-[var(--bg-tertiary)]/50 px-2 py-1.5 text-xs text-[var(--text-primary)] focus:border-[var(--accent-primary)] focus:outline-none"
                    >
                      <option value="TCP">TCP</option>
                      <option value="UDP">UDP</option>
                      <option value="TCP/UDP">TCP/UDP</option>
                    </select>
                    <input
                      type="text"
                      placeholder="e.g. 8080 or 3000-3100"
                      value={rule.ports}
                      onInput={(e) => updateRuleRow(idx(), "ports", e.currentTarget.value)}
                      class="flex-1 rounded-lg border border-[var(--border-default)] bg-[var(--bg-tertiary)]/50 px-2 py-1.5 text-xs text-[var(--text-primary)] placeholder-[var(--text-muted)] focus:border-[var(--accent-primary)] focus:outline-none"
                    />
                    <Show when={customRules().length > 1}>
                      <button
                        type="button"
                        onClick={() => removeRuleRow(idx())}
                        class="rounded p-1 text-[var(--text-tertiary)] hover:bg-red-500/10 hover:text-red-400"
                      >
                        <svg class="h-3.5 w-3.5" viewBox="0 0 20 20" fill="currentColor">
                          <path d="M6.28 5.22a.75.75 0 00-1.06 1.06L8.94 10l-3.72 3.72a.75.75 0 101.06 1.06L10 11.06l3.72 3.72a.75.75 0 101.06-1.06L11.06 10l3.72-3.72a.75.75 0 00-1.06-1.06L10 8.94 6.28 5.22z"/>
                        </svg>
                      </button>
                    </Show>
                  </div>
                )}
              </For>
            </div>
          </div>

          <div>
            <label class="text-xs font-medium text-[var(--text-tertiary)]">ASN Numbers (optional)</label>
            <input
              type="text"
              placeholder="e.g. 2906, 15169 — for IP-based routing precision"
              value={customASNs()}
              onInput={(e) => setCustomASNs(e.currentTarget.value)}
              class="mt-1 w-full rounded-lg border border-[var(--border-default)] bg-[var(--bg-tertiary)]/50 px-3 py-2 text-sm text-[var(--text-primary)] placeholder-[var(--text-muted)] focus:border-[var(--accent-primary)] focus:outline-none"
            />
            <p class="mt-1 text-[10px] text-[var(--text-tertiary)]">
              Comma-separated AS numbers. IP ranges will be resolved automatically for precise routing.
            </p>
          </div>

          <div>
            <label class="text-xs font-medium text-[var(--text-tertiary)]">Note (optional)</label>
            <input
              type="text"
              placeholder="e.g. Custom game server ports"
              value={customNote()}
              onInput={(e) => setCustomNote(e.currentTarget.value)}
              class="mt-1 w-full rounded-lg border border-[var(--border-default)] bg-[var(--bg-tertiary)]/50 px-3 py-2 text-sm text-[var(--text-primary)] placeholder-[var(--text-muted)] focus:border-[var(--accent-primary)] focus:outline-none"
            />
          </div>
        </div>

        <div class="mt-5 flex justify-end gap-2">
          <button
            type="button"
            onClick={props.onClose}
            class="rounded-lg border border-[var(--border-default)] bg-[var(--bg-tertiary)] px-4 py-2 text-[13px] font-medium text-[var(--text-secondary)] hover:bg-[var(--bg-hover)]"
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={() => void saveCustomProfile()}
            disabled={customSaving()}
            class="rounded-lg bg-[var(--accent-primary)] px-4 py-2 text-[13px] font-semibold text-[var(--bg-primary)] shadow-lg shadow-[var(--accent-primary)]/20 transition-all hover:brightness-110 disabled:opacity-50"
          >
            {customSaving() ? "Saving..." : "Add Service"}
          </button>
        </div>
      </div>
    </div>
  );
}

export default CustomProfileModal;
