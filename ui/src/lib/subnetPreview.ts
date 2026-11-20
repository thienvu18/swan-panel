export interface SubnetPreview {
  normalized: string;
  capacity: number;
  start: number;
  end: number;
}

/** Mirrors the gateway IPv4 /8–/30 pool rules; the backend remains authoritative. */
export function previewSubnet(input: string): SubnetPreview | null {
  const match = /^(\d{1,3}(?:\.\d{1,3}){3})\/(\d{1,2})$/.exec(input.trim());
  if (!match) return null;
  const octets = match[1].split('.').map(Number);
  const prefix = Number(match[2]);
  if (prefix < 8 || prefix > 30 || octets.some(o => o > 255)) return null;
  const ip = octets.reduce((acc, o) => (acc * 256 + o) >>> 0, 0);
  const mask = (0xffffffff << (32 - prefix)) >>> 0;
  const start = (ip & mask) >>> 0;
  const end = (start | (~mask >>> 0)) >>> 0;
  const normalized = [24, 16, 8, 0].map(bits => (start >>> bits) & 255).join('.') + `/${prefix}`;
  return { normalized, capacity: 2 ** (32 - prefix) - 3, start, end };
}

export function subnetOverlaps(a: SubnetPreview, b: SubnetPreview): boolean {
  return a.start <= b.end && b.start <= a.end;
}
