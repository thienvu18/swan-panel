import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', 'src');
const read = (path) => readFileSync(join(root, path), 'utf8');

const protectedPages = {
  'UsersPage.tsx': ['confirm.grant_admin_title', 'confirm.remove_admin_title', 'confirm.create_admin_title', 'confirm.delete_user_title', 'confirm.delete_group_title', 'confirm.reject_user_title', 'confirm.approve_user_title'],
  'ResourcesPage.tsx': ['confirm.revoke_title', 'confirm.delete_subnet_title'],
  'DashboardPage.tsx': ['confirm.disconnect_title', 'confirm.revoke_title'],
  'AccessRulesPage.tsx': ['confirm.delete_rule_title', 'confirm.create_rule_title'],
  'DNSEnginePage.tsx': ['confirm.delete_dns_title'],
  'OIDCPage.tsx': ['confirm.delete_provider_title'],
  'SystemLogsPage.tsx': ['confirm.stop_gateway_title', 'confirm.restart_gateway_title'],
  'ServerSettingsPage.tsx': ['confirm.rotate_ca_title', 'confirm.apply_advanced_title'],
};

test('all high-impact actions require a shared confirmation dialog', () => {
  for (const [page, markers] of Object.entries(protectedPages)) {
    const source = read('pages/' + page);
    assert.match(source, /useConfirmAction\(\)/, page + ' missing the confirmation hook');
    assert.match(source, /\{confirmationDialog\}/, page + ' missing the dialog mount');
    assert.doesNotMatch(source, /\b(?:window\.)?confirm\s*\(/, page + ' still uses a native confirm');
    for (const marker of markers) {
      assert.ok(source.includes(marker), page + ' missing confirmation: ' + marker);
    }
  }
});

test('confirmation keeps failed operations visible and blocks duplicate submits', () => {
  const source = read('components/ConfirmDialog.tsx');
  assert.match(source, /await action\.onConfirm\(\)/);
  assert.match(source, /if \(!action \|\| inFlight\.current\) return/);
  assert.match(source, /setError\(cause instanceof Error/);
  assert.match(source, /dialogRole="alertdialog"/);
  assert.match(source, /disabled=\{isPending\}/);
  const modal = read('components/Modal.tsx');
  assert.match(modal, /aria-modal="true"/);
  assert.match(modal, /openModalStack/);
  assert.match(modal, /stopImmediatePropagation/);
  assert.match(modal, /e\.key === 'Tab'/, 'the modal must trap keyboard focus');
  assert.match(modal, /opener\.focus\(\)/, 'closing must restore focus to the triggering control');
  assert.match(source, /data-initial-focus/, 'confirmation must focus Cancel first');
});

test('all native select fields use the same styled surface control', () => {
  let selects = 0;
  for (const page of Object.keys(protectedPages)) {
    const source = read('pages/' + page);
    for (const match of source.matchAll(/<select\b[\s\S]*?<\/select>/g)) {
      selects++;
      assert.match(match[0], /className=/, 'missing select styling on ' + page);
      assert.ok(match[0].includes('surface-input') || match[0].includes('{inputClass}'), 'non-standard select in ' + page);
    }
  }
  assert.ok(selects >= 8, 'the select audit unexpectedly skipped controls');
  const css = read('index.css');
  assert.match(css, /select\.surface-input\s*\{/);
  assert.match(css, /appearance:\s*none/);
  assert.match(css, /background-image:\s*url/);
  assert.match(css, /select\.surface-input:focus-visible/);
  assert.match(css, /html\.dark select\.surface-input option/);
  assert.match(css, /html:not\(\.dark\) select\.surface-input option/);
});

test('confirmation translations are complete in Vietnamese and English', () => {
  const en = JSON.parse(readFileSync(join(root, 'locales/en.json'), 'utf8'));
  const vi = JSON.parse(readFileSync(join(root, 'locales/vi.json'), 'utf8'));
  for (const marker of Object.values(protectedPages).flat()) {
    const key = marker.slice('confirm.'.length);
    assert.ok(en.confirm[key]?.length > 5, 'missing English ' + marker);
    assert.ok(vi.confirm[key]?.length > 5, 'missing Vietnamese ' + marker);
  }
  for (const key of ['delete', 'cancel', 'grant_admin', 'remove_admin', 'revoke']) {
    assert.ok(en.common[key] && vi.common[key], 'missing action label ' + key);
  }
});

test('dangerous actions cannot accidentally execute through old direct mutation handlers', () => {
  const forbidden = [
    [/deleteRuleMutation\.mutate\(/, 'AccessRulesPage.tsx'],
    [/createRuleMutation\.mutate\(/, 'AccessRulesPage.tsx'],
    [/deleteOverrideMutation\.mutate\(/, 'DNSEnginePage.tsx'],
    [/deleteProviderMutation\.mutate\(/, 'OIDCPage.tsx'],
    [/toggleRoleMutation\.mutate\(/, 'UsersPage.tsx'],
    [/revokeDeviceMutation\.mutate\(/, 'DashboardPage.tsx'],
    [/disconnectMutation\.mutate\(/, 'DashboardPage.tsx'],
    [/rotate\.mutate\(/, 'ServerSettingsPage.tsx'],
  ];
  for (const [pattern, page] of forbidden) {
    assert.doesNotMatch(read('pages/' + page), pattern, page + ' bypasses confirmation');
  }
});

test('host strongSwan mode never exposes unavailable daemon controls', () => {
  const page = read('pages/SystemLogsPage.tsx');
  assert.match(page, /control_supported === false/);
  assert.match(page, /logs\.host_control_unavailable/);
  assert.match(page, /role="note"/);
  const en = JSON.parse(read('locales/en.json'));
  const vi = JSON.parse(read('locales/vi.json'));
  assert.ok(en.logs.host_control_unavailable?.length > 25);
  assert.ok(vi.logs.host_control_unavailable?.length > 25);
});
