import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { extractSSEFrames, parseSSEFrame } from '../src/lib/transportSecurity.ts';

test('SSE stream extracts LF, CRLF and fragmented events without losing tail', () => {
  const a = '{"type":"log_event","timestamp":123,"payload":null}';
  const b = '{"type":"gateway_status","timestamp":124,"payload":{}}';
  const buffer = `data: ${a}\r\n\r\ndata: ${b.slice(0, 16)}`;
  const first = extractSSEFrames(buffer);
  assert.equal(first.frames.length, 1);
  assert.equal(parseSSEFrame(first.frames[0])?.type, 'log_event');
  assert.equal(first.rest, `data: ${b.slice(0, 16)}`);
  const second = extractSSEFrames(first.rest + b.slice(16) + '\n\n');
  assert.equal(parseSSEFrame(second.frames[0])?.type, 'gateway_status');
  assert.equal(second.rest, '');
});

test('SSE parser limits unbounded incomplete input', () => {
  assert.equal(extractSSEFrames('x'.repeat(1024 * 1024 + 1)).rest, '');
});

test('System log view uses SSE without polling and sidebar has no placeholder IP', () => {
  const logs = readFileSync(new URL('../src/pages/SystemLogsPage.tsx', import.meta.url), 'utf8');
  const side = readFileSync(new URL('../src/components/Sidebar.tsx', import.meta.url), 'utf8');
  assert.doesNotMatch(logs, /refetchInterval/);
  assert.match(logs, /isRealtimeConnected/);
  assert.doesNotMatch(side, /10\.0\.0\.1/);
});

test('SSE log notifications do not re-render the application shell', () => {
  const realtime = readFileSync(new URL('../src/hooks/useRealtime.ts', import.meta.url), 'utf8');
  const logs = readFileSync(new URL('../src/pages/SystemLogsPage.tsx', import.meta.url), 'utf8');
  assert.doesNotMatch(realtime, /setLastEvent/);
  assert.match(logs, /deferredSearch/);
});
