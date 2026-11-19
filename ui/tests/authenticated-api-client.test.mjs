import test from 'node:test';
import assert from 'node:assert/strict';
import { apiJSON, ApiError } from '../src/lib/api.ts';

test('API helper attaches bearer header and reports structured failures', async () => {
  const originalStore = globalThis.localStorage;
  const originalFetch = globalThis.fetch;
  globalThis.localStorage = { getItem: name => name === 'token' ? 'test-token' : null };
  let seen;
  globalThis.fetch = async (url, options) => {
    seen = { url, options };
    return new Response(JSON.stringify({ error: 'Rejected' }), { status: 409, headers: { 'Content-Type': 'application/json' } });
  };
  try {
    await assert.rejects(() => apiJSON('/api/example', { method: 'POST', body: JSON.stringify({ a: 1 }) }),
      error => error instanceof ApiError && error.status === 409 && error.message === 'Rejected');
    assert.equal(seen.url, '/api/example');
    assert.equal(seen.options.headers.get('Authorization'), 'Bearer test-token');
    assert.equal(seen.options.headers.get('Content-Type'), 'application/json');
  } finally {
    globalThis.fetch = originalFetch;
    globalThis.localStorage = originalStore;
  }
});
