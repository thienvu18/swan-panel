import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

const login = readFileSync(new URL('../src/pages/LoginPage.tsx', import.meta.url), 'utf8');
const styles = readFileSync(new URL('../src/index.css', import.meta.url), 'utf8');

test('login background and text follow the active page theme', () => {
  assert.match(login, /min-h-screen[^"]*bg-transparent text-current/);
  assert.doesNotMatch(login, /bg-\[#0b0f17\] text-\[#f8fafc\]/);
  assert.match(styles, /html\.dark body \{ background-color: #0b0f17; color: #f8fafc; \}/);
  assert.match(styles, /html:not\(\.dark\) body \{ background-color: #f8fafc; color: #0f172a; \}/);
});

test('native select menus use dark or light options based on the document theme', () => {
  assert.match(styles, /html\.dark select \{\s*color-scheme: dark;/);
  assert.match(styles, /html:not\(\.dark\) select \{\s*color-scheme: light;/);
  assert.match(styles, /html\.dark select option,[\s\S]*?background: #111827; color: #f8fafc;/);
  assert.match(styles, /html:not\(\.dark\) select option,[\s\S]*?background: #fff; color: #0f172a;/);
});
