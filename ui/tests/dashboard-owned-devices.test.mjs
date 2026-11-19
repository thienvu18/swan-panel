import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

test('admin User View requests only devices owned by the active user', () => {
  const dashboard = readFileSync(new URL('../src/pages/DashboardPage.tsx', import.meta.url), 'utf8');
  const app = readFileSync(new URL('../src/App.tsx', import.meta.url), 'utf8');
  assert.match(dashboard, /\?user_id=\$\{encodeURIComponent\(currentUserID\)\}/);
  assert.match(dashboard, /enabled: role === 'user' && \(!isAdmin \|\| Boolean\(currentUserID\)\)/);
  assert.match(app, /currentUserID=\{currentUser\?\.id \?\? ''\}/);
});

test('Android import passwords are concealed by default and copyable on demand', () => {
  const dialog = readFileSync(new URL('../src/components/AndroidNativeSetupDialog.tsx', import.meta.url), 'utf8');
  assert.match(dialog, /useState\(false\)/);
  assert.match(dialog, /showImportPassword \? setup\.import_password :/);
  assert.match(dialog, /CopyButton value=\{setup\.import_password\}/);
});
