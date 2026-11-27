import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { exportAuditLogs } from '../src/lib/auditApi.ts';

const srcRoot = join(dirname(fileURLToPath(import.meta.url)), '..', 'src');

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
  URL.createObjectURL = () => 'blob:audit-test';
  URL.revokeObjectURL = () => {};

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

test('Audit trail uses accessible tabs, region-wrapped tables, and header-authenticated exports', () => {
  const page = readFileSync(join(srcRoot, 'pages/AuditTrailPage.tsx'), 'utf8');
  assert.match(page, /role="tablist"/);
  assert.match(page, /role="tab"/);
  assert.match(page, /role="tabpanel"/);
  assert.match(page, /role="region"/);
  assert.match(page, /tabIndex=\{0\}/);
  assert.match(page, /<caption id=\{auditCaptionId\}/);
  assert.match(page, /<caption id=\{webhooksCaptionId\}/);
  assert.match(page, /exportAuditLogs/);
  assert.doesNotMatch(page, /\?.*token=/);
});

test('exportAuditLogs sends Authorization header, filters, sanitizes filenames, and surfaces errors', async () => {
  await withMockBrowser(async anchor => {
    let seenURL = '';
    let seenInit = null;
    const filename = await exportAuditLogs(
      'csv',
      { severity: 'critical', action: 'device.*', search: 'laptop' },
      async (url, init) => {
        seenURL = url;
        seenInit = init;
        return new Response('sequence,id\n1,aud_1\n', {
          status: 200,
          headers: { 'Content-Disposition': 'attachment; filename="../../evil\\audit.csv"' },
        });
      },
    );
    assert.equal(seenURL, '/api/audit-logs/export?format=csv&severity=critical&action=device.*&search=laptop');
    assert.equal(seenInit.headers.Authorization, 'Bearer admin-jwt');
    assert.equal(seenInit.cache, 'no-store');
    assert.equal(anchor.clicked, true);
    assert.equal(filename, '.._.._evil_audit.csv');
    assert.ok(!filename.includes('/') && !filename.includes('\\'));

    await assert.rejects(
      () => exportAuditLogs('jsonl', {}, async () => new Response(JSON.stringify({ error: 'Forbidden' }), { status: 403 })),
      /Forbidden/,
    );
  });
});

test('Audit trail navigation and SSE invalidation stay admin-only and free of polling', () => {
  const app = readFileSync(join(srcRoot, 'App.tsx'), 'utf8');
  const sidebar = readFileSync(join(srcRoot, 'components/Sidebar.tsx'), 'utf8');
  const realtime = readFileSync(join(srcRoot, 'hooks/useRealtime.ts'), 'utf8');
  assert.match(app, /'audit'/);
  assert.match(app, /AuditTrailPage/);
  assert.match(sidebar, /id:\s*'audit'/);
  assert.match(sidebar, /item\.id === "dashboard" \|\| item\.id === "resources"/);
  assert.match(realtime, /event\.type === 'audit_event'/);
  assert.doesNotMatch(readFileSync(join(srcRoot, 'pages/AuditTrailPage.tsx'), 'utf8'), /setInterval/);
});
