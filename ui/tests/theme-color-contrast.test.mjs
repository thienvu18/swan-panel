import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

const styles = readFileSync(new URL('../src/index.css', import.meta.url), 'utf8');
const usersPage = readFileSync(new URL('../src/pages/UsersPage.tsx', import.meta.url), 'utf8');
const accessRulesPage = readFileSync(new URL('../src/pages/AccessRulesPage.tsx', import.meta.url), 'utf8');

const luminance = (hex) => {
  const channels = hex.slice(1).match(/../g).map((channel) => parseInt(channel, 16) / 255);
  const [red, green, blue] = channels.map((value) =>
    value <= 0.04045 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4,
  );
  return red * 0.2126 + green * 0.7152 + blue * 0.0722;
};

const contrastRatio = (foreground, background) => {
  const values = [luminance(foreground), luminance(background)].sort((a, b) => b - a);
  return (values[0] + 0.05) / (values[1] + 0.05);
};

const themeTextColor = (selector) => {
  const declaration = styles.split(selector + ' {')[1]?.split('}')[0];
  assert.ok(declaration, `Missing theme color rule: ${selector}`);
  const color = declaration.match(/color:\s*(#[0-9a-f]{6})/i)?.[1];
  assert.ok(color, `Missing foreground color for ${selector}`);
  return color;
};

test('light surface labels, hints, and accent text meet WCAG AA normal-text contrast', () => {
  for (const role of ['text-subtle', 'text-dim', 'text-accent', 'text-status-success', 'text-status-warning', 'text-status-danger']) {
    const color = themeTextColor(`html:not(.dark) .${role}`);
    assert.ok(contrastRatio(color, '#ffffff') >= 4.5, `${role} is too faint on white: ${color}`);
  }
  assert.match(styles, /html:not\(\.dark\) \.surface-input::placeholder \{ color: #64748b; opacity: 1; \}/);
});

test('dark surface hints and status labels remain readable', () => {
  for (const role of ['text-dim', 'text-status-success', 'text-status-warning', 'text-status-danger']) {
    const color = themeTextColor(`html.dark .${role}`);
    assert.ok(contrastRatio(color, '#111827') >= 4.5, `${role} is too faint on the dark surface: ${color}`);
  }
});

test('table row separators use surface borders instead of inherited text colors', () => {
  assert.match(styles, /html:not\(\.dark\) \.divide-y\.subtle-border > :not\(:last-child\) \{ border-color: #e2e8f0; \}/);
  assert.match(styles, /html\.dark \.divide-y\.subtle-border > :not\(:last-child\) \{ border-color: #1f2937; \}/);
});

test('directory authentication badges do not force dark colors in light theme', () => {
  assert.doesNotMatch(usersPage, /border-zinc-700 bg-zinc-800 text-zinc-300/);
  assert.match(usersPage, /bg-slate-100 text-slate-700 dark:bg-slate-800 dark:text-slate-200/);
});

test('access rule modal subject type buttons do not use white text on light backgrounds', () => {
  assert.doesNotMatch(accessRulesPage, /bg-blue-600\/10 text-white/);
  assert.match(accessRulesPage, /border-blue-500 bg-blue-500\/10 text-accent font-semibold/);

  // Active accent color in light mode (#1d4ed8) against 10% blue tinted surface (~#eff6ff)
  const lightAccent = themeTextColor('html:not(.dark) .text-accent');
  assert.ok(contrastRatio(lightAccent, '#eff6ff') >= 4.5, `Active Subject Type contrast too low: ${lightAccent}`);
});

test('access rule DENY rows and action buttons use high-contrast status-danger colors', () => {
  assert.doesNotMatch(accessRulesPage, /<span className="text-rose-400 flex items-center/);
  assert.doesNotMatch(accessRulesPage, /text-rose-400 text-\[10px\] border border-rose-500\/30/);
  assert.doesNotMatch(accessRulesPage, /action === 'DENY' \? 'border-rose-500 bg-rose-600\/10 text-rose-400'/);

  assert.match(accessRulesPage, /text-status-danger flex items-center gap-1\.5 font-medium/);
  assert.match(accessRulesPage, /text-status-danger text-\[10px\] border border-rose-500\/30 bg-rose-500\/10 px-1\.5 py-0\.5 rounded font-mono font-medium/);
  assert.match(accessRulesPage, /action === 'DENY' \? 'border-rose-500 bg-rose-500\/15 text-status-danger font-semibold'/);

  // DENY text color in light mode (#be123c) against 5% rose tinted row background (~#fff1f2)
  const lightDanger = themeTextColor('html:not(.dark) .text-status-danger');
  assert.ok(contrastRatio(lightDanger, '#fff1f2') >= 4.5, `DENY text contrast too low on light row: ${lightDanger}`);
});
