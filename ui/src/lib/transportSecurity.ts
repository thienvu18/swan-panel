export interface SSEEvent {
  type: string;
  payload: unknown;
  timestamp: number;
}

/** SSE JSON is sent in unnamed "data:" frames by the Go broker. */
export function parseSSEFrame(frame: string): SSEEvent | null {
  const lines = frame.replace(/\r\n/g, '\n').split('\n');
  const data = lines.filter(line => line.startsWith('data:'))
    .map(line => line.slice(5).trimStart())
    .join('\n');
  if (!data) return null;
  try {
    const value: unknown = JSON.parse(data);
    if (!value || typeof value !== 'object') return null;
    const record = value as Record<string, unknown>;
    if (typeof record.type !== 'string' || !record.type ||
        typeof record.timestamp !== 'number') return null;
    return {
      type: record.type,
      payload: record.payload,
      timestamp: record.timestamp,
    };
  } catch {
    return null;
  }
}

/** Extract complete LF or CRLF SSE frames while retaining the unfinished tail. */
export function extractSSEFrames(buffer: string): { frames: string[]; rest: string } {
  const frames: string[] = [];
  let rest = buffer;
  let match: RegExpExecArray | null;
  const delimiter = /\r?\n\r?\n/;
  while ((match = delimiter.exec(rest)) !== null) {
    frames.push(rest.slice(0, match.index));
    rest = rest.slice(match.index + match[0].length);
  }
  // Never allow malformed upstream frames to exhaust the browser.
  if (rest.length > 1024 * 1024) rest = '';
  return { frames, rest };
}

export function oidcStartPath(id: string): string {
  return '/api/auth/oidc/' + encodeURIComponent(id) + '/start';
}

export function downloadFilename(contentDisposition: string | null, fallback: string): string {
  const matched = contentDisposition?.match(/filename="([^"]+)"/)?.[1];
  const proposed = matched || fallback;
  // Never trust a server-provided path as an output filename.
  return proposed.replace(/[\\/\x00-\x1f]/g, '_').slice(0, 180);
}
