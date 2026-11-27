import { useEffect, useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { extractSSEFrames, parseSSEFrame } from '../lib/transportSecurity';

export interface RealtimeEvent {
  type: string;
  payload: unknown;
  timestamp: number;
}

// Native EventSource cannot attach Bearer headers. Use a streaming fetch so
// the JWT never appears in URLs, proxy logs or browser history.
export function useRealtime(token?: string | null) {
  const [isConnected, setIsConnected] = useState(false);
  const queryClient = useQueryClient();

  useEffect(() => {
    const activeToken = token !== undefined ? token : localStorage.getItem('token');
    if (!activeToken) { setIsConnected(false); return; }

    const abort = new AbortController();
    let reconnectDelay: ReturnType<typeof setTimeout> | undefined;

    let lastLogsInvalidated = 0;
    const dispatch = (event: RealtimeEvent) => {
      if (event.type === 'gateway_health') {
        const snapshot = event.payload as { status?: unknown; checked_at?: unknown } | null;
        if (snapshot && typeof snapshot.status === 'string' && typeof snapshot.checked_at === 'string') {
          queryClient.setQueryData(['gateway_health'], { health: snapshot });
        }
        return;
      }
      if (event.type.includes('session') || event.type === 'gateway_status' ||
          event.type === 'device_created' || event.type === 'device_revoked') {
        queryClient.invalidateQueries({ queryKey: ['dashboard_stats'] });
        queryClient.invalidateQueries({ queryKey: ['devices'] });
        if (event.type === 'gateway_status') {
          queryClient.invalidateQueries({ queryKey: ['daemon_status'] });
          queryClient.invalidateQueries({ queryKey: ['runtime_apply_status'] });
          queryClient.invalidateQueries({ queryKey: ['effective_policy'] });
        }
        if (event.type === 'device_revoked' || event.type === 'device_created') queryClient.invalidateQueries({ queryKey: ['effective_policy'] });
      } else if (event.type === 'strongswan_settings_updated') {
        queryClient.invalidateQueries({ queryKey: ['strongswan_settings'] });
      } else if (event.type === 'approval_queue_update' || event.type === 'user_approved') {
        queryClient.invalidateQueries({ queryKey: ['users'] });
      } else if (event.type === 'log_event' && Date.now() - lastLogsInvalidated >= 1000) {
        lastLogsInvalidated = Date.now();
        queryClient.invalidateQueries({ queryKey: ['system_logs'] });
      } else if (event.type === 'audit_event') {
        queryClient.invalidateQueries({ queryKey: ['audit_logs'] });
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
        queryClient.invalidateQueries({ queryKey: ['gateway_health'] });
        const reader = response.body.getReader();
        const decoder = new TextDecoder();
        let textBuffer = '';
        while (!abort.signal.aborted) {
          const { done, value } = await reader.read();
          if (done) break;
          textBuffer += decoder.decode(value, { stream: true });
          // SSE frames end in a blank line; heartbeat frames are comments.
          const { frames, rest } = extractSSEFrames(textBuffer);
          textBuffer = rest;
          for (const frame of frames) {
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

  return { isConnected };
}