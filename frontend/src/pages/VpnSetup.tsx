import { createResource, createSignal, For, onMount, Show } from "solid-js";
import VPNCard from "./vpn-setup/VPNCard";
import DiscoveryModal from "./vpn-setup/DiscoveryModal";
import Card from "../components/Card";
import Button from "../components/Button";
import AlertBanner from "../components/AlertBanner";
import OpnsenseLink from "../components/OpnsenseLink";
import EmptyState from "../components/EmptyState";
import LegacyRulesWarning from "../components/LegacyRulesWarning";
import { SkeletonList } from "../components/Skeleton";
import { apiGet } from "../lib/api";
import { createLegacyCheck } from "../lib/legacy";
import { parseWireGuardConfig as parseWireGuardFields, wireGuardStemFromFile } from "../lib/wireguard";

// ─── Types needed by the page shell ──────────────────────────────

type VPNStatus = {
  id: number;
  name: string;
  protocol: "wireguard" | "openvpn";
  ip_version: "ipv4" | "ipv6";
  local_cidr?: string;
  remote_cidr?: string;
  endpoint?: string;
  enabled: boolean;
  has_private_key: boolean;
  has_peer_public_key: boolean;
  has_pre_shared_key: boolean;
  applied: boolean;
  policy_applied: boolean;
  routing_applied: boolean;
  gateway_online: boolean;
  gateway_applied: boolean;
  nat_applied: boolean;
  source_interfaces?: string[];
  wg_device?: string;
  wg_interface?: string;
  interface_assigned: boolean;
  gateway_name?: string;
  last_applied_at?: string;
  ownership_status?: "local_only" | "managed_pending" | "managed_verified" | "managed_drifted" | "needs_reimport";
  drift_reason?: string;
  last_verified_at?: string;
};

type VPNForm = {
  name: string;
  protocol: "wireguard" | "openvpn";
  ipVersion: "ipv4" | "ipv6";
  localCIDR: string;
  remoteCIDR: string;
  endpoint: string;
  dns: string;
  privateKey: string;
  peerPublicKey: string;
  preSharedKey: string;
};

type VPNListResponse = { vpns: VPNStatus[] };

// ─── Helpers ─────────────────────────────────────────────────────

async function fetchVPNList(): Promise<VPNStatus[]> {
  const { ok, data } = await apiGet<VPNListResponse & { error?: string }>("/api/vpn/configs");
  if (!ok) throw new Error(data.error ?? "Failed to load VPN configs");
  return data.vpns ?? [];
}

function buildWireGuardDraft(content: string, fileName: string): Partial<VPNForm> {
  const parsed = parseWireGuardFields(content);
  const draft: Partial<VPNForm> = {
    protocol: "wireguard",
    localCIDR: parsed.interfaceAddress,
    remoteCIDR: parsed.peerAllowedIPs,
    endpoint: parsed.endpoint,
    dns: parsed.interfaceDNS,
    privateKey: parsed.privateKey,
    peerPublicKey: parsed.peerPublicKey,
    preSharedKey: parsed.preSharedKey,
  };

  const name = (parsed.deviceName || wireGuardStemFromFile(fileName)).replace(/[^a-zA-Z0-9_-]/g, "_");
  if (name) draft.name = name;

  return draft;
}

// ─── Main page ───────────────────────────────────────────────────

