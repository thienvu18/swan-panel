import React, { useEffect, useId, useRef, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { effectiveGatewayStatus, GATEWAY_HEALTH_MAX_AGE_MS } from '../lib/gatewayHealth';
import type { GatewayHealth, GatewayStatus as Status } from '../lib/gatewayHealth';

const statusColors: Record<Status, string> = {
  online: 'bg-emerald-500',
  degraded: 'bg-amber-500',
  offline: 'bg-rose-500',
  unknown: 'bg-slate-500',
};

export const GatewayStatus: React.FC<{ compact?: boolean }> = ({ compact = false }) => {
  const { t, i18n } = useTranslation();
  const [now, setNow] = useState(() => Date.now());
  const [detailsOpen, setDetailsOpen] = useState(false);
  const detailsId = useId();
  const containerRef = useRef<HTMLDivElement>(null);
  const { data, isError, refetch } = useQuery<{ health: GatewayHealth }>({
    queryKey: ['gateway_health'],
    queryFn: async () => {
      const res = await fetch('/api/gateway/health', {
        headers: { Authorization: `Bearer ${localStorage.getItem('token') || ''}` },
        cache: 'no-store',
      });
      if (!res.ok) throw new Error('Gateway health request failed');
      return res.json();
    },
    enabled: Boolean(localStorage.getItem('token')),
  });


  // Close the detail panel on outside interaction or Escape.
  useEffect(() => {
    if (!detailsOpen) return;
    const onPointerDown = (event: PointerEvent) => {
      if (!containerRef.current?.contains(event.target as Node)) setDetailsOpen(false);
    };
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setDetailsOpen(false);
    };
    document.addEventListener('pointerdown', onPointerDown);
    document.addEventListener('keydown', onKeyDown);
    return () => {
      document.removeEventListener('pointerdown', onPointerDown);
      document.removeEventListener('keydown', onKeyDown);
    };
  }, [detailsOpen]);

  // Refresh once when a suspended tab becomes visible again.
  useEffect(() => {
    const onVisibility = () => {
      if (document.visibilityState === 'visible') {
        setNow(Date.now());
        void refetch();
      }
    };
    document.addEventListener('visibilitychange', onVisibility);
    return () => document.removeEventListener('visibilitychange', onVisibility);
  }, [refetch]);

  // A one-shot expiry timer changes the display to Unknown without polling.
  // Incoming SSE snapshots reschedule it; the browser never polls gateway data.
  useEffect(() => {
    const checked = Date.parse(data?.health?.checked_at || '');
    if (!Number.isFinite(checked)) return;
    setNow(Date.now());
    const remaining = GATEWAY_HEALTH_MAX_AGE_MS - (Date.now() - checked);
    const timeout = window.setTimeout(() => setNow(Date.now()), Math.max(0, remaining) + 30);
    return () => window.clearTimeout(timeout);
  }, [data?.health?.checked_at]);

  const status = isError ? 'unknown' : effectiveGatewayStatus(data?.health, now);
  const labels: Record<Status, string> = {
    online: t('nav.gateway_online'),
    degraded: t('nav.gateway_degraded'),
    offline: t('nav.gateway_offline'),
    unknown: t('nav.gateway_unknown'),
  };
  const compactLabels: Record<Status, string> = {
    online: t('nav.gateway_online_short'),
    degraded: t('nav.gateway_degraded_short'),
    offline: t('nav.gateway_offline_short'),
    unknown: t('nav.gateway_unknown_short'),
  };
  const label = compact ? compactLabels[status] : labels[status];
  const lastCheck = data?.health?.checked_at ? new Date(data.health.checked_at) : null;
  const checkTime = lastCheck && Number.isFinite(lastCheck.getTime())
    ? lastCheck.toLocaleTimeString(i18n.language.startsWith('vi') ? 'vi-VN' : 'en-US')
    : null;
  const tooltip = checkTime ? `${label} · ${t('nav.gateway_last_checked', { time: checkTime })}` : label;

  return (
    <div ref={containerRef} className="relative min-w-0">
      <button type="button" title={tooltip} aria-label={tooltip}
        aria-expanded={detailsOpen} aria-controls={detailsOpen ? detailsId : undefined}
        onClick={() => setDetailsOpen(open => !open)}
        className="inline-flex items-center gap-1.5 min-w-0 max-w-full rounded px-1 py-1 text-xs font-medium text-subtle hover:text-current">
        <span aria-hidden="true" className={`w-2 h-2 rounded-full shrink-0 ${statusColors[status]}`} />
        <span role="status" aria-live="polite" className="truncate">{label}</span>
      </button>
      {detailsOpen && (
        <div id={detailsId} className="absolute right-0 bottom-full mb-2 max-md:bottom-auto max-md:top-full max-md:mt-2 w-52 max-w-[calc(100vw-2rem)] rounded-lg border subtle-border surface-card p-3 text-xs text-heading shadow-lg z-50">
          <p className="font-semibold">{labels[status]}</p>
          <p className="text-subtle mt-1">
            {checkTime ? t('nav.gateway_last_checked', { time: checkTime }) : t('nav.gateway_never_checked')}
          </p>
        </div>
      )}
    </div>
  );
};
