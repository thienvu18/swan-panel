import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

test('Modal restores focus to its opener and locks background scrolling', () => {
  const modal = readFileSync(new URL('../src/components/Modal.tsx', import.meta.url), 'utf8');
  assert.match(modal, /openModalStack/);
  assert.match(modal, /document\.body\.style\.overflow/);
  assert.match(modal, /opener\.focus\(\)/);
});

test('gateway pending result is not announced as fully enforced', () => {
  for (const page of ['AccessRulesPage', 'ResourcesPage', 'UsersPage']) {
    const source = readFileSync(new URL(`../src/pages/${page}.tsx`, import.meta.url), 'utf8');
    assert.match(source, /runtime_status === 'pending'/, page);
    assert.match(source, /server\.enforcement_pending_notice/, page);
  }
});

test('application and login have recoverable verification failures', () => {
  const app = readFileSync(new URL('../src/App.tsx', import.meta.url), 'utf8');
  const login = readFileSync(new URL('../src/pages/LoginPage.tsx', import.meta.url), 'utf8');
  assert.match(app, /setAuthRetry\(retry => retry \+ 1\)/);
  assert.match(login, /setupStatusError/);
  assert.match(login, /checkSetupStatus\(\)/);
});

test('remote settings refresh preserves unsaved form edits', () => {
  const gateway = readFileSync(new URL('../src/pages/ServerSettingsPage.tsx', import.meta.url), 'utf8');
  const dns = readFileSync(new URL('../src/pages/DNSEnginePage.tsx', import.meta.url), 'utf8');
  assert.match(gateway, /if \(!basicDirty\) setForm/);
  assert.match(gateway, /if \(!advancedDirty\) setAdvancedText/);
  assert.match(dns, /if \(!settingsDirty\.suffix\)/);
  assert.match(dns, /if \(!settingsDirty\.upstreams\)/);
});

test('Access rule subject selectors expose pressed state to keyboard users', () => {
  const access = readFileSync(new URL('../src/pages/AccessRulesPage.tsx', import.meta.url), 'utf8');
  assert.match(access, /aria-pressed=\{subjectType === 'device'\}/);
});

test('Access rule subject label and placeholder follow selected subject type in EN and VI', () => {
  const access = readFileSync(new URL('../src/pages/AccessRulesPage.tsx', import.meta.url), 'utf8');
  const en = JSON.parse(readFileSync(new URL('../src/locales/en.json', import.meta.url), 'utf8'));
  const vi = JSON.parse(readFileSync(new URL('../src/locales/vi.json', import.meta.url), 'utf8'));
  for (const type of ['user_group', 'user', 'device']) {
    const suffix = type === 'user_group' ? 'group' : type;
    const labelKey = `select_subject_${suffix}`;
    const placeholderKey = `${labelKey}_placeholder`;
    assert.ok(access.includes(`${type}: { label: 'access.${labelKey}', placeholder: 'access.${placeholderKey}' }`));
    for (const locale of [en, vi]) {
      assert.ok(locale.access[labelKey], `${labelKey} missing in translation`);
      assert.ok(locale.access[placeholderKey], `${placeholderKey} missing in translation`);
    }
  }
  assert.match(access, /subjectFieldTranslations\[subjectType\]/);
  assert.match(access, /htmlFor="access-rule-subject"/);
  assert.match(access, /id="access-rule-subject"/);
  assert.match(access, /t\(subjectField\.label\)/);
  assert.match(access, /t\(subjectField\.placeholder\)/);
});

