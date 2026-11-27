import React, { useEffect, useMemo, useRef, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { ArchiveRestore, Check, ChevronDown, Code2, Download, Eye, EyeOff, FileJson2, RotateCcw, ShieldCheck, SlidersHorizontal, Upload } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useConfirmAction } from '../components/ConfirmDialog';
import { CopyButton } from '../components/CopyButton';
import { AdvancedJSONEditor } from '../components/AdvancedJSONEditor';
import { apiJSON } from '../lib/api';
import { downloadEncryptedBackup, RestoreOutcomeError, uploadRestoreBackup, waitForControlPlaneRestart } from '../lib/backupApi';
import { checkGatewayJSON, diffGatewayJSON } from '../lib/gatewayJSON';

interface ServerSettingsProps { onRegisterSaveButton?: (button: React.ReactNode) => void; }
type SettingsTab = 'basic' | 'advanced' | 'ca' | 'backup';
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
function getJSON<T>(url: string): Promise<T> { return apiJSON<T>(url); }
function submitJSON<T>(url: string, method: 'PUT' | 'POST', value: unknown): Promise<T> {
  return apiJSON<T>(url, { method, body: JSON.stringify(value) });
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
  const [basicDirty, setBasicDirty] = useState(false);
  const [advancedDirty, setAdvancedDirty] = useState(false);
  const saveSource = useRef<'basic' | 'advanced'>('basic');
  const [caForm, setCAForm] = useState({ common_name: '', organization: '', country: '', province: '', locality: '' });
  const [confirmation, setConfirmation] = useState('');
  const [notice, setNotice] = useState('');
  const [exportPass, setExportPass] = useState('');
  const [exportConfirmPass, setExportConfirmPass] = useState('');
  const [showExportPass, setShowExportPass] = useState(false);
  const [exportTouched, setExportTouched] = useState({ pass: false, confirm: false });
  const [exportSubmitAttempted, setExportSubmitAttempted] = useState(false);
  const [restoreFile, setRestoreFile] = useState<File | null>(null);
  const [restorePass, setRestorePass] = useState('');
  const [showRestorePass, setShowRestorePass] = useState(false);
  const [restoreConfirmText, setRestoreConfirmText] = useState('');
  const [restoreTouched, setRestoreTouched] = useState({ file: false, pass: false, confirm: false });
  const [restoreSubmitAttempted, setRestoreSubmitAttempted] = useState(false);
  const [restoreRestarting, setRestoreRestarting] = useState(false);
  const exportPassRef = useRef<HTMLInputElement>(null);
  const exportConfirmRef = useRef<HTMLInputElement>(null);
  const restoreFileRef = useRef<HTMLInputElement>(null);
  const restorePassRef = useRef<HTMLInputElement>(null);
  const restoreConfirmRef = useRef<HTMLInputElement>(null);

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
      setNotice(t('server.reconciliation_succeeded'));
      await queryClient.invalidateQueries({ queryKey: ['runtime_apply_status'] });
      await queryClient.invalidateQueries({ queryKey: ['dashboard_stats'] });
    },
  });
  useEffect(() => {
    const s = settings.data;
    if (!s) return;
    if (!basicDirty) setForm({
      vpn_name: s.vpn_name || 'SwanPanel VPN',
      server_endpoint: s.server_endpoint || '',
      server_identity: s.server_identity || '',
      mobike_enabled: s.mobike_enabled,
      force_encap_natt: s.force_encap_natt,
      cipher_preset: s.cipher_preset || 'balanced',
    });
    const { id: _id, updated_at: _updated, dpd_timeout: _unsupported, ...editable } = s;
    if (!advancedDirty) setAdvancedText(JSON.stringify({
      ...editable, advanced_connection: s.advanced_connection || {}, advanced_child: s.advanced_child || {},
    }, null, 2));
  }, [settings.data, basicDirty, advancedDirty]);
  useEffect(() => {
    if (!ca.data?.ca) return;
    const value = ca.data.ca;
    setCAForm({
      common_name: value.common_name, organization: value.organization,
      country: value.country, province: value.province, locality: value.locality,
    });
  }, [ca.data]);
  const save = useMutation({
    mutationFn: (payload: Record<string, unknown>) =>
      submitJSON<{ runtime_status: string; error?: string }>('/api/settings/strongswan', 'PUT', payload),
    onMutate: () => setNotice(''),
    onSuccess: async result => {
      if (saveSource.current === 'basic') setBasicDirty(false);
      else setAdvancedDirty(false);
      setNotice(result.runtime_status === 'applied'
        ? t('server.settings_applied')
        : t('server.settings_pending', { reason: result.error || t('server.status_unknown') }));
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
        ? t('server.ca_rotated')
        : t('server.ca_rotation_pending'));
      await queryClient.invalidateQueries({ queryKey: ['ca_identity'] });
    },
  });
  const exportBackup = useMutation({
    mutationFn: (passphrase: string) => downloadEncryptedBackup(passphrase),
    onMutate: () => setNotice(''),
    onSuccess: filename => {
      setExportPass('');
      setExportConfirmPass('');
      setExportTouched({ pass: false, confirm: false });
      setExportSubmitAttempted(false);
      setNotice(t('server.backup.export_success', { filename }));
    },
  });
  const restoreBackup = useMutation({
    mutationFn: ({ file, passphrase, confirm }: { file: File; passphrase: string; confirm: string }) =>
      uploadRestoreBackup(file, passphrase, confirm),
    onMutate: () => setNotice(''),
    onSuccess: async result => {
      setRestoreFile(null);
      setRestorePass('');
      setRestoreConfirmText('');
      setRestoreTouched({ file: false, pass: false, confirm: false });
      setRestoreSubmitAttempted(false);
      if (restoreFileRef.current) restoreFileRef.current.value = '';
      if (result.runtime_status === 'restarting') {
        setRestoreRestarting(true);
        setNotice(t('server.backup.restore_restarting'));
        try {
          await waitForControlPlaneRestart(30, 2000, fetch, result.restore_id);
          localStorage.removeItem('token');
          localStorage.removeItem('user');
          setNotice(t('server.backup.restore_restarted_reauth'));
          window.setTimeout(() => {
            window.location.reload();
          }, 1200);
        } catch (err) {
          setRestoreRestarting(false);
          if (err instanceof RestoreOutcomeError && err.outcome === 'rolled_back') {
            setNotice(t('server.backup.restore_rolled_back', { reason: err.message }));
            await queryClient.invalidateQueries();
          } else if (err instanceof RestoreOutcomeError && err.outcome === 'failed_closed') {
            setNotice(t('server.backup.restore_failed_closed'));
          } else if (err instanceof RestoreOutcomeError && err.outcome === 'unverified') {
            setNotice(t('server.backup.restore_outcome_unverified'));
          } else {
            setNotice(t('server.backup.restore_restart_timeout'));
          }
        }
        return;
      }
      setNotice(result.runtime_status === 'applied'
        ? t('server.backup.restore_applied')
        : t('server.backup.restore_pending', { reason: result.message || t('server.status_unknown') }));
      await queryClient.invalidateQueries();
    },
  });

  const exportPassError = (exportTouched.pass || exportSubmitAttempted) && (exportPass.length < 12 || exportPass.length > 128)
    ? t('server.backup.passphrase_length_error') : '';
  const exportConfirmError = (exportTouched.confirm || exportSubmitAttempted) && exportConfirmPass !== exportPass
    ? t('server.backup.passphrase_mismatch') : '';
  const restoreFileError = (restoreTouched.file || restoreSubmitAttempted) && !restoreFile
    ? t('server.backup.file_required') : '';
  const restorePassError = (restoreTouched.pass || restoreSubmitAttempted) && (restorePass.length < 12 || restorePass.length > 128)
    ? t('server.backup.passphrase_length_error') : '';
  const restoreConfirmError = (restoreTouched.confirm || restoreSubmitAttempted) && restoreConfirmText.trim() !== 'RESTORE BACKUP'
    ? t('server.backup.restore_confirm_error') : '';

  const submitExportBackup = (e: React.FormEvent) => {
    e.preventDefault();
    setExportSubmitAttempted(true);
    exportBackup.reset();
    if (exportPass.length < 12 || exportPass.length > 128) {
      exportPassRef.current?.focus();
      return;
    }
    if (exportConfirmPass !== exportPass) {
      exportConfirmRef.current?.focus();
      return;
    }
    exportBackup.mutate(exportPass);
  };

  const submitRestoreBackup = (e: React.FormEvent) => {
    e.preventDefault();
    setRestoreSubmitAttempted(true);
    restoreBackup.reset();
    if (!restoreFile) {
      restoreFileRef.current?.focus();
      return;
    }
    if (restorePass.length < 12 || restorePass.length > 128) {
      restorePassRef.current?.focus();
      return;
    }
    if (restoreConfirmText.trim() !== 'RESTORE BACKUP') {
      restoreConfirmRef.current?.focus();
      return;
    }
    const targetFile = restoreFile;
    requestConfirmation({
      title: t('confirm.restore_backup_title'),
      description: t('confirm.restore_backup_description', { name: targetFile.name }),
      confirmLabel: t('server.backup.restore_action'),
      onConfirm: () => restoreBackup.mutateAsync({
        file: targetFile,
        passphrase: restorePass,
        confirm: restoreConfirmText.trim(),
      }),
    });
  };
  const advanced = useMemo(() => {
    try {
      const value: unknown = JSON.parse(advancedText);
      if (!value || typeof value !== 'object' || Array.isArray(value)) {
        return { error: t('server.json_object_required'), value: null };
      }
      const issue = checkGatewayJSON(value as Record<string, unknown>);
      if (issue) return { error: t(issue.expected === 'supported' ? 'server.settings.json_unknown_field' : 'server.settings.json_invalid_field', { field: issue.field, expected: issue.expected }), value: null };
      return { error: '', value: value as Record<string, unknown> };
    } catch (error) {
      return { error: error instanceof Error ? error.message : t('server.invalid_json'), value: null };
    }
  }, [advancedText, t]);
  const savedAdvanced = useMemo(() => {
    if (!settings.data) return null;
    const { id: _id, updated_at: _updated, dpd_timeout: _unsupported, ...editable } = settings.data;
    return { ...editable, advanced_connection: settings.data.advanced_connection || {}, advanced_child: settings.data.advanced_child || {} };
  }, [settings.data]);
  const advancedChanges = useMemo(() => savedAdvanced && advanced.value
    ? diffGatewayJSON(savedAdvanced, advanced.value) : [], [savedAdvanced, advanced.value]);
  const gatewayStatus = runtimeStatus.data?.status;
  const gatewayApplied = gatewayStatus === 'applied' &&
    runtimeStatus.data?.desired_generation === runtimeStatus.data?.applied_generation;
  const displayedGatewayStatus = gatewayApplied ? 'applied' :
    gatewayStatus === 'applied' ? 'pending' : gatewayStatus;
  const activeDirty = tab === 'basic' ? basicDirty : tab === 'advanced' && advancedDirty;

  const submitBasic = () => {
    if (!settings.data || save.isPending) return;
    saveSource.current = 'basic';
    const selected = presets[form.cipher_preset];
    save.mutate({
      ...settings.data, ...form,
      ...(selected ? { ike_proposals: selected.ike, esp_proposals: selected.esp } : {}),
    });
  };

  const submitAdvanced = () => {
    if (!advanced.value || !advancedChanges.length) return;
    requestConfirmation({
      title: t('confirm.apply_advanced_title'),
      description: t('confirm.apply_advanced_description') + ' ' + t('server.settings.advanced_change_summary', { count: advancedChanges.length }),
      confirmLabel: t('server.settings.save_and_apply'),
      tone: 'warning',
      onConfirm: () => { saveSource.current = 'advanced'; return save.mutateAsync(advanced.value!); },
    });
  };

  useEffect(() => {
    if (!onRegisterSaveButton) return;
    if (!settings.data || tab === 'ca' || tab === 'backup') {
      onRegisterSaveButton(null);
      return;
    }
    const blocked = save.isPending || retryApply.isPending || !activeDirty || (tab === 'basic'
      ? !form.server_endpoint.trim() || !form.server_identity.trim() || !form.vpn_name.trim()
      : !advanced.value || advancedChanges.length === 0);
    onRegisterSaveButton(
      <button type="button" disabled={blocked}
        onClick={tab === 'basic' ? submitBasic : submitAdvanced}
        className="rounded-lg bg-blue-600 hover:bg-blue-500 text-white px-3 py-2 text-xs font-semibold disabled:opacity-50 inline-flex items-center gap-1.5">
        <Check className="h-4 w-4" />
        {save.isPending ? t('server.settings.applying') : t('server.settings.save_and_apply')}
      </button>
    );
    return () => onRegisterSaveButton(null);
  }, [onRegisterSaveButton, settings.data, tab, save.isPending, retryApply.isPending, form, advanced.value, advancedChanges.length, activeDirty, t]);

  return (
    <div className="space-y-4">
      {confirmationDialog}
      <nav className="flex flex-wrap gap-2" aria-label={t('server.settings_sections')}>
        {([
          ['basic', SlidersHorizontal, t('server.settings.basic_tab')],
          ['advanced', FileJson2, t('server.settings.advanced_tab')],
          ['ca', ShieldCheck, t('server.settings.ca_tab')],
          ['backup', ArchiveRestore, t('server.settings.backup_tab')],
        ] as const).map(([key, Icon, label]) => (
          <button type="button" key={key} onClick={() => {
            setTab(key);
            setNotice('');
            if (key !== 'backup') {
              setExportPass('');
              setExportConfirmPass('');
              setRestorePass('');
              setRestoreConfirmText('');
            }
          }}
            aria-current={tab === key ? 'page' : undefined}
            className={'flex items-center gap-2 border rounded-lg px-3 py-2 text-xs font-medium ' +
              (tab === key ? 'border-blue-500 bg-blue-500/10 text-accent' : 'subtle-border text-subtle hover:text-current')}>
            <Icon className="h-4 w-4" />{label}
          </button>
        ))}
      </nav>
      {settings.isLoading && <p role="status" className="text-xs text-dim">{t('server.settings.loading_gateway')}</p>}
      {settings.isError && <p role="alert" className="text-xs text-status-danger">{t('server.settings.error_gateway')}</p>}
      {notice && <p role="status" className="text-xs p-3 border subtle-border rounded-lg">{notice}</p>}
      {activeDirty && <p role="status" className="border border-amber-500/30 bg-amber-500/10 text-xs text-status-warning rounded-lg p-3">
        {t('server.settings.unsaved_changes')}
      </p>}
      <section className="rounded-lg border subtle-border surface-card px-4 py-3 flex flex-wrap items-center justify-between gap-3" aria-label={t('server.apply_status_heading')}>
        <div className="space-y-2 min-w-0">
          <h2 className="text-xs font-semibold">{t('server.apply_status_heading')}</h2>
          <p className="text-[11px] text-subtle">{t('server.settings.lifecycle_description')}</p>
          <div className="flex flex-wrap items-center gap-2 text-xs">
            <span className={`rounded-md border px-2 py-1 font-semibold ${gatewayApplied
              ? 'border-emerald-500/30 bg-emerald-500/10 text-status-success'
              : displayedGatewayStatus === 'failed'
                ? 'border-rose-500/30 bg-rose-500/10 text-status-danger'
                : 'border-amber-500/30 bg-amber-500/10 text-status-warning'}`}>
              {displayedGatewayStatus ? t(`server.settings.state_${displayedGatewayStatus}`) : t('server.status_unknown')}
            </span>
            {runtimeStatus.data && <>
              <span className="text-subtle">{t('server.settings.saved_revision', { number: runtimeStatus.data.desired_generation })}</span>
              <span className="text-subtle">{t('server.settings.active_revision', { number: runtimeStatus.data.applied_generation })}</span>
            </>}
          </div>
          {runtimeStatus.data?.last_error && runtimeStatus.data.status !== 'applied' &&
            <p role="alert" className="text-xs text-status-danger">{runtimeStatus.data.last_error}</p>}
          {runtimeStatus.isError && <p role="alert" className="text-xs text-status-danger">{t('server.settings.status_error')}</p>}
          {gatewayApplied && (
            <p className="text-[11px] text-status-success">{t('server.managed_config_applied')}</p>
          )}
          {displayedGatewayStatus === 'pending' && (
            <p className="text-[11px] text-status-warning">{t('server.settings.saved_pending_explanation')}</p>
          )}
          {runtimeStatus.data?.last_applied_at && <p className="text-[11px] text-dim">
            {t('server.settings.last_applied')}: {new Date(runtimeStatus.data.last_applied_at).toLocaleString()}
          </p>}
        </div>
        <div className="flex gap-2">
          <button type="button" onClick={() => { void runtimeStatus.refetch(); }}
            className="rounded-lg border subtle-border px-3 py-2 text-xs">{t('server.settings.refresh_status')}</button>
          {(displayedGatewayStatus === 'pending' || displayedGatewayStatus === 'failed') &&
            <button type="button" disabled={retryApply.isPending || save.isPending} onClick={() => retryApply.mutate()}
              className="rounded-lg bg-blue-600 px-3 py-2 text-xs font-medium text-white disabled:opacity-50">
              {retryApply.isPending ? t('server.settings.retrying') : t('server.settings.retry_apply')}
            </button>}
        </div>
      </section>
      {retryApply.isError && <p role="alert" className="text-xs text-status-danger">{retryApply.error.message}</p>}
      {save.isError && <p role="alert" className="text-xs text-status-danger">{save.error.message}</p>}
      {rotate.isError && <p role="alert" className="text-xs text-status-danger">{rotate.error.message}</p>}

      {tab === 'basic' && settings.data && (
        <section className="surface-card border subtle-border rounded-xl p-5 space-y-5 max-w-3xl">
          <div>
            <h2 className="text-sm font-semibold">{t('server.settings.common_vpn_settings')}</h2>
            <p className="text-xs text-dim mt-1">{t('server.settings.common_vpn_description')}</p>
          </div>
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <label className="block md:col-span-2">
              <span className={labelClass}>{t('server.settings.vpn_display_name')}</span>
              <input className={inputClass} value={form.vpn_name} maxLength={128}
                onChange={e => { setBasicDirty(true); setForm(s => ({ ...s, vpn_name: e.target.value })); }} placeholder="Company VPN" />
              <span className="text-[11px] text-dim">{t('server.settings.vpn_display_hint')}</span>
            </label>
            <label>
              <span className={labelClass}>{t('server.settings.server_address')}</span>
              <input className={inputClass} value={form.server_endpoint}
                onChange={e => { setBasicDirty(true); setForm(s => ({ ...s, server_endpoint: e.target.value })); }} placeholder="vpn.example.com" />
            </label>
            <label>
              <span className={labelClass}>{t('server.settings.certificate_identity')}</span>
              <input className={inputClass} value={form.server_identity}
                onChange={e => { setBasicDirty(true); setForm(s => ({ ...s, server_identity: e.target.value })); }} placeholder="vpn.example.com" />
              <span className="text-[11px] text-dim">{t('server.settings.certificate_hint')}</span>
            </label>
            <label>
              <span className={labelClass}>{t('server.settings.crypto_preset')}</span>
              <select className={inputClass} value={form.cipher_preset}
                onChange={e => { setBasicDirty(true); setForm(s => ({ ...s, cipher_preset: e.target.value })); }}>
                <option value="balanced">{t('server.settings.preset_balanced')}</option>
                <option value="modern">{t('server.settings.preset_modern')}</option>
                <option value="custom">{t('server.settings.preset_custom')}</option>
              </select>
            </label>
            <details className="group md:col-span-2 rounded-lg border subtle-border px-3 py-2">
              <summary className="cursor-pointer list-none flex items-center justify-between gap-2 text-xs font-semibold text-subtle">
                {t('server.settings.connection_options')}
                <ChevronDown aria-hidden="true" className="h-4 w-4 shrink-0 transition-transform group-open:rotate-180" />
              </summary>
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 pt-3 mt-3 border-t subtle-border">
                <label className="flex items-center gap-2 text-xs">
                  <input type="checkbox" checked={form.mobike_enabled}
                    onChange={e => { setBasicDirty(true); setForm(s => ({ ...s, mobike_enabled: e.target.checked })); }} />
                  {t('server.settings.mobike')}
                </label>
                <label className="flex items-center gap-2 text-xs">
                  <input type="checkbox" checked={form.force_encap_natt}
                    onChange={e => { setBasicDirty(true); setForm(s => ({ ...s, force_encap_natt: e.target.checked })); }} />
                  {t('server.settings.nat_t')}
                </label>
              </div>
            </details>
          </div>
          <p className="text-[11px] text-subtle">{t('server.settings.header_save_hint')}</p>
        </section>
      )}

      {tab === 'advanced' && settings.data && (
        <section className="surface-card border subtle-border rounded-xl p-5 space-y-4">
          <div>
            <h2 className="text-sm font-semibold flex items-center gap-2"><Code2 className="h-4 w-4" />{t('server.settings.advanced_title')}</h2>
            <p className="text-xs text-dim mt-1">
              {t('server.settings.advanced_description')}
            </p>
          </div>
          <AdvancedJSONEditor label={t('server.settings.advanced_title')} value={advancedText}
            onChange={value => { setAdvancedDirty(true); setAdvancedText(value); }} error={advanced.error} />
          {advanced.error
            ? <p id="advanced-json-error" role="alert" className="text-xs text-status-danger">{advanced.error}</p>
            : <p role="status" className="text-xs text-status-success">{t('server.settings.valid_json')}</p>}
          {advanced.value && advancedChanges.length > 0 && (
            <div className="rounded-lg border subtle-border overflow-hidden" role="region" aria-label={t('server.settings.advanced_changes')}>
              <div className="surface-header border-b subtle-border px-3 py-2 font-semibold text-xs">
                {t('server.settings.advanced_change_summary', { count: advancedChanges.length })}
              </div>
              <ul className="divide-y subtle-border text-xs max-h-60 overflow-y-auto">
                {advancedChanges.map(change => (
                  <li key={change.field} className="grid gap-1 sm:grid-cols-[minmax(130px,1fr)_2fr_2fr] p-3 font-mono break-all">
                    <strong className="text-heading">{change.field}</strong>
                    <span className="text-status-danger">- {change.before === undefined ? '—' : JSON.stringify(change.before)}</span>
                    <span className="text-status-success">+ {change.after === undefined ? '—' : JSON.stringify(change.after)}</span>
                  </li>
                ))}
              </ul>
            </div>
          )}
          <div className="flex flex-wrap gap-2">
            <button type="button" onClick={() => { if (advanced.value) setAdvancedText(JSON.stringify(advanced.value, null, 2)); }}
              disabled={!advanced.value} className="border subtle-border rounded-lg px-3 py-2 text-xs disabled:opacity-50">{t('server.settings.format_json')}</button>
            <button type="button" onClick={() => {
              if (!savedAdvanced) return;
              const reset = () => { setAdvancedDirty(false); setAdvancedText(JSON.stringify(savedAdvanced, null, 2)); return Promise.resolve(); };
              if (advancedDirty) requestConfirmation({
                title: t('server.settings.reset_advanced_title'),
                description: t('server.settings.reset_advanced_description', { count: advancedChanges.length }),
                confirmLabel: t('server.settings.reset_to_saved'), tone: 'warning', onConfirm: reset,
              });
              else void reset();
            }} className="border subtle-border rounded-lg px-3 py-2 text-xs inline-flex gap-2 items-center">
              <RotateCcw className="h-3.5 w-3.5" />{t('server.settings.reset_to_saved')}
            </button>
            <p className="text-[11px] text-subtle self-center">{t('server.settings.header_save_hint')}</p>
          </div>
          <p className="text-xs text-dim">{t('server.settings.advanced_warning')}</p>
        </section>
      )}

      {tab === 'ca' && (
        <section className="surface-card border subtle-border rounded-xl p-5 space-y-4 max-w-3xl">
          <div>
            <h2 className="font-semibold text-sm">{t('server.settings.ca_title')}</h2>
            <p className="text-xs text-dim mt-1">{t('server.settings.ca_description')}</p>
          </div>
          {ca.isLoading && <p className="text-xs">{t('server.settings.ca_loading')}</p>}
          {ca.isError && <p role="alert" className="text-status-danger text-xs">{t('server.settings.ca_error')}</p>}
          {ca.data && (
            <>
              <h3 className="text-xs font-semibold text-heading">{t('server.settings.current_ca_identity')}</h3>
              <dl className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                {([
                  ['common_name', t('server.settings.ca_common_name')],
                  ['organization', t('server.settings.ca_organization')],
                  ['country', t('server.settings.ca_country')],
                  ['province', t('server.settings.ca_province')],
                  ['locality', t('server.settings.ca_locality')],
                ] as const).map(([field, title]) => (
                  <div key={field}>
                    <dt className={labelClass}>{title}</dt>
                    <dd className="flex items-center justify-between gap-2 rounded-lg surface-header border subtle-border px-3 py-2.5 min-w-0">
                      <span className="font-medium break-all">{ca.data.ca[field] || t('common.unavailable')}</span>
                      {ca.data.ca[field] && <CopyButton value={ca.data.ca[field]} label={title} />}
                    </dd>
                  </div>
                ))}
              </dl>
              <p className="text-[11px] text-dim break-all">
                {t('server.settings.certificate_serial')}: {ca.data.ca.serial} · {t('server.settings.expires')}: {new Date(ca.data.ca.not_after).toLocaleDateString()}
              </p>
              {ca.data.can_rotate ? (
                <details className="border border-amber-500/40 rounded-lg p-4 space-y-3">
                  <summary className="cursor-pointer font-semibold text-status-warning">{t('server.settings.prepare_new_ca')}</summary>
                  <p className="text-xs text-status-warning">
                    {t('server.settings.ca_warning')}
                  </p>
                  <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                    {([
                      ['common_name', t('server.settings.ca_common_name')],
                      ['organization', t('server.settings.ca_organization')],
                      ['country', t('server.settings.ca_country')],
                      ['province', t('server.settings.ca_province')],
                      ['locality', t('server.settings.ca_locality')],
                    ] as const).map(([field, title]) => (
                      <label key={field}>
                        <span className={labelClass}>{title}</span>
                        <input className={inputClass} value={caForm[field]}
                          onChange={e => setCAForm(s => ({ ...s, [field]: e.target.value }))} />
                      </label>
                    ))}
                  </div>
                  <label className="block">
                    <span className={labelClass}>{t('server.settings.ca_confirm_type')}</span>
                    <input className={inputClass} value={confirmation} onChange={e => setConfirmation(e.target.value)} />
                  </label>
                  <button type="button" disabled={confirmation !== 'ROTATE CA' || rotate.isPending}
                    onClick={() => requestConfirmation({
                      title: t('confirm.rotate_ca_title'),
                      description: t('confirm.rotate_ca_description', { name: caForm.common_name }),
                      confirmLabel: t('server.settings.issue_ca'),
                      onConfirm: () => rotate.mutateAsync(),
                    })} className="rounded-lg border border-red-500 text-status-danger px-4 py-2 text-xs font-medium disabled:opacity-50">
                    {rotate.isPending ? t('server.settings.rotating') : t('server.settings.issue_ca')}
                  </button>
                </details>
              ) : (
                <p className="text-xs text-dim border subtle-border p-3 rounded-lg">
                  {t('server.settings.ca_locked')}
                </p>
              )}
            </>
          )}
        </section>
      )}

      {tab === 'backup' && (
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-4 max-w-5xl">
          <section className="surface-card border subtle-border rounded-xl p-5 space-y-4">
            <div>
              <h2 className="text-sm font-semibold">{t('server.backup.export_title')}</h2>
              <p className="text-xs text-dim mt-1">{t('server.backup.export_description')}</p>
            </div>
            <p className="text-xs text-subtle border subtle-border rounded-lg p-3">
              {t('server.backup.export_includes_hint')}
            </p>
            <p className="text-[11px] text-dim">{t('server.backup.limits_hint')}</p>
            <form onSubmit={submitExportBackup} noValidate className="space-y-3">
              <div>
                <label htmlFor="backup-export-passphrase" className={labelClass}>
                  {t('server.backup.passphrase_label')}
                </label>
                <div className="relative">
                  <input
                    id="backup-export-passphrase"
                    name="export_passphrase"
                    ref={exportPassRef}
                    type={showExportPass ? 'text' : 'password'}
                    autoComplete="new-password"
                    className={inputClass + ' pr-9'}
                    value={exportPass}
                    maxLength={128}
                    aria-invalid={Boolean(exportPassError)}
                    aria-describedby={exportPassError ? 'backup-export-pass-err' : 'backup-export-pass-hint'}
                    onChange={e => {
                      setExportPass(e.target.value);
                      if (exportSubmitAttempted) setExportSubmitAttempted(false);
                      exportBackup.reset();
                    }}
                    onBlur={() => setExportTouched(s => ({ ...s, pass: true }))}
                  />
                  <button
                    type="button"
                    aria-pressed={showExportPass}
                    aria-label={t('server.backup.toggle_passphrase')}
                    onClick={() => setShowExportPass(v => !v)}
                    className="absolute inset-y-0 right-0 px-2.5 text-dim hover:text-current"
                  >
                    {showExportPass ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                  </button>
                </div>
                <p id="backup-export-pass-hint" className="text-[11px] text-dim mt-1">
                  {t('server.backup.passphrase_hint')}
                </p>
                {exportPassError && (
                  <p id="backup-export-pass-err" role="alert" className="text-xs text-status-danger mt-1">
                    {exportPassError}
                  </p>
                )}
              </div>

              <div>
                <label htmlFor="backup-export-confirm" className={labelClass}>
                  {t('server.backup.confirm_passphrase_label')}
                </label>
                <input
                  id="backup-export-confirm"
                  name="export_passphrase_confirm"
                  ref={exportConfirmRef}
                  type={showExportPass ? 'text' : 'password'}
                  autoComplete="new-password"
                  className={inputClass}
                  value={exportConfirmPass}
                  maxLength={128}
                  aria-invalid={Boolean(exportConfirmError)}
                  aria-describedby={exportConfirmError ? 'backup-export-confirm-err' : undefined}
                  onChange={e => {
                    setExportConfirmPass(e.target.value);
                    if (exportSubmitAttempted) setExportSubmitAttempted(false);
                    exportBackup.reset();
                  }}
                  onBlur={() => setExportTouched(s => ({ ...s, confirm: true }))}
                />
                {exportConfirmError && (
                  <p id="backup-export-confirm-err" role="alert" className="text-xs text-status-danger mt-1">
                    {exportConfirmError}
                  </p>
                )}
              </div>

              {exportBackup.isError && (
                <p role="alert" className="text-xs text-status-danger">{exportBackup.error.message}</p>
              )}

              <button
                type="submit"
                disabled={exportBackup.isPending}
                className="rounded-lg bg-blue-600 hover:bg-blue-500 text-white px-4 py-2 text-xs font-semibold disabled:opacity-50 inline-flex items-center gap-2"
              >
                <Download className="h-4 w-4" />
                {exportBackup.isPending ? t('server.backup.exporting') : t('server.backup.export_action')}
              </button>
            </form>
          </section>

          <section className="surface-card border border-amber-500/40 rounded-xl p-5 space-y-4">
            <div>
              <h2 className="text-sm font-semibold">{t('server.backup.restore_title')}</h2>
              <p className="text-xs text-dim mt-1">{t('server.backup.restore_description')}</p>
            </div>
            <p className="text-xs text-status-warning border border-amber-500/30 bg-amber-500/10 rounded-lg p-3">
              {t('server.backup.restore_warning')}
            </p>
            <form onSubmit={submitRestoreBackup} noValidate className="space-y-3">
              <div>
                <label htmlFor="backup-restore-file" className={labelClass}>
                  {t('server.backup.file_label')}
                </label>
                <input
                  id="backup-restore-file"
                  ref={restoreFileRef}
                  type="file"
                  accept=".tar.enc,.enc"
                  className={inputClass}
                  aria-invalid={Boolean(restoreFileError)}
                  aria-describedby={restoreFileError ? 'backup-restore-file-err' : 'backup-restore-file-hint'}
                  onChange={e => {
                    const selected = e.target.files?.[0] || null;
                    setRestoreFile(selected);
                    if (restoreSubmitAttempted) setRestoreSubmitAttempted(false);
                    restoreBackup.reset();
                  }}
                  onBlur={() => setRestoreTouched(s => ({ ...s, file: true }))}
                />
                <p id="backup-restore-file-hint" className="text-[11px] text-dim mt-1">
                  {restoreFile ? restoreFile.name : t('server.backup.file_hint')}
                </p>
                {restoreFileError && (
                  <p id="backup-restore-file-err" role="alert" className="text-xs text-status-danger mt-1">
                    {restoreFileError}
                  </p>
                )}
              </div>

              <div>
                <label htmlFor="backup-restore-passphrase" className={labelClass}>
                  {t('server.backup.restore_passphrase_label')}
                </label>
                <div className="relative">
                  <input
                    id="backup-restore-passphrase"
                    name="restore_passphrase"
                    ref={restorePassRef}
                    type={showRestorePass ? 'text' : 'password'}
                    autoComplete="off"
                    className={inputClass + ' pr-9'}
                    value={restorePass}
                    maxLength={128}
                    aria-invalid={Boolean(restorePassError)}
                    aria-describedby={restorePassError ? 'backup-restore-pass-err' : undefined}
                    onChange={e => {
                      setRestorePass(e.target.value);
                      if (restoreSubmitAttempted) setRestoreSubmitAttempted(false);
                      restoreBackup.reset();
                    }}
                    onBlur={() => setRestoreTouched(s => ({ ...s, pass: true }))}
                  />
                  <button
                    type="button"
                    aria-pressed={showRestorePass}
                    aria-label={t('server.backup.toggle_passphrase')}
                    onClick={() => setShowRestorePass(v => !v)}
                    className="absolute inset-y-0 right-0 px-2.5 text-dim hover:text-current"
                  >
                    {showRestorePass ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                  </button>
                </div>
                {restorePassError && (
                  <p id="backup-restore-pass-err" role="alert" className="text-xs text-status-danger mt-1">
                    {restorePassError}
                  </p>
                )}
              </div>

              <div>
                <label htmlFor="backup-restore-confirm" className={labelClass}>
                  {t('server.backup.restore_confirm_type')}
                </label>
                <input
                  id="backup-restore-confirm"
                  name="restore_confirm"
                  ref={restoreConfirmRef}
                  className={inputClass}
                  value={restoreConfirmText}
                  placeholder="RESTORE BACKUP"
                  aria-invalid={Boolean(restoreConfirmError)}
                  aria-describedby={restoreConfirmError ? 'backup-restore-confirm-err' : undefined}
                  onChange={e => {
                    setRestoreConfirmText(e.target.value);
                    if (restoreSubmitAttempted) setRestoreSubmitAttempted(false);
                    restoreBackup.reset();
                  }}
                  onBlur={() => setRestoreTouched(s => ({ ...s, confirm: true }))}
                />
                {restoreConfirmError && (
                  <p id="backup-restore-confirm-err" role="alert" className="text-xs text-status-danger mt-1">
                    {restoreConfirmError}
                  </p>
                )}
              </div>

              {restoreBackup.isError && (
                <p role="alert" className="text-xs text-status-danger">{restoreBackup.error.message}</p>
              )}

              <button
                type="submit"
                disabled={restoreBackup.isPending || restoreRestarting}
                className="rounded-lg border border-red-500 text-status-danger px-4 py-2 text-xs font-semibold disabled:opacity-50 inline-flex items-center gap-2"
              >
                <Upload className="h-4 w-4" />
                {restoreRestarting
                  ? t('server.backup.restore_restarting_btn')
                  : restoreBackup.isPending
                    ? t('server.backup.restoring')
                    : t('server.backup.restore_action')}
              </button>
            </form>
          </section>
        </div>
      )}
    </div>
  );
};
