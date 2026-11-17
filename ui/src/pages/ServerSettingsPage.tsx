import React, { useEffect, useMemo, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Check, Code2, FileJson2, RotateCcw, ShieldCheck, SlidersHorizontal } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useConfirmAction } from '../components/ConfirmDialog';

interface ServerSettingsProps { onRegisterSaveButton?: (button: React.ReactNode) => void; }
type SettingsTab = 'basic' | 'advanced' | 'ca';
interface GatewaySettings {
  id?: number;
  vpn_name: string;
  server_endpoint: string;
  server_identity: string;
  mobike_enabled: boolean;
  force_encap_natt: boolean;
  cipher_preset: string;
  ike_proposals: string;
  esp_proposals: string;
  advanced_connection?: Record<string, unknown> | null;
  advanced_child?: Record<string, unknown> | null;
  updated_at?: string;
  [key: string]: unknown;
}
interface CAIdentity {
  common_name: string; organization: string; country: string;
  province: string; locality: string; serial: string; not_after: string;
}
interface CAResponse { ca: CAIdentity; can_rotate: boolean; }
interface RuntimeApplyStatus {
  status: 'pending' | 'applying' | 'applied' | 'failed';
  desired_generation: number;
  applied_generation: number;
  last_error?: string;
  last_applied_at?: string | null;
}

const presets: Record<string, { ike: string; esp: string }> = {
  modern: { ike: 'aes256gcm16-prfsha384-ecp384,chacha20poly1305-prfsha256-curve25519', esp: 'aes256gcm16,chacha20poly1305' },
  balanced: { ike: 'aes256gcm16-prfsha384-ecp384,aes256-sha256-modp2048', esp: 'aes256gcm16,aes128gcm16,chacha20poly1305' },
};
function authHeaders(): HeadersInit { return { Authorization: 'Bearer ' + (localStorage.getItem('token') || '') }; }
async function getJSON<T>(url: string): Promise<T> {
  const response = await fetch(url, { headers: authHeaders(), cache: 'no-store' });
  if (!response.ok) throw new Error('Request failed (HTTP ' + response.status + ')');
  return response.json() as Promise<T>;
}
async function submitJSON<T>(url: string, method: 'PUT' | 'POST', value: unknown): Promise<T> {
  const response = await fetch(url, {
    method,
    headers: { ...authHeaders(), 'Content-Type': 'application/json' },
    body: JSON.stringify(value),
  });
  const result = await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(result.error || 'Request failed (HTTP ' + response.status + ')');
  return result as T;
}
const inputClass = 'w-full rounded-lg border subtle-border surface-input px-3 py-2 text-xs focus:outline-none focus:border-blue-500';
const labelClass = 'block mb-1.5 font-medium text-xs text-subtle';

