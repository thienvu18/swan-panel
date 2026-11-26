import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const srcRoot = join(dirname(fileURLToPath(import.meta.url)), '..', 'src');

test('Audit trail uses accessible tabs, region-wrapped tables, and header-authenticated exports', () => {
  const page = readFileSync(join(srcRoot, 'pages/AuditTrailPage.tsx'), 'utf8');
  assert.match(page, /role="tablist"/);
  assert.match(page, /role="tab"/);
  assert.match(page, /role="tabpanel"/);
  assert.match(page, /role="region"/);
  assert.match(page, /tabIndex=\{0\}/);
  assert.match(page, /<caption id=\{auditCaptionId\}/);
  assert.match(page, /<caption id=\{webhooksCaptionId\}/);
  assert.match(page, /\/api\/audit-logs\/export\?/);
  assert.match(page, /Authorization:\s*`Bearer \$\{token\}`/);
  assert.doesNotMatch(page, /\?.*token=/);
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
