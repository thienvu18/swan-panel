/** A display-only formatter. The raw gateway value remains available as a tooltip. */
export function formatDuration(value: string | number | null | undefined, unavailable: string): string {
  if (value == null || value === '') return unavailable;
  const raw = String(value).trim();
  const seconds = typeof value === 'number' ? value
    : /^\d+(?:\.\d+)?s?$/.test(raw) ? Number(raw.replace(/s$/, '')) : NaN;
  if (!Number.isFinite(seconds) || seconds < 0) return raw === 'Active' ? unavailable : raw;
  let remaining = Math.floor(seconds);
  const units: Array<[string, number]> = [['d', 86400], ['h', 3600], ['m', 60], ['s', 1]];
  const parts: string[] = [];
  for (const [label, size] of units) {
    const count = Math.floor(remaining / size);
    if (count || (label === 's' && parts.length === 0)) parts.push(`${count}${label}`);
    remaining %= size;
    if (parts.length === 2) break;
  }
  return parts.join(' ');
}

export function availableMetric(value: string | number | null | undefined, unavailable: string): string {
  const raw = value == null ? '' : String(value).trim();
  if (!raw || raw === '0' || /^0(?:\.0+)?\s*(?:MB|KB|B)$/i.test(raw)) return unavailable;
  return raw;
}
