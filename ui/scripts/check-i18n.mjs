import { readFileSync, readdirSync } from 'node:fs';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', 'src');
const en = JSON.parse(readFileSync(join(root, 'locales/en.json'), 'utf8'));
const vi = JSON.parse(readFileSync(join(root, 'locales/vi.json'), 'utf8'));

function flatten(data, prefix = '', target = {}) {
  for (const [key, value] of Object.entries(data)) {
    const name = prefix ? `${prefix}.${key}` : key;
    if (typeof value === 'string') target[name] = value;
    else if (value && typeof value === 'object') flatten(value, name, target);
    else throw new Error(`Invalid translation: ${name}`);
  }
  return target;
}

const english = flatten(en);
const vietnamese = flatten(vi);
const errors = [];
for (const key of new Set([...Object.keys(english), ...Object.keys(vietnamese)])) {
  if (!(key in english)) errors.push(`Missing English translation: ${key}`);
  if (!(key in vietnamese)) errors.push(`Missing Vietnamese translation: ${key}`);
  if (!english[key]?.trim() || !vietnamese[key]?.trim()) errors.push(`Blank translation: ${key}`);
}

function visit(dir) {
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) { if (entry.name !== 'locales') visit(path); continue; }
    if (!/\.tsx?$/.test(entry.name)) continue;
    const source = readFileSync(path, 'utf8');
    for (const match of source.matchAll(/\bt\(\s*(['"])([^'"]+)\1/g)) {
      const key = match[2];
      if (!(key in english) && !(`${key}_one` in english) && !(`${key}_other` in english)) {
        errors.push(`Unknown translation key ${key} in ${entry.name}`);
      }
    }
  }
}
visit(root);
if (errors.length > 0) {
  console.error(errors.join('\n'));
  process.exitCode = 1;
} else {
  console.log(`Verified ${Object.keys(english).length} EN/VI translations and referenced keys.`);
}
