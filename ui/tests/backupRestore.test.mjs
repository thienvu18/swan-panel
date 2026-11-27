import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { downloadEncryptedBackup, RestoreOutcomeError, uploadRestoreBackup, waitForControlPlaneRestart } from '../src/lib/backupApi.ts';

const uiRoot = join(dirname(fileURLToPath(import.meta.url)), '..', 'src');

function withMockBrowser(fn) {
  const prevStorage = globalThis.localStorage;
  const prevWindow = globalThis.window;
  const prevDocument = globalThis.document;
  const prevCreateObjectURL = URL.createObjectURL;
  const prevRevokeObjectURL = URL.revokeObjectURL;

  const anchor = { href: '', download: '', clicked: false, removed: false, click() { this.clicked = true; }, remove() { this.removed = true; } };
  globalThis.localStorage = { getItem: key => (key === 'token' ? 'admin-jwt' : null) };
  globalThis.window = { setTimeout: fnCall => { fnCall(); return 1; } };
  globalThis.document = {
    createElement: tag => {
      assert.equal(tag, 'a');
      return anchor;
    },
    body: {
      appended: null,
      appendChild(node) { this.appended = node; },
    },
  };
  URL.createObjectURL = () => 'blob:backup-test';
  URL.revokeObjectURL = url => {
    assert.equal(url, 'blob:backup-test');
  };

  return Promise.resolve()
    .then(() => fn(anchor))
    .finally(() => {
      globalThis.localStorage = prevStorage;
      globalThis.window = prevWindow;
      globalThis.document = prevDocument;
      URL.createObjectURL = prevCreateObjectURL;
      URL.revokeObjectURL = prevRevokeObjectURL;
    });
}

test('encrypted backup export uses authenticated POST with no-store cache', async () => {
  await withMockBrowser(async anchor => {
    let seenURL = '';
    let seenInit = null;
    const filename = await downloadEncryptedBackup('correct-horse-battery-staple', async (url, init) => {
      seenURL = url;
      seenInit = init;
      return new Response(new Uint8Array([1, 2, 3, 4]), {
        status: 200,
        headers: {
          'Content-Disposition': 'attachment; filename="swanpanel-backup-2026-11-25.tar.enc"',
        },
      });
    });

    assert.equal(seenURL, '/api/system/backup');
    assert.equal(seenInit.method, 'POST');
    assert.equal(seenInit.cache, 'no-store');
    assert.equal(seenInit.headers.Authorization, 'Bearer admin-jwt');
    assert.equal(seenInit.headers['Content-Type'], 'application/json');
    assert.deepEqual(JSON.parse(seenInit.body), { passphrase: 'correct-horse-battery-staple' });
    assert.equal(filename, 'swanpanel-backup-2026-11-25.tar.enc');
    assert.equal(anchor.clicked, true);
    assert.equal(anchor.download, 'swanpanel-backup-2026-11-25.tar.enc');
  });
});

test('backup restore uploads multipart archive with passphrase and explicit confirmation', async () => {
  const prevStorage = globalThis.localStorage;
  globalThis.localStorage = { getItem: key => (key === 'token' ? 'admin-jwt' : null) };
  try {
    const file = new File([new Uint8Array([9, 8, 7])], 'swanpanel-backup.tar.enc', { type: 'application/octet-stream' });
    let capturedForm = null;
    const result = await uploadRestoreBackup(file, 'correct-horse-battery-staple', 'RESTORE BACKUP', async (url, init) => {
      assert.equal(url, '/api/system/restore');
      assert.equal(init.method, 'POST');
      assert.equal(init.cache, 'no-store');
      assert.equal(init.headers.Authorization, 'Bearer admin-jwt');
      assert.ok(init.body instanceof FormData);
      capturedForm = init.body;
      return new Response(JSON.stringify({ runtime_status: 'applied', message: 'Restored cleanly' }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      });
    });

    assert.equal(result.runtime_status, 'applied');
    assert.equal(capturedForm.get('passphrase'), 'correct-horse-battery-staple');
    assert.equal(capturedForm.get('confirm'), 'RESTORE BACKUP');
    assert.equal(capturedForm.get('backup_file').name, 'swanpanel-backup.tar.enc');
  } finally {
    globalThis.localStorage = prevStorage;
  }
});

test('ServerSettingsPage exposes Backup & Restore tab with typed confirmation and accessibility guards', () => {
  const page = readFileSync(join(uiRoot, 'pages/ServerSettingsPage.tsx'), 'utf8');
  assert.match(page, /'backup'/);
  assert.match(page, /RESTORE BACKUP/);
  assert.match(page, /confirm\.restore_backup_title/);
  assert.match(page, /restore_restarting/);
  assert.match(page, /waitForControlPlaneRestart/);
  assert.match(page, /aria-describedby/);
  assert.match(page, /aria-invalid/);
  assert.doesNotMatch(page, /dangerouslySetInnerHTML/);
});

test('waitForControlPlaneRestart waits across 503 maintenance and resolves when healthy', async () => {
  let calls = 0;
  await waitForControlPlaneRestart(5, 1, async () => {
    calls++;
    if (calls === 1) {
      throw new TypeError('connection refused during restart');
    }
    if (calls < 3) {
      return new Response(JSON.stringify({ status: 'maintenance', maintenance: true }), { status: 503 });
    }
    return new Response(JSON.stringify({ status: 'healthy', maintenance: false }), { status: 200 });
  });
  assert.equal(calls, 3);
});

test('waitForControlPlaneRestart rejects clearly on timeout', async () => {
  await assert.rejects(
    () => waitForControlPlaneRestart(2, 1, async () => new Response('', { status: 503 })),
    err => err instanceof RestoreOutcomeError && err.outcome === 'timeout',
  );
});

test('waitForControlPlaneRestart distinguishes rolled_back and failed_closed outcomes from applied healthy restarts', async () => {
  const seenUrls = [];
  await assert.rejects(
    () => waitForControlPlaneRestart(3, 1, async url => {
      seenUrls.push(url);
      return new Response(JSON.stringify({
        status: 'healthy',
        maintenance: false,
        restore_status: 'rolled_back',
        restore_error: 'checksum mismatch',
      }), { status: 200 });
    }, 'restore-123'),
    err => err instanceof RestoreOutcomeError && err.outcome === 'rolled_back' && /checksum mismatch/.test(err.message),
  );
  assert.equal(seenUrls[0], '/api/health?restore_id=restore-123');

  await assert.rejects(
    () => waitForControlPlaneRestart(3, 1, async () => new Response(JSON.stringify({
      status: 'maintenance',
      mode: 'failed_closed',
      restore_status: 'failed_closed',
      restore_error: 'disk failure',
    }), { status: 503 }), 'restore-456'),
    err => err instanceof RestoreOutcomeError && err.outcome === 'failed_closed' && /disk failure/.test(err.message),
  );

  const outcome = await waitForControlPlaneRestart(3, 1, async () => new Response(JSON.stringify({
    status: 'healthy',
    maintenance: false,
    restore_status: 'applied',
  }), { status: 200 }), 'restore-789');
  assert.equal(outcome, 'applied');

  await assert.rejects(
    () => waitForControlPlaneRestart(3, 1, async () => new Response(JSON.stringify({
      status: 'healthy',
      maintenance: false,
    }), { status: 200 }), 'restore-missing-correlation'),
    err => err instanceof RestoreOutcomeError && err.outcome === 'unverified',
  );
});
