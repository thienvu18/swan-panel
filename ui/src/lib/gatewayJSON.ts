const stringFields = [
  'vpn_name', 'server_endpoint', 'server_identity', 'reauth_time', 'rekey_time_ike',
  'rekey_time_child', 'inactivity_timeout', 'fragmentation', 'dpd_delay', 'dpd_action',
  'cipher_preset', 'ike_proposals', 'esp_proposals',
];
const boolFields = ['mobike_enabled', 'force_encap_natt'];
const objectFields = ['advanced_connection', 'advanced_child'];
const allowed = new Set([...stringFields, ...boolFields, ...objectFields, 'mss_clamping']);

export interface GatewayJSONIssue { field: string; expected: string; }

/** Light client-side schema checks. The authoritative policy is validated on the server. */
export function checkGatewayJSON(value: Record<string, unknown>): GatewayJSONIssue | null {
  for (const [field, item] of Object.entries(value)) {
    if (!allowed.has(field)) return { field, expected: 'supported' };
    const expected = stringFields.includes(field) ? 'string' : boolFields.includes(field) ? 'boolean'
      : objectFields.includes(field) ? 'object' : 'number';
    if (expected === 'object' ? item !== null && typeof item === 'object' && !Array.isArray(item)
      : typeof item === expected && (expected !== 'number' || Number.isInteger(item))) continue;
    return { field, expected };
  }
  if ('mss_clamping' in value && typeof value.mss_clamping === 'number'
    && value.mss_clamping !== 0 && (value.mss_clamping < 500 || value.mss_clamping > 8960)) {
    return { field: 'mss_clamping', expected: '0 or 500–8960' };
  }
  return null;
}

export interface GatewayJSONChange { field: string; before: unknown; after: unknown; }

export function diffGatewayJSON(before: Record<string, unknown>, after: Record<string, unknown>): GatewayJSONChange[] {
  return [...new Set([...Object.keys(before), ...Object.keys(after)])].sort().flatMap(field => {
    const was = before[field];
    const now = after[field];
    return JSON.stringify(was) === JSON.stringify(now) ? [] : [{ field, before: was, after: now }];
  });
}
