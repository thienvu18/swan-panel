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

test('compact layouts keep translated navigation, capacity and mobile controls readable', () => {
  const source = (file) => readFileSync(new URL(`../src/${file}`, import.meta.url), 'utf8');
  const sidebar = source('components/Sidebar.tsx');
  const resources = source('pages/ResourcesPage.tsx');
  const android = source('components/AndroidNativeSetupDialog.tsx');
  const access = source('components/EffectiveAccessPanel.tsx');
  const vietnamese = JSON.parse(source('locales/vi.json'));
  const english = JSON.parse(source('locales/en.json'));

  assert.match(sidebar, /nav\.resources_short/);
  assert.equal(vietnamese.nav.resources_short, 'Tài nguyên');
  assert.equal(english.nav.resources_short, 'Resources');
  assert.match(resources, /w-48 font-sans/);
  assert.match(resources, /whitespace-nowrap tabular-nums/);
  assert.match(android, /sm:flex-row sm:items-center sm:justify-between/);
  assert.match(android, /overflow-x-auto whitespace-nowrap/);
  assert.match(access, /flex-col gap-2 items-stretch sm:flex-row/);
  assert.match(access, /justify-center gap-1\.5 whitespace-nowrap w-full sm:w-auto/);
});

test('management page actions reflow without clipping at mobile and tablet widths', () => {
  for (const page of ['ResourcesPage', 'UsersPage', 'OIDCPage']) {
    const source = readFileSync(new URL(`../src/pages/${page}.tsx`, import.meta.url), 'utf8');
    assert.match(source, /flex flex-col items-stretch gap-3 lg:flex-row/, page);
    assert.match(source, /flex flex-col sm:flex-row items-stretch gap-2 w-full min-w-0 lg:max-w/, page);
    assert.match(source, /relative flex-1 min-w-0/, page);
    assert.match(source, /justify-center gap-1\.5 shadow-xs text-xs shrink-0 whitespace-nowrap w-full sm:w-auto/, page);
  }
  const android = readFileSync(new URL('../src/components/AndroidNativeSetupDialog.tsx', import.meta.url), 'utf8');
  assert.match(android, /title=\{t\('resources\.android_setup_title'\)\}/);
  assert.match(android, /t\('resources\.device_hostname'\)/);
});

test('Users tabs stack evenly on narrow phones and authentication badges remain single-line', () => {
  const users = readFileSync(new URL('../src/pages/UsersPage.tsx', import.meta.url), 'utf8');
  assert.match(users, /flex flex-col sm:flex-row self-stretch sm:self-start lg:self-auto/);
  assert.equal((users.match(/w-full sm:w-auto text-center px-3 py-1\.5/g) || []).length, 2);
  assert.match(users, /inline-flex items-center whitespace-nowrap border subtle-border bg-slate-100/);
});

test('global header shows no SSE health pill while Logs keeps stream feedback in context', () => {
  const app = readFileSync(new URL('../src/App.tsx', import.meta.url), 'utf8');
  const header = readFileSync(new URL('../src/components/Header.tsx', import.meta.url), 'utf8');
  const logs = readFileSync(new URL('../src/pages/SystemLogsPage.tsx', import.meta.url), 'utf8');
  const sidebar = readFileSync(new URL('../src/components/Sidebar.tsx', import.meta.url), 'utf8');

  assert.doesNotMatch(header, /isRealtimeConnected|nav\.live|nav\.reconnecting/);
  const headerUsage = app.match(/<Header\b[\s\S]*?\/>/)?.[0];
  assert.ok(headerUsage);
  assert.doesNotMatch(headerUsage, /isRealtimeConnected=/);
  assert.match(app, /<SystemLogsPage isRealtimeConnected=\{isConnected\}/);
  assert.match(logs, /isRealtimeConnected \? t\('logs\.live_events'\)/);
  assert.match(sidebar, /<GatewayStatus \/>/);
  assert.match(header, /<GatewayStatus compact \/>/);
});

test('Dashboard reports unavailable sessions instead of false zero when gateway runtime is down', () => {
  const dashboard = readFileSync(new URL('../src/pages/DashboardPage.tsx', import.meta.url), 'utf8');
  const english = JSON.parse(readFileSync(new URL('../src/locales/en.json', import.meta.url), 'utf8'));
  const vietnamese = JSON.parse(readFileSync(new URL('../src/locales/vi.json', import.meta.url), 'utf8'));
  assert.match(dashboard, /active_sessions: number \| null/);
  assert.match(dashboard, /active_sessions: null/);
  assert.match(dashboard, /isStatsError \|\| stats\.active_sessions === null \? '—'/);
  assert.doesNotMatch(english.dashboard.auto_refresh_hint, /SSE|Zero Downtime/);
  assert.doesNotMatch(vietnamese.dashboard.auto_refresh_hint, /SSE|Zero Downtime/);
});
