import test from 'node:test';
import assert from 'node:assert/strict';
import { parseSSEFrame, downloadFilename, oidcStartPath } from '../src/lib/transportSecurity.ts';

test('SSE: Go broker unnamed data frame is decoded', () => {
  const value = parseSSEFrame('data: {"type":"device_revoked","payload":{"device_id":"dev_1"},"timestamp":123}\n');
  assert.deepEqual(value, { type:'device_revoked', payload:{device_id:'dev_1'}, timestamp:123 });
});

test('SSE: keepalive, corrupted JSON, and wrong types are ignored', () => {
  assert.equal(parseSSEFrame(': keepalive\n'), null);
  assert.equal(parseSSEFrame('data: broken'), null);
  assert.equal(parseSSEFrame('data: {"type":42,"timestamp":123}'), null);
  assert.equal(parseSSEFrame('data: {"type":"device","timestamp":"yesterday"}'), null);
});

test('SSE: CRLF and multi-line data work', () => {
  const event = parseSSEFrame('data: {"type":"session_update",\r\ndata: "payload":{},"timestamp":2}\r\n');
  assert.equal(event?.type, 'session_update');
});

test('OIDC start URL encodes provider ID; never carries JWT', () => {
  assert.equal(oidcStartPath('google'), '/api/auth/oidc/google/start');
  assert.equal(oidcStartPath('provider/with spaces'), '/api/auth/oidc/provider%2Fwith%20spaces/start');
  assert.doesNotMatch(oidcStartPath('provider'), /\?token=/);
});

test('download filename strips separators and control bytes', () => {
  assert.equal(downloadFilename('attachment; filename="setup-device.mobileconfig"', 'fallback'), 'setup-device.mobileconfig');
  assert.equal(downloadFilename('attachment; filename="../private\\key.p12"', 'fallback'), '.._private_key.p12');
  assert.equal(downloadFilename(null,'download.sswan'), 'download.sswan');
  assert.equal(downloadFilename('attachment; filename="a\u0000b.zip"','fallback'), 'a_b.zip');
});