export default function VpnSetup(props: { onNavigate?: (section: string) => void }) {
  const [vpnList, { refetch }] = createResource(fetchVPNList);
  const [expandedId, setExpandedId] = createSignal<number | "new" | null>(null);
  const [newDraft, setNewDraft] = createSignal<Partial<VPNForm> | null>(null);
  const [newDraftNotice, setNewDraftNotice] = createSignal("");
  const [pageError, setPageError] = createSignal("");
  const { legacyCount, legacyChecked, checkLegacyRules } = createLegacyCheck();
  let importInputRef: HTMLInputElement | undefined;

  onMount(() => void checkLegacyRules());

  const toggle = (id: number) => setExpandedId((prev) => (prev === id ? null : id));
  const openNew = () => {
    setPageError("");
    setNewDraft(null);
    setNewDraftNotice("");
    setExpandedId("new");
  };

  const handleCreated = (newId?: number) => {
    setNewDraft(null);
    setNewDraftNotice("");
    setExpandedId(newId ?? null);
    void refetch();
  };

  const triggerImport = () => {
    setPageError("");
    importInputRef?.click();
  };

  const handleTopLevelImport = async (e: Event) => {
    setPageError("");
    const input = e.currentTarget as HTMLInputElement;
    const file = input.files?.[0];
    if (!file) return;
    try {
      const content = await file.text();
      const parsed = buildWireGuardDraft(content, file.name);
      setNewDraft(parsed);
      setNewDraftNotice(`Imported ${file.name}. Review the values, then create the profile.`);
      setExpandedId("new");
    } catch (err) {
      setPageError(err instanceof Error ? err.message : "Failed to parse config file.");
    } finally {
      input.value = "";
    }
  };

  const activeVpnName = () => {
    const list = vpnList() ?? [];
    const active = list.find((v) => v.routing_applied);
    return active?.name;
  };

  // Discovery state
  const [showDiscovery, setShowDiscovery] = createSignal(false);
  const [readoptTarget, setReadoptTarget] = createSignal<{
    id: number; name: string; endpoint?: string; wg_device?: string; gateway_name?: string;
  } | null>(null);

  return (
    <div class="space-y-5">
      <input
        ref={importInputRef}
        type="file"
        accept=".conf,.wg,.txt"
        onChange={handleTopLevelImport}
        class="hidden"
      />

      {/* Header */}
      <div class="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
        <div>
          <h1 class="text-2xl font-semibold tracking-tight text-fg">
            VPN
          </h1>
          <p class="mt-1 text-sm text-fg-tertiary">
            Manage WireGuard VPN profiles, deploy them to OPNsense, and switch which one actively routes traffic.
          </p>
        </div>
        <div class="flex flex-wrap items-center gap-2">
          <OpnsenseLink path="/ui/wireguard/general" label="WireGuard" />
          <Button variant="secondary" size="md" onClick={() => setShowDiscovery(true)}>
            <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <circle cx="11" cy="11" r="8" />
              <path d="m21 21-4.35-4.35" />
            </svg>
            Scan OPNsense
          </Button>
          <Button variant="primary" size="md" onClick={triggerImport}>
            <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
              <polyline points="17,8 12,3 7,8" />
              <line x1="12" y1="3" x2="12" y2="15" />
            </svg>
            Import .conf
          </Button>
          <Button
            variant="secondary"
            size="md"
            onClick={openNew}
            disabled={expandedId() === "new" && !newDraft()}
          >
            <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <line x1="12" y1="5" x2="12" y2="19" />
              <line x1="5" y1="12" x2="19" y2="12" />
            </svg>
            Add manually
          </Button>
        </div>
      </div>

      {/* Error Banner */}
      <Show when={pageError()}>
        <AlertBanner tone="error">{pageError()}</AlertBanner>
      </Show>

      {/* Legacy rules warning */}
      <Show when={legacyChecked() && legacyCount() > 0}>
        <LegacyRulesWarning
          count={legacyCount()}
          detail="Gator cannot manage VPN routing until these are migrated."
          onNavigate={props.onNavigate ? () => props.onNavigate!("migration") : undefined}
        />
      </Show>

      {/* Loading State */}
      <Show when={vpnList.loading}>
        <SkeletonList items={2} />
      </Show>

      {/* Error State */}
      <Show when={vpnList.error}>
        <AlertBanner tone="error">Could not load VPN profiles.</AlertBanner>
      </Show>

      {/* New VPN form */}
      <Show when={expandedId() === "new"}>
        <VPNCard
          vpn={null}
          onSaved={handleCreated}
          onCancel={() => {
            setExpandedId(null);
            setNewDraft(null);
            setNewDraftNotice("");
          }}
          refetchList={refetch}
          initialForm={newDraft()}
          initialNotice={newDraftNotice()}
        />
      </Show>

      {/* Existing VPN cards */}
      <Show when={!vpnList.loading && !vpnList.error}>
        <For each={vpnList() ?? []}>
          {(vpn) => (
            <VPNCard
              vpn={vpn}
              expanded={expandedId() === vpn.id}
              onToggle={() => toggle(vpn.id)}
              onSaved={() => void refetch()}
              onDeleted={() => void refetch()}
              onReadopt={() => {
                setReadoptTarget({
                  id: vpn.id, name: vpn.name,
                  endpoint: vpn.endpoint, wg_device: vpn.wg_device,
                  gateway_name: vpn.gateway_name,
                });
                setShowDiscovery(true);
              }}
              legacyRuleCount={legacyCount()}
              refetchList={refetch}
              activeVpnName={activeVpnName()}
            />
          )}
        </For>

        {/* Empty State */}
        <Show when={(vpnList() ?? []).length === 0 && expandedId() !== "new"}>
          <Card variant="elevated" class="overflow-hidden">
            <EmptyState
              variant="vpn"
              title="No VPN profiles yet"
              description="Import a WireGuard configuration file for the fastest setup, or create a profile manually to get started."
              action={
                <div class="flex flex-wrap justify-center gap-3">
                  <Button variant="primary" size="md" onClick={triggerImport}>
                    <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                      <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
                      <polyline points="17,8 12,3 7,8" />
                      <line x1="12" y1="3" x2="12" y2="15" />
                    </svg>
                    Import config
                  </Button>
                  <Button variant="secondary" size="md" onClick={openNew}>
                    Create manually
                  </Button>
                </div>
              }
            />
          </Card>
        </Show>
      </Show>

      {/* Discovery Modal */}
      <Show when={showDiscovery()}>
        <DiscoveryModal
          onClose={() => {
            setShowDiscovery(false);
            setReadoptTarget(null);
          }}
          onImported={() => {
            setShowDiscovery(false);
            setReadoptTarget(null);
            void refetch();
          }}
          readoptId={readoptTarget()?.id}
          readoptName={readoptTarget()?.name}
          readoptEndpoint={readoptTarget()?.endpoint}
          readoptWGDevice={readoptTarget()?.wg_device}
          readoptGatewayName={readoptTarget()?.gateway_name}
        />
      </Show>
    </div>
  );
}
