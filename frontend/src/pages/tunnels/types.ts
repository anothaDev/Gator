export type TunnelOwnershipStatus = "local_only" | "managed_pending" | "managed_verified" | "managed_drifted" | "needs_reimport";

export type TunnelStatus = {
  id: number;
  name: string;
  description: string;
  remote_host: string;
  tunnel_subnet: string;
  firewall_ip: string;
  remote_ip: string;
  listen_port: number;
  remote_wg_interface: string;
  deployed: boolean;
  status: string;
  handshake?: string;
  transfer_rx?: string;
  transfer_tx?: string;
  remote_reachable: boolean;
  created_at: string;
  ownership_status?: TunnelOwnershipStatus;
  drift_reason?: string;
  last_verified_at?: string;
};

export type TunnelDetail = TunnelStatus & {
  ssh_port: number;
  ssh_user: string;
  has_ssh_key: boolean;
  has_ssh_password: boolean;
  firewall_public_key?: string;
  remote_public_key?: string;
  keepalive: number;
};

export type SubnetSuggestion = {
  tunnel_subnet: string;
  firewall_ip: string;
  remote_ip: string;
};

export type TunnelForm = {
  name: string;
  description: string;
  remote_host: string;
  ssh_port: number;
  ssh_user: string;
  ssh_private_key: string;
  ssh_password: string;
  tunnel_subnet: string;
  firewall_ip: string;
  remote_ip: string;
  listen_port: number;
  keepalive: number;
};

export type DiscoveredTunnel = {
  type: string;
  server_uuid: string;
  server_name: string;
  peer_uuid: string;
  peer_name: string;
  local_cidr: string;
  remote_cidr: string;
  endpoint: string;
  peer_pubkey: string;
  listen_port: string;
  wg_device: string;
  wg_iface: string;
  iface_desc: string;
  gateway_uuid: string;
  gateway_name: string;
  gateway_ip: string;
};

export type DeployStep = {
  label: string;
  doneLabel: string;
  step: string;
  status: "pending" | "running" | "done" | "error";
  error?: string;
  result?: Record<string, unknown>;
};

export const emptyForm: TunnelForm = {
  name: "",
  description: "",
  remote_host: "",
  ssh_port: 22,
  ssh_user: "root",
  ssh_private_key: "",
  ssh_password: "",
  tunnel_subnet: "",
  firewall_ip: "",
  remote_ip: "",
  listen_port: 51820,
  keepalive: 25,
};
