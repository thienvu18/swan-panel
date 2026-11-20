export type GatewayStatus = 'online' | 'degraded' | 'offline' | 'unknown';

export interface GatewayHealth {
  status: GatewayStatus;
  checked_at: string;
}

export const GATEWAY_HEALTH_MAX_AGE_MS = 45_000;

// The UI must never keep a green gateway indicator after fresh evidence stops.
export function effectiveGatewayStatus(health: GatewayHealth | null | undefined, now = Date.now()): GatewayStatus {
  if (!health) return 'unknown';
  if (!['online', 'degraded', 'offline', 'unknown'].includes(health.status)) return 'unknown';
  const checked = Date.parse(health.checked_at);
  if (!Number.isFinite(checked) || checked > now + 5_000 || now - checked > GATEWAY_HEALTH_MAX_AGE_MS) return 'unknown';
  return health.status;
}
