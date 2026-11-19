import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { formatDuration, availableMetric } from '../src/lib/telemetry.ts';
import { previewSubnet, subnetOverlaps } from '../src/lib/subnetPreview.ts';
import { checkGatewayJSON, diffGatewayJSON } from '../src/lib/gatewayJSON.ts';

test('session duration is compact, human-readable, and retains unavailable states', () => {
  assert.equal(formatDuration('8100s', 'Unavailable'), '2h 15m');
  assert.equal(formatDuration(90061, 'Unavailable'), '1d 1h');
  assert.equal(formatDuration('0s', 'Unavailable'), '0s');
  assert.equal(formatDuration(undefined, 'Unavailable'), 'Unavailable');
  assert.equal(formatDuration('Active', 'Unavailable'), 'Unavailable');
  assert.equal(availableMetric(0, 'Unavailable'), 'Unavailable');
  assert.equal(availableMetric('0 MB', 'Unavailable'), 'Unavailable');
  assert.equal(availableMetric('128 MB', 'Unavailable'), '128 MB');
});

test('CIDR preview matches gateway reserved-address calculation and finds overlaps', () => {
  assert.equal(previewSubnet('10.122.0.19/24')?.normalized, '10.122.0.0/24');
  assert.equal(previewSubnet('10.122.0.0/24')?.capacity, 253);
  assert.equal(previewSubnet('10.0.0.0/30')?.capacity, 1);
  assert.equal(previewSubnet('10.0.0.0/31'), null);
  assert.equal(previewSubnet('10.0.0.256/24'), null);
  assert.equal(previewSubnet('10.0.0.0/7'), null);
  assert.equal(subnetOverlaps(previewSubnet('10.122.1.0/24'), previewSubnet('10.122.0.0/16')), true);
  assert.equal(subnetOverlaps(previewSubnet('10.123.0.0/24'), previewSubnet('10.122.0.0/16')), false);
});

test('advanced JSON rejects unsupported fields and wrong primitive types', () => {
  assert.deepEqual(checkGatewayJSON({ cipher_preset: 'balanced', mobike_enabled: true, advanced_child: {} }), null);
  assert.deepEqual(checkGatewayJSON({ imaginary_option: true }), { field: 'imaginary_option', expected: 'supported' });
  assert.deepEqual(checkGatewayJSON({ mobike_enabled: 'false' }), { field: 'mobike_enabled', expected: 'boolean' });
  assert.deepEqual(checkGatewayJSON({ mss_clamping: 200 }), { field: 'mss_clamping', expected: '0 or 500–8960' });
  assert.equal(diffGatewayJSON({ mobike_enabled: true, vpn_name: 'VPN' }, { mobike_enabled: false, vpn_name: 'VPN' }).length, 1);
});

test('UX regressions: table loading, contextual actions, CA read-only identity, server-authoritative callback', () => {
  const files = Object.fromEntries(['ResourcesPage', 'UsersPage', 'ServerSettingsPage', 'OIDCPage'].map(page => [page,
    readFileSync(new URL(`../src/pages/${page}.tsx`, import.meta.url), 'utf8')]));
  assert.match(files.ResourcesPage, /TableQueryState columns=\{6\}/);
  assert.match(files.UsersPage, /activeSubView === 'directory' \? t\('users.add_user'\) : t\('users.add_group'\)/);
  assert.match(files.ServerSettingsPage, /<dl className="grid/);
  assert.match(files.ServerSettingsPage, /diffGatewayJSON/);
  assert.match(files.OIDCPage, /redirect_uri: string/);
  assert.doesNotMatch(files.OIDCPage, /window\.location\.origin/);
});
