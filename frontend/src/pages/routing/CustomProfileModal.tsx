import { createSignal, For, Show } from "solid-js";
import { apiPost } from "../../lib/api";
import Select from "../../components/Select";
import type { PortRule } from "./types";

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
      <div class="mx-4 w-full max-w-lg rounded-2xl border border-line bg-surface-secondary p-6 shadow-2xl">
        <h2 class="text-lg font-bold text-fg">Add Custom Service</h2>
        <p class="mt-1 text-xs text-fg-tertiary">Define a custom app or service with its protocol and port rules.</p>

        <Show when={customErr()}>
          <div class="mt-3 rounded-lg border border-red-500/30 bg-red-500/10 px-3 py-2 text-xs text-red-300">
            {customErr()}
          </div>
        </Show>

        <div class="mt-4 space-y-3">
          <div>
            <label class="text-xs font-medium text-fg-tertiary">Name</label>
            <input
              type="text"
              placeholder="e.g. My Game Server"
              value={customName()}
              onInput={(e) => setCustomName(e.currentTarget.value)}
              class="mt-1 w-full rounded-lg border border-line bg-surface-tertiary/50 px-3 py-2 text-sm text-fg placeholder-fg-muted focus:border-accent focus:outline-none"
            />
          </div>

          <div>
            <Select
              label="Category"
              value={customCategory()}
              options={[
                { value: "custom", label: "Custom" },
                { value: "gaming", label: "Gaming" },
                { value: "streaming", label: "Streaming" },
                { value: "communication", label: "Communication" },
                { value: "file_sharing", label: "File Sharing" },
                { value: "browsing", label: "Browsing" },
                { value: "remote_access", label: "Remote Access" },
                { value: "home_iot", label: "Home & IoT" },
                { value: "mail", label: "Mail" },
              ]}
              onChange={(v) => setCustomCategory(v)}
            />
          </div>

          <div>
            <div class="flex items-center justify-between">
              <label class="text-xs font-medium text-fg-tertiary">Port Rules</label>
              <button
                type="button"
                onClick={addRuleRow}
                class="text-xs text-success hover:text-success"
              >
                + Add rule
              </button>
            </div>
            <div class="mt-2 space-y-2">
              <For each={customRules()}>
                {(rule, idx) => (
                  <div class="flex items-center gap-2">
                    <Select
                      value={rule.protocol}
                      options={[
                        { value: "TCP", label: "TCP" },
                        { value: "UDP", label: "UDP" },
                        { value: "TCP/UDP", label: "TCP/UDP" },
                      ]}
                      onChange={(v) => updateRuleRow(idx(), "protocol", v)}
                      class="w-28"
                    />
                    <input
                      type="text"
                      placeholder="e.g. 8080 or 3000-3100"
                      value={rule.ports}
                      onInput={(e) => updateRuleRow(idx(), "ports", e.currentTarget.value)}
                      class="flex-1 rounded-lg border border-line bg-surface-tertiary/50 px-2 py-1.5 text-xs text-fg placeholder-fg-muted focus:border-accent focus:outline-none"
                    />
                    <Show when={customRules().length > 1}>
                      <button
                        type="button"
                        onClick={() => removeRuleRow(idx())}
                        class="rounded p-1 text-fg-tertiary hover:bg-red-500/10 hover:text-red-400"
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
            <label class="text-xs font-medium text-fg-tertiary">ASN Numbers (optional)</label>
            <input
              type="text"
              placeholder="e.g. 2906, 15169 — for IP-based routing precision"
              value={customASNs()}
              onInput={(e) => setCustomASNs(e.currentTarget.value)}
              class="mt-1 w-full rounded-lg border border-line bg-surface-tertiary/50 px-3 py-2 text-sm text-fg placeholder-fg-muted focus:border-accent focus:outline-none"
            />
            <p class="mt-1 text-[10px] text-fg-tertiary">
              Comma-separated AS numbers. IP ranges will be resolved automatically for precise routing.
            </p>
          </div>

          <div>
            <label class="text-xs font-medium text-fg-tertiary">Note (optional)</label>
            <input
              type="text"
              placeholder="e.g. Custom game server ports"
              value={customNote()}
              onInput={(e) => setCustomNote(e.currentTarget.value)}
              class="mt-1 w-full rounded-lg border border-line bg-surface-tertiary/50 px-3 py-2 text-sm text-fg placeholder-fg-muted focus:border-accent focus:outline-none"
            />
          </div>
        </div>

        <div class="mt-5 flex justify-end gap-2">
          <button
            type="button"
            onClick={props.onClose}
            class="rounded-lg border border-line bg-surface-tertiary px-4 py-2 text-[13px] font-medium text-fg-secondary hover:bg-hover"
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={() => void saveCustomProfile()}
            disabled={customSaving()}
            class="rounded-lg bg-accent px-4 py-2 text-[13px] font-semibold text-surface shadow-lg shadow-accent/20 transition-all hover:brightness-110 disabled:opacity-50"
          >
            {customSaving() ? "Saving..." : "Add Service"}
          </button>
        </div>
      </div>
    </div>
  );
}

export default CustomProfileModal;
