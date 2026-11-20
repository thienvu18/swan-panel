import test from 'node:test';
import assert from 'node:assert/strict';
import { effectiveGatewayStatus, GATEWAY_HEALTH_MAX_AGE_MS } from '../src/lib/gatewayHealth.ts';

const now = Date.parse('2026-11-20T04:00:00.000Z');
const checked_at = new Date(now - 15_000).toISOString();

test('fresh verified gateway evidence preserves the four states', () => {
  for (const status of ['online', 'degraded', 'offline', 'unknown']) {
    assert.equal(effectiveGatewayStatus({ status, checked_at }, now), status);
  }
});

test('missing, invalid, expired and future snapshots never remain online', () => {
  assert.equal(effectiveGatewayStatus(undefined, now), 'unknown');
  assert.equal(effectiveGatewayStatus({ status: 'online', checked_at: 'broken' }, now), 'unknown');
  assert.equal(effectiveGatewayStatus({ status: 'online', checked_at: new Date(now - GATEWAY_HEALTH_MAX_AGE_MS - 1).toISOString() }, now), 'unknown');
  assert.equal(effectiveGatewayStatus({ status: 'online', checked_at: new Date(now + 5_001).toISOString() }, now), 'unknown');
  assert.equal(effectiveGatewayStatus({ status: 'invalid', checked_at }, now), 'unknown');
  assert.equal(effectiveGatewayStatus({ status: 'online', checked_at: new Date(now - GATEWAY_HEALTH_MAX_AGE_MS).toISOString() }, now), 'online');
});
