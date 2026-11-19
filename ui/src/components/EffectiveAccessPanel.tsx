import React, { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { AlertTriangle, CheckCircle2, Search, ShieldCheck, ShieldX } from 'lucide-react';
import { apiJSON } from '../lib/api';

interface DeviceOption { id: string; hostname: string; is_revoked: boolean }
const isValidDestinationIPv4 = (value: string): boolean => {
  const parts = value.split('.');
  return parts.length === 4 && parts.every(part => /^(0|[1-9][0-9]{0,2})$/.test(part) && Number(part) <= 255);
};
interface PolicyResult {
  policy: {
    device_id: string;
    virtual_ip: string;
    allowed_selectors: string[] | null;
    denied_selectors: string[] | null;
  };
  runtime: { status: string; desired_generation: number; applied_generation: number };
  decision?: {
    destination: string;
    action: 'ALLOW' | 'DENY';
    reason: string;
    matched_cidr?: string;
    rule_sources?: { id: string; subject_type: 'device' | 'user' | 'user_group'; rule_nature: string }[];
  };
}

export const EffectiveAccessPanel: React.FC<{ devices: DeviceOption[] }> = ({ devices }) => {
  const { t } = useTranslation();
  const [deviceId, setDeviceId] = useState('');
  const [destination, setDestination] = useState('');
  const [submittedDestination, setSubmittedDestination] = useState('');

  const currentDevices = devices.filter(device => !device.is_revoked);
  const destinationValid = isValidDestinationIPv4(destination.trim());
  const query = useQuery<PolicyResult>({
    queryKey: ['effective_policy', deviceId, submittedDestination],
    enabled: Boolean(deviceId),
    queryFn: () => apiJSON<PolicyResult>(
      '/api/access-rules/effective/' + encodeURIComponent(deviceId) +
      (submittedDestination ? '?destination=' + encodeURIComponent(submittedDestination) : '')
    ),
    retry: false,
  });

  const result = query.data;
  const runtimeApplied = result?.runtime.status === 'applied' &&
    result.runtime.desired_generation === result.runtime.applied_generation;
  const allowed = result?.policy.allowed_selectors || [];
  const denied = result?.policy.denied_selectors || [];
  const decision = result?.decision;
  const ruleSources = decision?.rule_sources || [];
  const reason = decision?.reason === 'explicit_allow' ? t('effective_policy.explicit_allow')
    : decision?.reason === 'explicit_deny' ? t('effective_policy.explicit_deny')
    : decision?.reason === 'default_deny' ? t('effective_policy.default_deny') : '';

  return (
    <section className="border subtle-border rounded-xl surface-card p-4 space-y-4" aria-labelledby="effective-policy-title">
      <div className="flex items-start gap-2">
        <ShieldCheck className="w-4 h-4 text-accent mt-0.5 shrink-0" aria-hidden="true" />
        <div>
          <h2 id="effective-policy-title" className="font-semibold text-sm">{t('effective_policy.title')}</h2>
          <p className="text-xs text-subtle mt-1 leading-relaxed">{t('effective_policy.description')}</p>
        </div>
      </div>
      <div className="grid grid-cols-1 md:grid-cols-3 gap-3 items-end">
        <label className="block space-y-1 text-xs">
          <span className="font-medium text-subtle">{t('effective_policy.device')}</span>
          <select className="w-full surface-input border subtle-border rounded-lg text-xs" value={deviceId}
            onChange={event => { setDeviceId(event.target.value); setSubmittedDestination(''); setDestination(''); }}>
            <option value="">{t('effective_policy.select_device')}</option>
            {currentDevices.map(device => <option key={device.id} value={device.id}>{device.hostname}</option>)}
          </select>
        </label>
        <form className="md:col-span-2 flex gap-2 items-end" onSubmit={event => {
          event.preventDefault();
          if (destinationValid) setSubmittedDestination(destination.trim());
        }}>
          <label className="flex-1 min-w-0 space-y-1 text-xs">
            <span className="font-medium text-subtle">{t('effective_policy.destination')}</span>
            <input className="w-full surface-input border subtle-border rounded-lg px-3 py-2 text-xs font-mono"
              placeholder="10.99.1.10" value={destination} disabled={!deviceId}
              onChange={event => setDestination(event.target.value)} inputMode="decimal" aria-invalid={Boolean(destination.trim() && !destinationValid)} />
            {destination.trim() && !destinationValid &&
              <span role="alert" className="block text-[11px] text-status-danger">{t('effective_policy.invalid_destination')}</span>}
          </label>
          <button type="submit" disabled={!deviceId || !destinationValid}
            className="bg-blue-600 hover:bg-blue-500 rounded-lg px-3 py-2 text-xs font-semibold text-white disabled:opacity-50 flex items-center gap-1.5">
            <Search className="w-3.5 h-3.5" aria-hidden="true" />
            {t('effective_policy.check')}
          </button>
        </form>
      </div>
      {query.isFetching && <p role="status" className="text-xs text-subtle">{t('effective_policy.loading')}</p>}
      {query.isError && <div role="alert" className="text-xs text-status-danger flex flex-wrap items-center gap-2">
        {t('effective_policy.unable')}
        <button type="button" onClick={() => void query.refetch()} className="underline">
          {t('effective_policy.retry')}
        </button>
      </div>}
      {result && !query.isError && (
        <div className="border-t subtle-border pt-3 space-y-3">
          <div className="flex flex-wrap items-center gap-2 text-xs">
            <span className={`border rounded-md px-2 py-1 font-medium ${runtimeApplied
              ? 'border-emerald-500/30 text-status-success bg-emerald-500/10'
              : 'border-amber-500/30 text-status-warning bg-amber-500/10'}`}>
              {runtimeApplied ? t('effective_policy.runtime_applied') : t('effective_policy.runtime_pending')}
            </span>
            <code className="font-mono text-subtle">{result.policy.virtual_ip}</code>
          </div>
          {!runtimeApplied && <p role="status" className="flex gap-2 text-xs text-status-warning">
            <AlertTriangle className="w-4 h-4 shrink-0" aria-hidden="true" />{t('effective_policy.preview_warning')}
          </p>}
          {decision && <div role="status" aria-live="polite" className={`rounded-lg border p-4 space-y-3 ${decision.action === 'ALLOW'
            ? 'border-emerald-500/40 bg-emerald-500/10' : 'border-rose-500/40 bg-rose-500/10'}`}>
            <div className="flex gap-3 items-start">
              {decision.action === 'ALLOW'
                ? <CheckCircle2 className="w-6 h-6 shrink-0 text-status-success" aria-hidden="true" />
                : <ShieldX className="w-6 h-6 shrink-0 text-status-danger" aria-hidden="true" />}
              <div className="min-w-0 space-y-1">
                <p className="text-[11px] font-medium text-subtle">{t('effective_policy.verdict_for')}</p>
                <code className="block font-mono text-xs break-all">{decision.destination}</code>
                <p className={`text-base font-bold ${decision.action === 'ALLOW' ? 'text-status-success' : 'text-status-danger'}`}>
                  {t(decision.action === 'ALLOW' ? 'effective_policy.result_allow' : 'effective_policy.result_deny')}
                </p>
                <p className="text-xs text-subtle">{reason}</p>
              </div>
            </div>
            {decision.matched_cidr && <p className="text-xs">
              <span className="text-subtle">{t('effective_policy.matched')}: </span>
              <code className="font-mono font-semibold break-all">{decision.matched_cidr}</code>
            </p>}
            {ruleSources.length > 0 && <div className="border-t subtle-border pt-2 space-y-1">
              <p className="text-xs font-semibold">{t('effective_policy.matched_sources')}</p>
              <ul className="space-y-1">
                {ruleSources.map(source => <li key={source.id} className="flex flex-wrap items-center gap-2 text-xs">
                  <span className="text-subtle">{t(`effective_policy.source_${source.subject_type}`)}</span>
                  <code className="font-mono text-[11px]">{source.id}</code>
                </li>)}
              </ul>
            </div>}
            <p className="text-[11px] text-subtle">{t('effective_policy.preview_note')}</p>
          </div>}
          {!decision && submittedDestination === '' && <p className="text-xs text-subtle">{t('effective_policy.enter_destination')}</p>}
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
            {([
              [t('effective_policy.allowed'), allowed, 'text-status-success'],
              [t('effective_policy.denied'), denied, 'text-status-danger'],
            ] as const).map(([label, selectors, color]) => (
              <div key={label} className="surface-header rounded-lg border subtle-border p-3 space-y-2">
                <h3 className={`text-xs font-semibold ${color}`}>{label}</h3>
                <div className="flex flex-wrap gap-1.5">
                  {selectors.length
                    ? selectors.map(cidr => <code key={cidr} className="font-mono text-[11px] rounded border subtle-border px-2 py-1">{cidr}</code>)
                    : <span className="text-xs text-subtle">{t('effective_policy.none')}</span>}
                </div>
              </div>
            ))}
          </div>
          {allowed.length === 0 && <p className="text-xs text-subtle">{t('effective_policy.no_grants')}</p>}
          {!decision && <p className="text-[11px] text-dim">{t('effective_policy.preview_note')}</p>}
        </div>
      )}
    </section>
  );
};
