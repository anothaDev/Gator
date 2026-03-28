// Shared types for the routing feature (Routing, AppCard, RoutingToolbar, CustomProfileModal).

export type PortRule = {
  protocol: string;
  ports: string;
};

export type URLTableHint = {
  download_url: string;
  jq_filter: string;
  description: string;
  filename: string;
};

export type AppProfile = {
  id: string;
  name: string;
  icon: string;
  category: string;
  rules: PortRule[];
  asns?: number[];
  url_table_hint?: URLTableHint;
  note?: string;
  is_custom?: boolean;
};

export type AppPreset = {
  id: string;
  name: string;
  description: string;
  vpn_on?: string[];
  vpn_off?: string[];
};

export type CategoryInfo = {
  key: string;
  label: string;
  enabledCount: number;
};