export const ServerSettingsPage: React.FC<ServerSettingsProps> = ({ onRegisterSaveButton }) => {
  const queryClient = useQueryClient();
  const { t } = useTranslation();
  const { requestConfirmation, confirmationDialog } = useConfirmAction();
  const [tab, setTab] = useState<SettingsTab>('basic');
  const [form, setForm] = useState({
    vpn_name: 'SwanPanel VPN', server_endpoint: '', server_identity: '',
    mobike_enabled: true, force_encap_natt: false, cipher_preset: 'balanced',
  });
  const [advancedText, setAdvancedText] = useState('{}');
  const [caForm, setCAForm] = useState({ common_name: '', organization: '', country: '', province: '', locality: '' });
  const [confirmation, setConfirmation] = useState('');
  const [notice, setNotice] = useState('');

  const settings = useQuery<GatewaySettings>({
    queryKey: ['strongswan_settings'], queryFn: () => getJSON('/api/settings/strongswan'),
  });
  const ca = useQuery<CAResponse>({
    queryKey: ['ca_identity'], queryFn: () => getJSON('/api/settings/ca'), enabled: tab === 'ca',
  });
  const runtimeStatus = useQuery<RuntimeApplyStatus>({
    queryKey: ['runtime_apply_status'], queryFn: () => getJSON('/api/system/apply-status'),
  });
  const retryApply = useMutation({
    mutationFn: () => submitJSON('/api/system/apply', 'POST', {}),
    onSuccess: async () => {
      setNotice('Gateway reconciliation completed.');
      await queryClient.invalidateQueries({ queryKey: ['runtime_apply_status'] });
      await queryClient.invalidateQueries({ queryKey: ['dashboard_stats'] });
    },
  });
  useEffect(() => {
    const s = settings.data;
    if (!s) return;
    setForm({
      vpn_name: s.vpn_name || 'SwanPanel VPN',
      server_endpoint: s.server_endpoint || '',
      server_identity: s.server_identity || '',
      mobike_enabled: s.mobike_enabled,
      force_encap_natt: s.force_encap_natt,
      cipher_preset: s.cipher_preset || 'balanced',
    });
    const { id: _id, updated_at: _updated, dpd_timeout: _unsupported, ...editable } = s;
    setAdvancedText(JSON.stringify({
      ...editable, advanced_connection: s.advanced_connection || {}, advanced_child: s.advanced_child || {},
    }, null, 2));
  }, [settings.data]);
  useEffect(() => {
    if (!ca.data?.ca) return;
    const value = ca.data.ca;
    setCAForm({
      common_name: value.common_name, organization: value.organization,
      country: value.country, province: value.province, locality: value.locality,
    });
  }, [ca.data]);
  useEffect(() => {
    onRegisterSaveButton?.(null);
    return () => onRegisterSaveButton?.(null);
  }, [onRegisterSaveButton]);

  const save = useMutation({
    mutationFn: (payload: Record<string, unknown>) =>
      submitJSON<{ runtime_status: string; error?: string }>('/api/settings/strongswan', 'PUT', payload),
    onMutate: () => setNotice(''),
    onSuccess: async result => {
      setNotice(result.runtime_status === 'applied'
        ? 'Configuration saved and applied to the gateway.'
        : 'Configuration saved, gateway reconciliation pending: ' + (result.error || ''));
      await queryClient.invalidateQueries({ queryKey: ['strongswan_settings'] });
      await queryClient.invalidateQueries({ queryKey: ['runtime_apply_status'] });
    },
  });
  const rotate = useMutation({
    mutationFn: () => submitJSON<{ runtime_status: string }>('/api/settings/ca/rotate', 'POST', { ...caForm, confirm: confirmation }),
    onMutate: () => setNotice(''),
    onSuccess: async result => {
      setConfirmation('');
      setNotice(result.runtime_status === 'applied'
        ? 'New Root CA issued and gateway updated. Keep the old CA backup securely.'
        : 'New Root CA issued. Gateway configuration is pending; use Retry apply to complete.');
      await queryClient.invalidateQueries({ queryKey: ['ca_identity'] });
    },
  });
  const advanced = useMemo(() => {
    try {
      const value: unknown = JSON.parse(advancedText);
      if (!value || typeof value !== 'object' || Array.isArray(value)) {
        return { error: 'Root must be a JSON object', value: null };
      }
      return { error: '', value: value as Record<string, unknown> };
    } catch (error) {
      return { error: error instanceof Error ? error.message : 'Invalid JSON', value: null };
    }
  }, [advancedText]);
  const submitBasic = () => {
    if (!settings.data) return;
    const selected = presets[form.cipher_preset];
    save.mutate({
      ...settings.data, ...form,
      ...(selected ? { ike_proposals: selected.ike, esp_proposals: selected.esp } : {}),
    });
  };

  return (
    <div className="space-y-4">
      {confirmationDialog}
      <nav className="flex flex-wrap gap-2" aria-label="Gateway settings sections">
        {([
          ['basic', SlidersHorizontal, 'Basic settings'],
          ['advanced', FileJson2, 'Advanced JSON'],
          ['ca', ShieldCheck, 'Certificate Authority'],
        ] as const).map(([key, Icon, label]) => (
          <button type="button" key={key} onClick={() => { setTab(key); setNotice(''); }}
            aria-current={tab === key ? 'page' : undefined}
            className={'flex items-center gap-2 border rounded-lg px-3 py-2 text-xs font-medium ' +
              (tab === key ? 'border-blue-500 bg-blue-500/10 text-blue-500' : 'subtle-border text-subtle hover:text-current')}>
            <Icon className="h-4 w-4" />{label}
          </button>
        ))}
      </nav>
      {settings.isLoading && <p role="status" className="text-xs text-dim">Loading gateway settings…</p>}
      {settings.isError && <p role="alert" className="text-xs text-red-500">Unable to load gateway settings.</p>}
      {notice && <p role="status" className="text-xs p-3 border subtle-border rounded-lg">{notice}</p>}
      <section className="rounded-lg border subtle-border surface-card px-4 py-3 flex flex-wrap items-center justify-between gap-3" aria-label="Gateway apply status">
        <div className="space-y-1">
          <div className="text-xs font-semibold">Gateway configuration status: {runtimeStatus.data?.status ?? 'unknown'}</div>
          {runtimeStatus.data?.last_error && runtimeStatus.data.status !== 'applied' &&
            <p role="alert" className="text-xs text-red-500">{runtimeStatus.data.last_error}</p>}
          {runtimeStatus.isError && <p role="alert" className="text-xs text-red-500">Could not verify the gateway configuration status.</p>}
          {runtimeStatus.data?.status === 'applied' && (
            <p className="text-[11px] text-emerald-500">{t('server.managed_config_applied')}</p>
          )}
          {runtimeStatus.data?.last_applied_at && <p className="text-[11px] text-dim">
            Last successful apply: {new Date(runtimeStatus.data.last_applied_at).toLocaleString()}
          </p>}
        </div>
        <div className="flex gap-2">
          <button type="button" onClick={() => { void runtimeStatus.refetch(); }}
            className="rounded-lg border subtle-border px-3 py-2 text-xs">Refresh status</button>
          {(runtimeStatus.data?.status === 'pending' || runtimeStatus.data?.status === 'failed') &&
            <button type="button" disabled={retryApply.isPending} onClick={() => retryApply.mutate()}
              className="rounded-lg bg-blue-600 px-3 py-2 text-xs font-medium text-white disabled:opacity-50">
              {retryApply.isPending ? 'Retrying…' : 'Retry apply'}
            </button>}
        </div>
      </section>
      {retryApply.isError && <p role="alert" className="text-xs text-red-500">{retryApply.error.message}</p>}
      {save.isError && <p role="alert" className="text-xs text-red-500">{save.error.message}</p>}
      {rotate.isError && <p role="alert" className="text-xs text-red-500">{rotate.error.message}</p>}

      {tab === 'basic' && settings.data && (
        <section className="surface-card border subtle-border rounded-xl p-5 space-y-5 max-w-3xl">
          <div>
            <h2 className="text-sm font-semibold">Common VPN settings</h2>
            <p className="text-xs text-dim mt-1">Only the settings commonly changed during administration. All other supported options are in Advanced JSON.</p>
          </div>
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <label className="block md:col-span-2">
              <span className={labelClass}>VPN display name</span>
              <input className={inputClass} value={form.vpn_name} maxLength={128}
                onChange={e => setForm(s => ({ ...s, vpn_name: e.target.value }))} placeholder="Company VPN" />
              <span className="text-[11px] text-dim">Used in newly exported client profiles. Installed profiles are not renamed.</span>
            </label>
            <label>
              <span className={labelClass}>VPN server address (IP or hostname)</span>
              <input className={inputClass} value={form.server_endpoint}
                onChange={e => setForm(s => ({ ...s, server_endpoint: e.target.value }))} placeholder="vpn.example.com" />
            </label>
            <label>
              <span className={labelClass}>Server certificate identity</span>
              <input className={inputClass} value={form.server_identity}
                onChange={e => setForm(s => ({ ...s, server_identity: e.target.value }))} placeholder="vpn.example.com" />
              <span className="text-[11px] text-dim">Must match the certificate SAN, even when connecting by IP.</span>
            </label>
            <label>
              <span className={labelClass}>Cryptography preset</span>
              <select className={inputClass} value={form.cipher_preset}
                onChange={e => setForm(s => ({ ...s, cipher_preset: e.target.value }))}>
                <option value="balanced">Balanced</option>
                <option value="modern">Modern</option>
                <option value="custom">Custom (edit proposals in Advanced)</option>
              </select>
            </label>
            <div className="space-y-3">
              <label className="flex items-center gap-2 text-xs">
                <input type="checkbox" checked={form.mobike_enabled}
                  onChange={e => setForm(s => ({ ...s, mobike_enabled: e.target.checked }))} />
                Enable MOBIKE (network roaming)
              </label>
              <label className="flex items-center gap-2 text-xs">
                <input type="checkbox" checked={form.force_encap_natt}
                  onChange={e => setForm(s => ({ ...s, force_encap_natt: e.target.checked }))} />
                Force UDP encapsulation (NAT-T)
              </label>
            </div>
          </div>
          <button type="button" disabled={save.isPending || !form.server_endpoint.trim() || !form.server_identity.trim() || !form.vpn_name.trim()}
            onClick={submitBasic} className="rounded-lg bg-blue-600 px-4 py-2 text-xs font-semibold text-white disabled:opacity-50">
            <span className="inline-flex items-center gap-2"><Check className="h-4 w-4" />{save.isPending ? 'Applying…' : 'Save settings'}</span>
          </button>
        </section>
      )}

      {tab === 'advanced' && settings.data && (
        <section className="surface-card border subtle-border rounded-xl p-5 space-y-4">
          <div>
            <h2 className="text-sm font-semibold flex items-center gap-2"><Code2 className="h-4 w-4" />Advanced strongSwan configuration</h2>
            <p className="text-xs text-dim mt-1">
              Edit the full gateway settings JSON. Add extra VICI options to <code>advanced_connection</code> or <code>advanced_child</code>.
              Authentication, peer identity, address pools and traffic selectors are controlled by SwanPanel and cannot be overridden.
            </p>
          </div>
          <textarea aria-label="Advanced gateway JSON configuration" spellCheck={false} value={advancedText}
            onChange={e => setAdvancedText(e.target.value)} rows={22}
            className="w-full font-mono text-xs leading-5 rounded-lg border subtle-border surface-input p-4 focus:outline-none focus:border-blue-500" />
          {advanced.error
            ? <p role="alert" className="text-xs text-red-500">{advanced.error}</p>
            : <p role="status" className="text-xs text-emerald-500">Valid JSON</p>}
          <div className="flex flex-wrap gap-2">
            <button type="button" onClick={() => { if (advanced.value) setAdvancedText(JSON.stringify(advanced.value, null, 2)); }}
              disabled={!advanced.value} className="border subtle-border rounded-lg px-3 py-2 text-xs disabled:opacity-50">Format JSON</button>
            <button type="button" onClick={() => {
              const { id: _id, updated_at: _updated, dpd_timeout: _unsupported, ...editable } = settings.data!;
              setAdvancedText(JSON.stringify({
                ...editable, advanced_connection: settings.data!.advanced_connection || {},
                advanced_child: settings.data!.advanced_child || {},
              }, null, 2));
            }} className="border subtle-border rounded-lg px-3 py-2 text-xs inline-flex gap-2 items-center">
              <RotateCcw className="h-3.5 w-3.5" />Reset to saved
            </button>
            <button type="button" disabled={save.isPending || !advanced.value}
              onClick={() => {
                if (!advanced.value) return;
                const value = advanced.value;
                requestConfirmation({
                  title: t('confirm.apply_advanced_title'),
                  description: t('confirm.apply_advanced_description'),
                  confirmLabel: t('common.save_changes'),
                  tone: 'warning',
                  onConfirm: () => save.mutateAsync(value),
                });
              }}
              className="rounded-lg bg-blue-600 px-4 py-2 text-xs font-semibold text-white disabled:opacity-50">
              {save.isPending ? 'Applying…' : 'Validate & apply JSON'}
            </button>
          </div>
          <p className="text-[11px] text-dim">Unknown top-level fields are rejected by the server. Invalid or unsupported VICI options fail reconciliation and will not be shown as applied.</p>
        </section>
      )}

      {tab === 'ca' && (
        <section className="surface-card border subtle-border rounded-xl p-5 space-y-4 max-w-3xl">
          <div>
            <h2 className="font-semibold text-sm">VPN Root Certificate Authority</h2>
            <p className="text-xs text-dim mt-1">Values embedded in the signed Root CA certificate, not merely UI labels. A CA subject cannot be edited in place.</p>
          </div>
          {ca.isLoading && <p className="text-xs">Loading certificate identity…</p>}
          {ca.isError && <p role="alert" className="text-red-500 text-xs">Unable to load CA information.</p>}
          {ca.data && (
            <>
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                {([
                  ['common_name', 'CA common name'],
                  ['organization', 'Organization'],
                  ['country', 'Country (2 letters)'],
                  ['province', 'Province / state'],
                  ['locality', 'City / locality'],
                ] as const).map(([field, title]) => (
                  <label key={field}>
                    <span className={labelClass}>{title}</span>
                    <input className={inputClass} value={caForm[field]} disabled={!ca.data.can_rotate}
                      onChange={e => setCAForm(s => ({ ...s, [field]: e.target.value }))} />
                  </label>
                ))}
              </div>
              <p className="text-[11px] text-dim break-all">
                Certificate serial: {ca.data.ca.serial} · Expires: {new Date(ca.data.ca.not_after).toLocaleDateString()}
              </p>
              {ca.data.can_rotate ? (
                <div className="border border-amber-500/40 rounded-lg p-3 space-y-3">
                  <p className="text-xs text-amber-500">
                    Issuing a new Root CA invalidates trust in the previous CA. This is only permitted before the first device enrollment.
                    Old CA material is retained as a backup.
                  </p>
                  <label className="block">
                    <span className={labelClass}>Type ROTATE CA to confirm</span>
                    <input className={inputClass} value={confirmation} onChange={e => setConfirmation(e.target.value)} />
                  </label>
                  <button type="button" disabled={confirmation !== 'ROTATE CA' || rotate.isPending}
                    onClick={() => requestConfirmation({
                      title: t('confirm.rotate_ca_title'),
                      description: t('confirm.rotate_ca_description', { name: caForm.common_name }),
                      confirmLabel: 'Issue new Root CA',
                      onConfirm: () => rotate.mutateAsync(),
                    })} className="rounded-lg border border-red-500 text-red-500 px-4 py-2 text-xs font-medium disabled:opacity-50">
                    {rotate.isPending ? 'Rotating…' : 'Issue new Root CA'}
                  </button>
                </div>
              ) : (
                <p className="text-xs text-dim border subtle-border p-3 rounded-lg">
                  CA identity is locked because device or revocation records exist. A separate, explicitly planned CA migration is required.
                </p>
              )}
            </>
          )}
        </section>
      )}
    </div>
  );
};
