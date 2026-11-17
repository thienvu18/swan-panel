import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const src = join(dirname(fileURLToPath(import.meta.url)), '..', 'src');
const read = (name) => readFileSync(join(src, name), 'utf8');

test('Android native setup stays app-free and removes ADB and ZIP actions', () => {
  const dialog = read('components/AndroidNativeSetupDialog.tsx');
  const api = read('lib/androidSetup.ts');
  assert.match(dialog, /fetchAndroidNativeSetup/);
  assert.match(dialog, /downloadAndroidCredential\(setup\.client_p12/);
  assert.match(dialog, /downloadAndroidCredential\(setup\.root_ca/);
  assert.match(dialog, /setup\.import_password/);
  assert.match(api, /config\/android-native/);
  assert.match(api, /cache: 'no-store'/);
  assert.match(api, /Authorization: `Bearer \$\{token\}`/);
  for (const page of ['ResourcesPage.tsx', 'DashboardPage.tsx']) {
    const source = read('pages/' + page);
    assert.match(source, /AndroidNativeSetupDialog/);
    assert.doesNotMatch(source, /\.sswan/);
    assert.doesNotMatch(source, /handleDownload(?:Profile)?\([^\n]*'android-native'/);
  }
});
test('native Android field labels and safe-use instructions are localized', () => {
  const en = JSON.parse(read('locales/en.json'));
  const vi = JSON.parse(read('locales/vi.json'));
  const names = [
    'android_setup_title', 'android_setup_intro', 'android_step_certificate',
    'android_download_p12', 'android_import_password', 'android_step_connection',
    'android_server_address', 'android_server_identity', 'android_client_identity',
    'android_step_ca', 'android_download_ca', 'android_ca_note',
    'android_security_note', 'android_oem_note',
  ];
  for (const name of names) {
    assert.ok(en.resources[name]?.length > 5, 'missing English ' + name);
    assert.ok(vi.resources[name]?.length > 5, 'missing Vietnamese ' + name);
  }
});
