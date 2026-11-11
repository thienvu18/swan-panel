import { useEffect, useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';

export interface RealtimeEvent {
  type: string;
  payload: any;
  timestamp: number;
}

export function useRealtime(token?: string | null) {
  const [isConnected, setIsConnected] = useState(false);
  const [lastEvent, setLastEvent] = useState<RealtimeEvent | null>(null);
  const queryClient = useQueryClient();

  useEffect(() => {
    const activeToken = token !== undefined ? token : localStorage.getItem('token');
    if (!activeToken) {
      setIsConnected(false);
      return;
    }

    let eventSource: EventSource | null = null;
    let reconnectTimeout: ReturnType<typeof setTimeout>;

    const connect = () => {
      const currentToken = token !== undefined ? token : localStorage.getItem('token');
      if (!currentToken) {
        setIsConnected(false);
        return;
      }

      const streamUrl = `/api/realtime/stream?token=${encodeURIComponent(currentToken)}`;
      eventSource = new EventSource(streamUrl);

      eventSource.onopen = () => {
        setIsConnected(true);
      };

      eventSource.onerror = () => {
        setIsConnected(false);
        if (eventSource) {
          eventSource.close();
        }
        // Automatically reconnect after 3 seconds if connection is interrupted
        reconnectTimeout = setTimeout(connect, 3000);
      };

      // Process real-time events
      eventSource.addEventListener('session_update', (e: MessageEvent) => {
        try {
          const data = JSON.parse(e.data);
          setLastEvent({ type: 'session_update', payload: data, timestamp: Date.now() });
          // Automatically invalidate dashboard cache immediately
          queryClient.invalidateQueries({ queryKey: ['dashboard_stats'] });
        } catch {}
      });

      eventSource.addEventListener('strongswan_settings_updated', (e: MessageEvent) => {
        try {
          const data = JSON.parse(e.data);
          setLastEvent({ type: 'strongswan_settings_updated', payload: data, timestamp: Date.now() });
          queryClient.invalidateQueries({ queryKey: ['strongswan_settings'] });
        } catch {}
      });

      eventSource.addEventListener('log_event', (e: MessageEvent) => {
        try {
          const data = JSON.parse(e.data);
          setLastEvent({ type: 'log_event', payload: data, timestamp: Date.now() });
        } catch {}
      });
    };

    connect();

    return () => {
      if (eventSource) {
        eventSource.close();
      }
      clearTimeout(reconnectTimeout);
    };
  }, [queryClient, token]);

  return { isConnected, lastEvent };
}