test('Gateway status is accessible in desktop footer and mobile header without developer jargon', () => {
  const sidebar = readFileSync(new URL('../src/components/Sidebar.tsx', import.meta.url), 'utf8');
  const header = readFileSync(new URL('../src/components/Header.tsx', import.meta.url), 'utf8');
  const backend = readFileSync(new URL('../../internal/api/handlers/dashboard.go', import.meta.url), 'utf8');
  assert.match(backend, /vici\.GetClient\(""\)\.ListSAs\(ctx\)/);
  assert.match(sidebar, /hidden md:block p-2\.5 text-subtle/);
  assert.match(sidebar, /<GatewayStatus \/>/);
  assert.match(header, /md:hidden[^"]*"/);
  assert.match(header, /<GatewayStatus compact \/>/);
  const gateway = readFileSync(new URL('../src/components/GatewayStatus.tsx', import.meta.url), 'utf8');
  assert.match(gateway, /onClick=\{\(\) => setDetailsOpen\(open => !open\)\}/);
  assert.match(gateway, /aria-expanded=\{detailsOpen\}/);
  assert.match(gateway, /\['gateway_health'\]/);
  assert.doesNotMatch(sidebar, /IKEv2/);
  assert.doesNotMatch(sidebar, /currentUser\.name \|\| 'Admin'/);
  assert.match(sidebar, /currentUser\.name \|\| currentUser\.email \|\| t\('common\.unavailable'\)/);
  assert.match(sidebar, /aria-label=\{t\('common\.close'\)\}/);
});

test('Dashboard sessions table exposes keyboard-scrollable overflow and mobile hint', () => {
  const dashboard = readFileSync(new URL('../src/pages/DashboardPage.tsx', import.meta.url), 'utf8');
  const en = JSON.parse(readFileSync(new URL('../src/locales/en.json', import.meta.url), 'utf8'));
  const vi = JSON.parse(readFileSync(new URL('../src/locales/vi.json', import.meta.url), 'utf8'));
  assert.match(dashboard, /overflow-x-auto[^"\n]*" role="region"[^\n]*tabIndex=\{0\}/);
  assert.match(dashboard, /id="dashboard-session-scroll-hint"/);
  assert.match(dashboard, /aria-describedby="dashboard-session-scroll-hint"/);
  assert.ok(en.dashboard.scroll_table_hint && vi.dashboard.scroll_table_hint);
});


test('Android native onboarding has ordered steps and keeps credential protection prominent', () => {
  const dialog = readFileSync(new URL('../src/components/AndroidNativeSetupDialog.tsx', import.meta.url), 'utf8');
  const en = JSON.parse(readFileSync(new URL('../src/locales/en.json', import.meta.url), 'utf8'));
  const vi = JSON.parse(readFileSync(new URL('../src/locales/vi.json', import.meta.url), 'utf8'));
  assert.match(dialog, /<ol className="list-decimal/);
  assert.match(dialog, /resources\.android_step_finish/);
  assert.match(dialog, /resources\.android_cleanup_note/);
  assert.match(dialog, /showImportPassword \? setup\.import_password : '••••••••••••'/);
  assert.ok(dialog.indexOf('resources.android_security_note') < dialog.indexOf('resources.android_download_p12'));
  for (const locale of [en, vi]) {
    for (const key of ['android_password_masked_hint', 'android_step_finish', 'android_cleanup_note']) {
      assert.ok(locale.resources[key], `${key} missing`);
    }
  }
});

test('Gateway settings use one header save action with saved versus active status', () => {
  const page = readFileSync(new URL('../src/pages/ServerSettingsPage.tsx', import.meta.url), 'utf8');
  assert.match(page, /onRegisterSaveButton\(/);
  assert.match(page, /!activeDirty/);
  assert.match(page, /server\.settings\.save_and_apply/);
  assert.match(page, /server\.settings\.saved_revision/);
  assert.match(page, /server\.settings\.active_revision/);
  assert.match(page, /displayedGatewayStatus === 'pending'/);
  assert.doesNotMatch(page, /onClick=\{submitBasic\}/);
  assert.doesNotMatch(page, /onClick=\{submitAdvanced\}/);
});

test('Effective access highlights the verdict and rule origins before selector lists', () => {
  const panel = readFileSync(new URL('../src/components/EffectiveAccessPanel.tsx', import.meta.url), 'utf8');
  const en = JSON.parse(readFileSync(new URL('../src/locales/en.json', import.meta.url), 'utf8'));
  const vi = JSON.parse(readFileSync(new URL('../src/locales/vi.json', import.meta.url), 'utf8'));
  assert.match(panel, /rule_sources\?/);
  assert.match(panel, /decision\.destination/);
  assert.match(panel, /isValidDestinationIPv4/);
  assert.match(panel, /effective_policy\.invalid_destination/);
  assert.match(panel, /effective_policy\.matched_sources/);
  assert.match(panel, /effective_policy\.preview_note/);
  assert.ok(panel.indexOf('effective_policy.verdict_for') < panel.indexOf('effective_policy.allowed'));
  for (const locale of [en, vi]) {
    for (const key of ['matched_sources', 'source_device', 'source_user', 'source_user_group', 'verdict_for']) {
      assert.ok(locale.effective_policy[key], `${key} missing`);
    }
  }
});
