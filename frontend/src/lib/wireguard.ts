export type ParsedWireGuardConfig = {
  deviceName: string;
  interfaceAddress: string;
  interfaceDNS: string;
  privateKey: string;
  peerAllowedIPs: string;
  peerPublicKey: string;
  endpoint: string;
  preSharedKey: string;
};

export function wireGuardStemFromFile(fileName: string): string {
  return fileName.replace(/\.[^/.]+$/, "").trim();
}

export function parseWireGuardConfig(content: string): ParsedWireGuardConfig {
  let section = "";
  let deviceName = "";
  let interfaceAddress = "";
  let interfaceDNS = "";
  let privateKey = "";
  let peerAllowedIPs = "";
  let peerPublicKey = "";
  let endpoint = "";
  let preSharedKey = "";

  for (const line of content.split(/\r?\n/)) {
    const trimmed = line.trim();
    if (trimmed === "") continue;

    const deviceMatch = trimmed.match(/^#\s*Device:\s*(.+)$/i);
    if (deviceMatch) {
      deviceName = deviceMatch[1].trim();
      continue;
    }

    if (trimmed.startsWith("[") && trimmed.endsWith("]")) {
      section = trimmed.slice(1, -1).trim().toLowerCase();
      continue;
    }

    if (trimmed.startsWith("#") || trimmed.startsWith(";")) continue;

    const idx = trimmed.indexOf("=");
    if (idx < 0) continue;

    const key = trimmed.slice(0, idx).trim().toLowerCase();
    const value = trimmed.slice(idx + 1).trim();

    if (section === "interface" && key === "address") interfaceAddress = value;
    if (section === "interface" && key === "dns") interfaceDNS = value;
    if (section === "interface" && key === "privatekey") privateKey = value;
    if (section === "peer" && key === "allowedips") peerAllowedIPs = value;
    if (section === "peer" && key === "publickey") peerPublicKey = value;
    if (section === "peer" && key === "endpoint") endpoint = value;
    if (section === "peer" && (key === "presharedkey" || key === "preshared_key")) {
      preSharedKey = value;
    }
  }

  return {
    deviceName,
    interfaceAddress,
    interfaceDNS,
    privateKey,
    peerAllowedIPs,
    peerPublicKey,
    endpoint,
    preSharedKey,
  };
}
