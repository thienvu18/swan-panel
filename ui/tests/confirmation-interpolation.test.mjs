import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

test('confirmation templates interpolate named variables in both languages', () => {
  for (const language of ['en', 'vi']) {
    const locale = JSON.parse(readFileSync(new URL(`../src/locales/${language}.json`, import.meta.url), 'utf8'));
    const descriptions = Object.entries(locale.confirm).filter(([key]) => key.endsWith('_description'));
    assert.ok(descriptions.length > 0);
    for (const [key, template] of descriptions) {
      assert.doesNotMatch(template, /(?<!\{)\{(?:name|email|cidr|subject|target)\}(?!\})/, `${language}:${key}`);
    }
    assert.ok(descriptions.some(([, value]) => value.includes('{{name}}')), language);
  }
});
