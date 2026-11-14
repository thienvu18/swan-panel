import { useEffect, useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { parseSSEFrame } from '../lib/transportSecurity';

export interface RealtimeEvent {
  type: string;
  payload: unknown;
  timestamp: number;
}

// Native EventSource cannot attach Bearer headers. Use a streaming fetch so
// the JWT never appears in URLs, proxy logs or browser history.
export function useRealtime(token?: string | null) {
  const [isConnected, setIsConnected] = useState(false);
  const [lastEvent, setLastEvent] = useState<RealtimeEvent | null>(null);
  const queryClient = useQueryClient();

  useEffect(() => {
    const activeToken = token !== undefined ? token : localStorage.getItem('token');
    if (!activeToken) { setIsConnected(false); return; }

    const abort = new AbortController();
    let reconnectDelay: ReturnType<typeof setTimeout> | undefined;

    const dispatch = (event: RealtimeEvent) => {
      setLastEvent(event);
      if (event.type.includes('session') || event.type === 'gateway_status' ||
          event.type === 'device_created' || event.type === 'device_revoked') {
        queryClient.invalidateQueries({ queryKey: ['dashboard_stats'] });
        queryClient.invalidateQueries({ queryKey: ['devices'] });
      } else if (event.type === 'strongswan_settings_updated') {
        queryClient.invalidateQueries({ queryKey: ['strongswan_settings'] });
      } else if (event.type === 'approval_queue_update') {
        queryClient.invalidateQueries({ queryKey: ['users'] });
      }
    };

    const connect = async () => {
      if (abort.signal.aborted) return;
      try {
        const response = await fetch('/api/realtime/stream', {
          headers: { Authorization: `Bearer ${activeToken}`, Accept: 'text/event-stream' },
          signal: abort.signal,
          cache: 'no-store',
        });
        if (!response.ok || !response.body) {
          throw new Error(`SSE unavailable (HTTP ${response.status})`);
        }
        setIsConnected(true);
        const reader = response.body.getReader();
        const decoder = new TextDecoder();
        let textBuffer = '';
        while (!abort.signal.aborted) {
          const { done, value } = await reader.read();
          if (done) break;
          textBuffer += decoder.decode(value, { stream: true });
          // SSE frames end in a blank line; heartbeat frames are comments.
          let frameEnd: number;
          while ((frameEnd = textBuffer.indexOf('\n\n')) >= 0) {
            const frame = textBuffer.slice(0, frameEnd);
            textBuffer = textBuffer.slice(frameEnd + 2);
            const parsed = parseSSEFrame(frame);
            if (parsed) dispatch(parsed);
          }
        }
      } catch (error) {
        if (!abort.signal.aborted) console.warn('SwanPanel realtime disconnected:', error);
      } finally {
        setIsConnected(false);
        if (!abort.signal.aborted) reconnectDelay = setTimeout(connect, 3000);
      }
    };

    void connect();
    return () => {
      abort.abort();
      if (reconnectDelay !== undefined) clearTimeout(reconnectDelay);
      setIsConnected(false);
    };
  }, [queryClient, token]);

  return { isConnected, lastEvent };
}
