import test from 'node:test';
import assert from 'node:assert/strict';
import { downloadDeviceProfile } from '../src/lib/profileDownload.ts';

async function withBrowserStubs(options, run) {
  const originals = {
    localStorage: globalThis.localStorage,
    fetch: globalThis.fetch,
    document: globalThis.document,
    window: globalThis.window,
    URL: globalThis.URL,
  };
  const events = { calls: [], links: [], revoked: [] };
  globalThis.localStorage = { getItem: () => options.token ?? null };
  globalThis.fetch = async (url, init) => {
    events.calls.push({ url, init });
    return options.response;
  };
  globalThis.document = {
    createElement: () => {
      const anchor = {
        href: '', download: '', click() { events.links.push(this.download); },
        remove() {},
      };
      return anchor;
    },
    body: { appendChild() {} },
  };
  globalThis.window = { setTimeout: (fn) => fn() };
  globalThis.URL = {
    createObjectURL: () => 'blob:profile-test',
    revokeObjectURL: (url) => events.revoked.push(url),
  };
  try {
    await run(events);
  } finally {
    for (const [name, value] of Object.entries(originals)) {
      if (value === undefined) delete globalThis[name];
      else globalThis[name] = value;
    }
  }
}

test('profile downloads use the Authorization header, never a JWT in the URL', async () => {
  await withBrowserStubs({
    token: 'sensitive-jwt',
    response: {
      ok: true,
      headers: { get: () => 'attachment; filename="work-laptop.mobileconfig"' },
      blob: async () => new Blob(['certificate']),
    },
  }, async ({ calls, links, revoked }) => {
    await downloadDeviceProfile('dev /123', 'macos', 'work-laptop');
    assert.equal(calls.length, 1);
    assert.equal(calls[0].url, '/api/devices/dev%20%2F123/config/macos');
    assert.equal(calls[0].init.headers.Authorization, 'Bearer sensitive-jwt');
    assert.equal(calls[0].init.cache, 'no-store');
    assert.doesNotMatch(calls[0].url, /token|sensitive-jwt/);
    assert.deepEqual(links, ['work-laptop.mobileconfig']);
    assert.deepEqual(revoked, ['blob:profile-test']);
  });
});
test('expired authentication aborts the download with a useful message', async () => {
  await withBrowserStubs({ token: 'stale', response: { ok: false, status: 401 } }, async ({ links }) => {
    await assert.rejects(
      downloadDeviceProfile('dev_1', 'ios', 'iphone'),
      /session has expired/,
    );
    assert.equal(links.length, 0);
  });
});

test('missing session token does not make an unauthenticated request', async () => {
  await withBrowserStubs({ token: null }, async ({ calls }) => {
    await assert.rejects(
      downloadDeviceProfile('dev_1', 'linux', 'server'),
      /Sign in again/,
    );
    assert.equal(calls.length, 0);
  });
});
