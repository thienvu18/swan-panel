import React, { useEffect, useId, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  AlertTriangle,
  Bell,
  CheckCircle2,
  ChevronLeft,
  ChevronRight,
  Download,
  Eye,
  EyeOff,
  Pencil,
  Plus,
  Search,
  Send,
  ShieldAlert,
  ShieldCheck,
  Trash2,
} from 'lucide-react';
import { Modal } from '../components/Modal';
import { useConfirmAction } from '../components/ConfirmDialog';
import { EmptyState } from '../components/EmptyState';
import { TableQueryState } from '../components/TableQueryState';
import { TableActionLegend } from '../components/TableActionLegend';
import { QueryErrorBanner } from '../components/QueryErrorBanner';
import { useToast } from '../components/ToastProvider';
import { apiJSON } from '../lib/api';
import { downloadFilename } from '../lib/transportSecurity';

interface AuditLogEntry {
  id: string;
  sequence: number;
  created_at: string;
  actor_id: string;
  actor_email: string;
  actor_role: string;
  source_ip: string;
  action: string;
  severity: 'info' | 'warning' | 'critical';
  target_type: string;
  target_id: string;
  summary: string;
  details?: string;
  prev_hash: string;
  entry_hash: string;
}

interface ChainVerification {
  valid: boolean;
  total: number;
  checked_at: string;
  broken_sequence?: number;
  failure_reason?: string;
  latest_sequence?: number;
  latest_entry_hash?: string;
}

interface WebhookItem {
  id: string;
  name: string;
  provider_type: 'generic' | 'slack' | 'telegram' | 'teams';
  webhook_url_preview: string;
  telegram_chat_id?: string;
  has_signing_secret: boolean;
  events: string[];
  is_active: boolean;
  last_delivered_at?: string;
  last_status_code?: number;
  last_error?: string;
  created_at: string;
  updated_at: string;
}

interface AuditTrailPageProps {
  onRegisterHeaderAction?: (button: React.ReactNode) => void;
}

type AuditSection = 'logs' | 'webhooks';

const EVENT_PRESETS = [
  { id: '*', labelKey: 'audit.event_all' },
  { id: 'auth.*', labelKey: 'audit.event_auth' },
  { id: 'device.*', labelKey: 'audit.event_devices' },
  { id: 'gateway.*', labelKey: 'audit.event_gateway' },
  { id: 'access_rule.*', labelKey: 'audit.event_access' },
  { id: 'user.*', labelKey: 'audit.event_users' },
  { id: 'severity:critical', labelKey: 'audit.event_critical' },
] as const;

const PAGE_SIZE = 25;

function validateWebhookEndpoint(raw: string): boolean {
  try {
    const parsed = new URL(raw.trim());
    return parsed.protocol === 'https:' || (parsed.protocol === 'http:' && (parsed.hostname === 'localhost' || parsed.hostname === '127.0.0.1'));
  } catch {
    return false;
  }
}

export const AuditTrailPage: React.FC<AuditTrailPageProps> = ({ onRegisterHeaderAction }) => {
  const { t } = useTranslation();
  const toast = useToast();
  const queryClient = useQueryClient();
  const { requestConfirmation, confirmationDialog } = useConfirmAction();

  const auditTabId = useId();
  const webhooksTabId = useId();
  const auditCaptionId = useId();
  const webhooksCaptionId = useId();
  const webhookFormHeadingId = useId();

  const [section, setSection] = useState<AuditSection>('logs');
  const [severityFilter, setSeverityFilter] = useState<'all' | 'info' | 'warning' | 'critical'>('all');
  const [categoryFilter, setCategoryFilter] = useState('all');
  const [searchInput, setSearchInput] = useState('');
  const [deferredSearch, setDeferredSearch] = useState('');
  const [offset, setOffset] = useState(0);
  const [verification, setVerification] = useState<ChainVerification | null>(null);
  const [exportingFormat, setExportingFormat] = useState<'csv' | 'jsonl' | null>(null);

  // Webhook modal state
  const [isWebhookModalOpen, setIsWebhookModalOpen] = useState(false);
  const [editingWebhook, setEditingWebhook] = useState<WebhookItem | null>(null);
  const [whName, setWhName] = useState('');
  const [whProvider, setWhProvider] = useState<'generic' | 'slack' | 'telegram' | 'teams'>('slack');
  const [whURL, setWhURL] = useState('');
  const [whChatID, setWhChatID] = useState('');
  const [whSecret, setWhSecret] = useState('');
  const [showWhSecret, setShowWhSecret] = useState(false);
  const [clearWhSecret, setClearWhSecret] = useState(false);
  const [whEvents, setWhEvents] = useState<string[]>(['severity:critical', 'device.*', 'auth.bruteforce.suspected']);
  const [whActive, setWhActive] = useState(true);
  const [touchedFields, setTouchedFields] = useState<Record<string, boolean>>({});
  const [submitAttempted, setSubmitAttempted] = useState(false);

  useEffect(() => {
    const timer = window.setTimeout(() => {
      setDeferredSearch(searchInput.trim());
      setOffset(0);
    }, 250);
    return () => window.clearTimeout(timer);
  }, [searchInput]);

  const auditQuery = useQuery<{ audit_logs: AuditLogEntry[]; total: number }>({
    queryKey: ['audit_logs', severityFilter, categoryFilter, deferredSearch, offset],
    queryFn: () => {
      const params = new URLSearchParams({
        limit: String(PAGE_SIZE),
        offset: String(offset),
      });
      if (severityFilter !== 'all') params.set('severity', severityFilter);
      if (categoryFilter !== 'all') params.set('action', categoryFilter);
      if (deferredSearch) params.set('search', deferredSearch);
      return apiJSON(`/api/audit-logs?${params.toString()}`);
    },
  });

  const webhooksQuery = useQuery<{ webhooks: WebhookItem[] }>({
    queryKey: ['alert_webhooks'],
    queryFn: () => apiJSON('/api/webhooks'),
    enabled: section === 'webhooks',
  });

  const verifyChainMutation = useMutation({
    mutationFn: () => apiJSON<{ verification: ChainVerification }>('/api/audit-logs/verify'),
    onSuccess: (res) => {
      setVerification(res.verification);
      if (res.verification.valid) {
        toast.success(t('audit.chain_verified_toast', { count: res.verification.total }));
      } else {
        toast.error(t('audit.chain_tampered_toast'));
      }
    },
    onError: (err: Error) => toast.error(err.message),
  });

  const openNewWebhookModal = () => {
    setEditingWebhook(null);
    setWhName('');
    setWhProvider('slack');
    setWhURL('');
    setWhChatID('');
    setWhSecret('');
    setShowWhSecret(false);
    setClearWhSecret(false);
    setWhEvents(['severity:critical', 'device.*', 'auth.bruteforce.suspected']);
    setWhActive(true);
    setTouchedFields({});
    setSubmitAttempted(false);
    setIsWebhookModalOpen(true);
  };

  const openEditWebhookModal = (hook: WebhookItem) => {
    setEditingWebhook(hook);
    setWhName(hook.name);
    setWhProvider(hook.provider_type);
    setWhURL('');
    setWhChatID(hook.telegram_chat_id || '');
    setWhSecret('');
    setShowWhSecret(false);
    setClearWhSecret(false);
    setWhEvents(hook.events && hook.events.length > 0 ? hook.events : ['*']);
    setWhActive(hook.is_active);
    setTouchedFields({});
    setSubmitAttempted(false);
    setIsWebhookModalOpen(true);
  };

  useEffect(() => {
    if (!onRegisterHeaderAction) return;
    if (section === 'webhooks') {
      onRegisterHeaderAction(
        <button
          type="button"
          onClick={openNewWebhookModal}
          className="rounded-lg bg-blue-600 hover:bg-blue-500 text-white px-3 py-2 text-xs font-semibold inline-flex items-center gap-1.5 shadow-xs"
        >
          <Plus className="w-4 h-4" />
          {t('audit.add_webhook')}
        </button>,
      );
    } else {
      onRegisterHeaderAction(
        <button
          type="button"
          disabled={verifyChainMutation.isPending}
          onClick={() => verifyChainMutation.mutate()}
          className="rounded-lg bg-blue-600 hover:bg-blue-500 text-white px-3 py-2 text-xs font-semibold inline-flex items-center gap-1.5 shadow-xs disabled:opacity-50"
        >
          <ShieldCheck className="w-4 h-4" />
          {verifyChainMutation.isPending ? t('common.loading') : t('audit.verify_integrity')}
        </button>,
      );
    }
    return () => onRegisterHeaderAction(null);
  }, [onRegisterHeaderAction, section, verifyChainMutation.isPending, t]);

  const handleExport = async (format: 'csv' | 'jsonl') => {
    const token = localStorage.getItem('token');
    if (!token) {
      toast.error(t('common.error_occurred'));
      return;
    }
    setExportingFormat(format);
    try {
      const params = new URLSearchParams({ format });
      if (severityFilter !== 'all') params.set('severity', severityFilter);
      if (categoryFilter !== 'all') params.set('action', categoryFilter);
      if (deferredSearch) params.set('search', deferredSearch);

      const response = await fetch(`/api/audit-logs/export?${params.toString()}`, {
        headers: { Authorization: `Bearer ${token}` },
        cache: 'no-store',
      });
      if (!response.ok) {
        const body = await response.json().catch(() => ({}));
        throw new Error(body.error || t('common.error_occurred'));
      }
      const blob = await response.blob();
      const filename = downloadFilename(
        response.headers.get('Content-Disposition'),
        `swanpanel-audit.${format}`,
      );
      const url = URL.createObjectURL(blob);
      try {
        const a = document.createElement('a');
        a.href = url;
        a.download = filename;
        document.body.appendChild(a);
        a.click();
        a.remove();
      } finally {
        window.setTimeout(() => URL.revokeObjectURL(url), 1000);
      }
      toast.success(t('audit.export_ready'));
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t('common.error_occurred'));
    } finally {
      setExportingFormat(null);
    }
  };

  const saveWebhookMutation = useMutation({
    mutationFn: async () => {
      const payload: Record<string, unknown> = {
        name: whName.trim(),
        provider_type: whProvider,
        telegram_chat_id: whProvider === 'telegram' ? whChatID.trim() : '',
        events: whEvents,
        is_active: whActive,
      };
      if (whURL.trim() !== '') {
        payload.webhook_url = whURL.trim();
      }
      if (whProvider === 'generic') {
        if (clearWhSecret) {
          payload.clear_signing_secret = true;
        } else if (whSecret !== '') {
          payload.signing_secret = whSecret;
        }
      }
      if (editingWebhook) {
        return apiJSON<{ webhook: WebhookItem }>(`/api/webhooks/${encodeURIComponent(editingWebhook.id)}`, {
          method: 'PUT',
          body: JSON.stringify(payload),
        });
      }
      return apiJSON<{ webhook: WebhookItem }>('/api/webhooks', {
        method: 'POST',
        body: JSON.stringify(payload),
      });
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['alert_webhooks'] });
      void queryClient.invalidateQueries({ queryKey: ['audit_logs'] });
      toast.success(t('common.operation_saved'));
      setIsWebhookModalOpen(false);
    },
  });

  const deleteWebhookMutation = useMutation({
    mutationFn: (id: string) => apiJSON(`/api/webhooks/${encodeURIComponent(id)}`, { method: 'DELETE' }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['alert_webhooks'] });
      void queryClient.invalidateQueries({ queryKey: ['audit_logs'] });
      toast.success(t('common.operation_deleted'));
    },
  });

  const testWebhookMutation = useMutation({
    mutationFn: (id: string) =>
      apiJSON<{ success: boolean; status_code: number }>(`/api/webhooks/${encodeURIComponent(id)}/test`, {
        method: 'POST',
        body: JSON.stringify({}),
      }),
    onSuccess: (res) => {
      void queryClient.invalidateQueries({ queryKey: ['alert_webhooks'] });
      void queryClient.invalidateQueries({ queryKey: ['audit_logs'] });
      toast.success(t('audit.webhook_test_ok', { status: res.status_code }));
    },
    onError: (err: Error) => {
      void queryClient.invalidateQueries({ queryKey: ['alert_webhooks'] });
      toast.error(err.message);
    },
  });

  const nameError = (touchedFields.name || submitAttempted) && !whName.trim() ? t('audit.validation_name_required') : '';
  const urlRequired = !editingWebhook;
  const urlError =
    (touchedFields.url || submitAttempted) && ((urlRequired && !whURL.trim()) || (whURL.trim() !== '' && !validateWebhookEndpoint(whURL)))
      ? t('audit.validation_url_invalid')
      : '';
  const chatIDError =
    (touchedFields.chatID || submitAttempted) && whProvider === 'telegram' && !whChatID.trim()
      ? t('audit.validation_chat_id_required')
      : '';
  const eventsError = submitAttempted && whEvents.length === 0 ? t('audit.validation_events_required') : '';

  const handleWebhookSubmit = (event: React.FormEvent) => {
    event.preventDefault();
    setSubmitAttempted(true);
    if (!whName.trim()) {
      document.getElementById('webhook-name')?.focus();
      return;
    }
    if ((urlRequired && !whURL.trim()) || (whURL.trim() !== '' && !validateWebhookEndpoint(whURL))) {
      document.getElementById('webhook-url')?.focus();
      return;
    }
    if (whProvider === 'telegram' && !whChatID.trim()) {
      document.getElementById('webhook-chat-id')?.focus();
      return;
    }
    if (whEvents.length === 0) return;
    if (saveWebhookMutation.isPending) return;
    saveWebhookMutation.mutate();
  };

  const toggleEventPreset = (eventPattern: string) => {
    setWhEvents((prev) =>
      prev.includes(eventPattern) ? prev.filter((item) => item !== eventPattern) : [...prev, eventPattern],
    );
  };

  const entries = auditQuery.data?.audit_logs || [];
  const totalEntries = auditQuery.data?.total || 0;
  const webhooks = webhooksQuery.data?.webhooks || [];

  const severityBadgeClass = (severity: string) => {
    if (severity === 'critical') return 'bg-rose-500/10 text-status-danger border-rose-500/30';
    if (severity === 'warning') return 'bg-amber-500/10 text-status-warning border-amber-500/30';
    return 'bg-blue-500/10 text-accent border-blue-500/30';
  };

  return (
    <div className="space-y-4">
      {confirmationDialog}

      {/* Accessible Single-Row Tablist */}
      <div
        role="tablist"
        aria-label={t('audit.sections_label')}
        className="flex overflow-x-auto border-b subtle-border gap-1"
      >
        <button
          id={auditTabId}
          type="button"
          role="tab"
          aria-selected={section === 'logs'}
          onClick={() => setSection('logs')}
          className={`relative shrink-0 inline-flex items-center gap-2 px-4 py-2.5 text-xs font-semibold transition-colors ${
            section === 'logs'
              ? 'text-accent border-b-2 border-blue-500'
              : 'text-subtle hover:text-current border-b-2 border-transparent'
          }`}
        >
          <ShieldCheck className="w-4 h-4" />
          <span>{t('audit.tab_logs')}</span>
        </button>
        <button
          id={webhooksTabId}
          type="button"
          role="tab"
          aria-selected={section === 'webhooks'}
          onClick={() => setSection('webhooks')}
          className={`relative shrink-0 inline-flex items-center gap-2 px-4 py-2.5 text-xs font-semibold transition-colors ${
            section === 'webhooks'
              ? 'text-accent border-b-2 border-blue-500'
              : 'text-subtle hover:text-current border-b-2 border-transparent'
          }`}
        >
          <Bell className="w-4 h-4" />
          <span>{t('audit.tab_webhooks')}</span>
          {webhooks.length > 0 && (
            <span className="font-mono text-[10px] px-1.5 py-0.5 rounded bg-slate-500/10 text-subtle">
              {webhooks.length}
            </span>
          )}
        </button>
      </div>

      {/* PANEL 1: IMMUTABLE AUDIT TRAIL */}
      <div role="tabpanel" aria-labelledby={auditTabId} hidden={section !== 'logs'}>
        <div className="space-y-4">
          {auditQuery.isError && <QueryErrorBanner onRetry={() => { void auditQuery.refetch(); }} />}

          {/* Integrity Status Summary */}
          <section className="surface-card border subtle-border rounded-xl p-4 flex flex-wrap items-center justify-between gap-3 shadow-xs">
            <div className="flex items-start gap-3">
              <div
                className={`w-9 h-9 rounded-lg border flex items-center justify-center shrink-0 ${
                  verification === null
                    ? 'bg-blue-500/10 text-accent border-blue-500/20'
                    : verification.valid
                    ? 'bg-emerald-500/10 text-status-success border-emerald-500/20'
                    : 'bg-rose-500/10 text-status-danger border-rose-500/20'
                }`}
              >
                {verification && !verification.valid ? (
                  <AlertTriangle className="w-4 h-4" />
                ) : (
                  <CheckCircle2 className="w-4 h-4" />
                )}
              </div>
              <div className="space-y-0.5">
                <div className="flex flex-wrap items-center gap-2">
                  <h2 className="text-xs font-semibold text-heading">{t('audit.integrity_heading')}</h2>
                  {verification && (
                    <span
                      className={`px-2 py-0.5 rounded-md border text-[10px] font-semibold ${
                        verification.valid
                          ? 'bg-emerald-500/10 text-status-success border-emerald-500/30'
                          : 'bg-rose-500/10 text-status-danger border-rose-500/30'
                      }`}
                    >
                      {verification.valid ? t('audit.chain_valid') : t('audit.chain_broken')}
                    </span>
                  )}
                </div>
                <p className="text-xs text-subtle">
                  {verification
                    ? verification.valid
                      ? t('audit.integrity_verified_detail', {
                          count: verification.total,
                          seq: verification.latest_sequence ?? 0,
                        })
                      : t('audit.integrity_broken_detail', {
                          seq: verification.broken_sequence ?? 0,
                          reason: verification.failure_reason || '',
                        })
                    : t('audit.integrity_hint')}
                </p>
              </div>
            </div>
            {verification?.latest_entry_hash && (
              <div className="text-[11px] font-mono text-dim">
                {t('audit.latest_hash')}: {verification.latest_entry_hash.slice(0, 16)}…
              </div>
            )}
          </section>

          {/* Search & Filter Toolbar */}
          <search>
            <div className="surface-card border subtle-border rounded-xl p-3 flex flex-wrap items-center justify-between gap-3 shadow-xs text-xs">
              <div className="flex flex-wrap items-center gap-2">
                {/* Severity Chips */}
                <div className="flex items-center gap-1" role="group" aria-label={t('audit.filter_severity')}>
                  {(['all', 'info', 'warning', 'critical'] as const).map((lvl) => {
                    const active = severityFilter === lvl;
                    return (
                      <button
                        key={lvl}
                        type="button"
                        aria-pressed={active}
                        onClick={() => {
                          setSeverityFilter(lvl);
                          setOffset(0);
                        }}
                        className={`px-2.5 py-1 rounded-md border subtle-border text-[11px] font-medium transition-colors ${
                          active
                            ? lvl === 'critical'
                              ? 'bg-rose-500/15 border-rose-500/40 text-status-danger font-semibold'
                              : lvl === 'warning'
                              ? 'bg-amber-500/15 border-amber-500/40 text-status-warning font-semibold'
                              : 'bg-blue-600/15 border-blue-500/40 text-accent font-semibold'
                            : 'text-subtle hover:text-current hover:bg-slate-500/10'
                        }`}
                      >
                        {t(`audit.severity_${lvl}`)}
                      </button>
                    );
                  })}
                </div>

                {/* Category Filter */}
                <select
                  aria-label={t('audit.filter_category')}
                  value={categoryFilter}
                  onChange={(e) => {
                    setCategoryFilter(e.target.value);
                    setOffset(0);
                  }}
                  className="surface-input border subtle-border rounded-lg px-2.5 py-1 text-xs"
                >
                  <option value="all">{t('audit.category_all')}</option>
                  <option value="auth.*">{t('audit.category_auth')}</option>
                  <option value="device.*">{t('audit.category_devices')}</option>
                  <option value="access_rule.*">{t('audit.category_access')}</option>
                  <option value="user.*">{t('audit.category_users')}</option>
                  <option value="gateway.*">{t('audit.category_gateway')}</option>
                  <option value="dns.*">{t('audit.category_dns')}</option>
                  <option value="oidc.*">{t('audit.category_oidc')}</option>
                  <option value="session.*">{t('audit.category_sessions')}</option>
                  <option value="webhook.*">{t('audit.category_webhooks')}</option>
                </select>

                {/* Search Input */}
                <div className="relative">
                  <input
                    type="search"
                    aria-label={t('audit.search_placeholder')}
                    value={searchInput}
                    onChange={(e) => setSearchInput(e.target.value)}
                    placeholder={t('audit.search_placeholder')}
                    autoCorrect="off"
                    autoCapitalize="none"
                    spellCheck={false}
                    className="surface-input border subtle-border rounded-lg pl-7 pr-2.5 py-1 text-xs w-56 focus:outline-none focus:border-blue-500"
                  />
                  <Search className="w-3.5 h-3.5 text-dim absolute left-2 top-2.5" aria-hidden="true" />
                </div>
              </div>

              {/* Export Actions */}
              <div className="flex items-center gap-1.5">
                <button
                  type="button"
                  disabled={exportingFormat !== null}
                  onClick={() => void handleExport('csv')}
                  className="border subtle-border hover:bg-slate-500/10 px-2.5 py-1.5 rounded-lg text-subtle hover:text-current transition-colors inline-flex items-center gap-1.5 disabled:opacity-50"
                >
                  <Download className="w-3.5 h-3.5" />
                  {exportingFormat === 'csv' ? t('common.loading') : t('audit.export_csv')}
                </button>
                <button
                  type="button"
                  disabled={exportingFormat !== null}
                  onClick={() => void handleExport('jsonl')}
                  className="border subtle-border hover:bg-slate-500/10 px-2.5 py-1.5 rounded-lg text-subtle hover:text-current transition-colors inline-flex items-center gap-1.5 disabled:opacity-50"
                >
                  <Download className="w-3.5 h-3.5" />
                  {exportingFormat === 'jsonl' ? t('common.loading') : t('audit.export_json')}
                </button>
              </div>
            </div>
          </search>

          {/* Responsive Audit Table */}
          <div className="surface-card border subtle-border rounded-xl overflow-hidden shadow-xs">
            <div
              tabIndex={0}
              role="region"
              aria-labelledby={auditCaptionId}
              className="overflow-x-auto"
            >
              <table className="w-full text-left border-collapse">
                <caption id={auditCaptionId} className="sr-only">
                  {t('audit.table_caption')}
                </caption>
                <thead className="surface-header border-b subtle-border text-subtle font-medium">
                  <tr>
                    <th scope="col" className="px-4 py-3 whitespace-nowrap">{t('audit.col_sequence')}</th>
                    <th scope="col" className="px-4 py-3 whitespace-nowrap">{t('audit.col_severity')}</th>
                    <th scope="col" className="px-4 py-3 whitespace-nowrap">{t('audit.col_event')}</th>
                    <th scope="col" className="px-4 py-3 whitespace-nowrap">{t('audit.col_actor')}</th>
                    <th scope="col" className="px-4 py-3">{t('audit.col_summary')}</th>
                    <th scope="col" className="px-4 py-3 whitespace-nowrap">{t('audit.col_hash')}</th>
                  </tr>
                </thead>
                <tbody className="divide-y subtle-border">
                  {auditQuery.isPending || auditQuery.isError ? (
                    <TableQueryState
                      columns={6}
                      error={auditQuery.isError}
                      onRetry={() => { void auditQuery.refetch(); }}
                    />
                  ) : entries.length === 0 ? (
                    <tr>
                      <td colSpan={6} className="p-4">
                        <EmptyState
                          icon={ShieldAlert}
                          title={t('audit.empty_title')}
                          description={t('audit.empty_description')}
                        />
                      </td>
                    </tr>
                  ) : (
                    entries.map((entry) => (
                      <tr key={entry.id} className="hover-row transition-colors">
                        <td className="px-4 py-3 whitespace-nowrap font-mono text-[11px] text-subtle">
                          <div className="font-semibold text-current">#{entry.sequence}</div>
                          <div>{new Date(entry.created_at).toLocaleString()}</div>
                        </td>
                        <td className="px-4 py-3 whitespace-nowrap">
                          <span className={`inline-flex items-center px-2 py-0.5 rounded-md border text-[10px] font-semibold uppercase ${severityBadgeClass(entry.severity)}`}>
                            {t(`audit.severity_${entry.severity}`)}
                          </span>
                        </td>
                        <td className="px-4 py-3 whitespace-nowrap font-mono text-[11px] text-accent">
                          {entry.action}
                        </td>
                        <td className="px-4 py-3 whitespace-nowrap text-xs">
                          <div className="font-medium text-current">{entry.actor_email || entry.actor_role || t('common.unavailable')}</div>
                          <div className="font-mono text-[11px] text-dim">{entry.source_ip || '—'}</div>
                        </td>
                        <td className="px-4 py-3 text-xs text-current min-w-[220px]">
                          <div>{entry.summary}</div>
                          {entry.target_id && (
                            <div className="font-mono text-[11px] text-dim mt-0.5">
                              {entry.target_type}: {entry.target_id}
                            </div>
                          )}
                        </td>
                        <td className="px-4 py-3 whitespace-nowrap font-mono text-[11px] text-dim" title={entry.entry_hash}>
                          {entry.entry_hash.slice(0, 12)}…
                        </td>
                      </tr>
                    ))
                  )}
                </tbody>
              </table>
            </div>

            {/* Pagination Footer */}
            {totalEntries > 0 && (
              <div className="surface-header border-t subtle-border px-4 py-2.5 flex flex-wrap items-center justify-between gap-2 text-xs text-subtle">
                <span>
                  {t('audit.showing_range', {
                    start: offset + 1,
                    end: Math.min(offset + PAGE_SIZE, totalEntries),
                    total: totalEntries,
                  })}
                </span>
                <div className="flex items-center gap-1.5">
                  <button
                    type="button"
                    disabled={offset === 0}
                    onClick={() => setOffset((prev) => Math.max(0, prev - PAGE_SIZE))}
                    className="border subtle-border rounded-lg px-2.5 py-1 hover:bg-slate-500/10 disabled:opacity-50 inline-flex items-center gap-1"
                  >
                    <ChevronLeft className="w-3.5 h-3.5" />
                    {t('audit.prev_page')}
                  </button>
                  <button
                    type="button"
                    disabled={offset + PAGE_SIZE >= totalEntries}
                    onClick={() => setOffset((prev) => prev + PAGE_SIZE)}
                    className="border subtle-border rounded-lg px-2.5 py-1 hover:bg-slate-500/10 disabled:opacity-50 inline-flex items-center gap-1"
                  >
                    {t('audit.next_page')}
                    <ChevronRight className="w-3.5 h-3.5" />
                  </button>
                </div>
              </div>
            )}
          </div>
        </div>
      </div>

      {/* PANEL 2: ALERT WEBHOOKS */}
      <div role="tabpanel" aria-labelledby={webhooksTabId} hidden={section !== 'webhooks'}>
        <div className="space-y-4">
          {webhooksQuery.isError && <QueryErrorBanner onRetry={() => { void webhooksQuery.refetch(); }} />}

          <TableActionLegend
            actions={[
              { icon: Send, label: t('audit.action_test') },
              { icon: Pencil, label: t('common.edit') },
              { icon: Trash2, label: t('common.delete') },
            ]}
          />

          <div className="surface-card border subtle-border rounded-xl overflow-hidden shadow-xs">
            <div
              tabIndex={0}
              role="region"
              aria-labelledby={webhooksCaptionId}
              className="overflow-x-auto"
            >
              <table className="w-full text-left border-collapse">
                <caption id={webhooksCaptionId} className="sr-only">
                  {t('audit.webhooks_table_caption')}
                </caption>
                <thead className="surface-header border-b subtle-border text-subtle font-medium">
                  <tr>
                    <th scope="col" className="px-4 py-3">{t('audit.col_webhook_name')}</th>
                    <th scope="col" className="px-4 py-3 whitespace-nowrap">{t('audit.col_provider')}</th>
                    <th scope="col" className="px-4 py-3">{t('audit.col_destination')}</th>
                    <th scope="col" className="px-4 py-3">{t('audit.col_events')}</th>
                    <th scope="col" className="px-4 py-3 whitespace-nowrap">{t('audit.col_delivery_status')}</th>
                    <th scope="col" className="px-4 py-3 text-right whitespace-nowrap">{t('common.actions')}</th>
                  </tr>
                </thead>
                <tbody className="divide-y subtle-border">
                  {webhooksQuery.isPending || webhooksQuery.isError ? (
                    <TableQueryState
                      columns={6}
                      error={webhooksQuery.isError}
                      onRetry={() => { void webhooksQuery.refetch(); }}
                    />
                  ) : webhooks.length === 0 ? (
                    <tr>
                      <td colSpan={6} className="p-4">
                        <EmptyState
                          icon={Bell}
                          title={t('audit.webhooks_empty_title')}
                          description={t('audit.webhooks_empty_description')}
                          actionLabel={t('audit.add_webhook')}
                          actionIcon={Plus}
                          onAction={openNewWebhookModal}
                        />
                      </td>
                    </tr>
                  ) : (
                    webhooks.map((hook) => (
                      <tr key={hook.id} className="hover-row transition-colors">
                        <td className="px-4 py-3">
                          <div className="font-semibold text-current">{hook.name}</div>
                          <span
                            className={`inline-block mt-1 px-2 py-0.5 rounded text-[10px] font-medium border ${
                              hook.is_active
                                ? 'bg-emerald-500/10 text-status-success border-emerald-500/30'
                                : 'bg-slate-500/10 text-subtle subtle-border'
                            }`}
                          >
                            {hook.is_active ? t('audit.webhook_active') : t('audit.webhook_paused')}
                          </span>
                        </td>
                        <td className="px-4 py-3 whitespace-nowrap text-xs font-medium">
                          {t(`audit.provider_${hook.provider_type}`)}
                        </td>
                        <td className="px-4 py-3 font-mono text-[11px] text-subtle">
                          <div>{hook.webhook_url_preview}</div>
                          {hook.telegram_chat_id && (
                            <div className="text-dim mt-0.5">{t('audit.telegram_chat_label')}: {hook.telegram_chat_id}</div>
                          )}
                        </td>
                        <td className="px-4 py-3">
                          <div className="flex flex-wrap gap-1">
                            {(hook.events || []).map((ev) => (
                              <span
                                key={ev}
                                className="px-1.5 py-0.5 rounded bg-blue-500/10 text-accent border border-blue-500/20 font-mono text-[10px]"
                              >
                                {ev}
                              </span>
                            ))}
                          </div>
                        </td>
                        <td className="px-4 py-3 whitespace-nowrap text-xs">
                          {hook.last_delivered_at ? (
                            <div>
                              <div
                                className={
                                  hook.last_error
                                    ? 'text-status-danger font-medium'
                                    : 'text-status-success font-medium'
                                }
                              >
                                {hook.last_error
                                  ? hook.last_error
                                  : t('audit.delivery_ok', { code: hook.last_status_code || 200 })}
                              </div>
                              <div className="text-[11px] text-dim">
                                {new Date(hook.last_delivered_at).toLocaleString()}
                              </div>
                            </div>
                          ) : (
                            <span className="text-dim">{t('audit.delivery_never')}</span>
                          )}
                        </td>
                        <td className="px-4 py-3 text-right whitespace-nowrap">
                          <div className="inline-flex items-center gap-1">
                            <button
                              type="button"
                              disabled={testWebhookMutation.isPending}
                              onClick={() => testWebhookMutation.mutate(hook.id)}
                              data-tooltip={t('audit.action_test')}
                              aria-label={t('audit.action_test')}
                              className="p-1.5 rounded-lg border subtle-border hover:bg-blue-500/10 text-subtle hover:text-accent transition-colors disabled:opacity-50"
                            >
                              <Send className="w-3.5 h-3.5" />
                            </button>
                            <button
                              type="button"
                              onClick={() => openEditWebhookModal(hook)}
                              data-tooltip={t('common.edit')}
                              aria-label={t('common.edit')}
                              className="p-1.5 rounded-lg border subtle-border hover:bg-slate-500/10 text-subtle hover:text-current transition-colors"
                            >
                              <Pencil className="w-3.5 h-3.5" />
                            </button>
                            <button
                              type="button"
                              onClick={() =>
                                requestConfirmation({
                                  title: t('confirm.delete_webhook_title'),
                                  description: t('confirm.delete_webhook_description', { name: hook.name }),
                                  confirmLabel: t('common.delete'),
                                  tone: 'danger',
                                  onConfirm: () => deleteWebhookMutation.mutateAsync(hook.id),
                                })
                              }
                              data-tooltip={t('common.delete')}
                              aria-label={t('common.delete')}
                              className="p-1.5 rounded-lg border border-rose-500/20 bg-rose-500/5 hover:bg-rose-500/15 text-status-danger transition-colors"
                            >
                              <Trash2 className="w-3.5 h-3.5" />
                            </button>
                          </div>
                        </td>
                      </tr>
                    ))
                  )}
                </tbody>
              </table>
            </div>
          </div>
        </div>
      </div>

      {/* ADD / EDIT WEBHOOK MODAL */}
      <Modal
        isOpen={isWebhookModalOpen}
        onClose={() => {
          if (!saveWebhookMutation.isPending) {
            setIsWebhookModalOpen(false);
            saveWebhookMutation.reset();
          }
        }}
        title={editingWebhook ? t('audit.edit_webhook_title') : t('audit.add_webhook')}
        error={saveWebhookMutation.isError ? saveWebhookMutation.error.message : null}
        closeDisabled={saveWebhookMutation.isPending}
      >
        <form onSubmit={handleWebhookSubmit} aria-labelledby={webhookFormHeadingId} noValidate className="space-y-4">
          <span id={webhookFormHeadingId} className="sr-only">
            {editingWebhook ? t('audit.edit_webhook_title') : t('audit.add_webhook')}
          </span>

          <div>
            <label htmlFor="webhook-name" className="block mb-1.5 text-xs font-medium text-subtle">
              {t('audit.field_name')} <span aria-hidden="true">*</span>
            </label>
            <input
              id="webhook-name"
              name="name"
              type="text"
              required
              maxLength={128}
              value={whName}
              aria-invalid={nameError ? 'true' : undefined}
              aria-describedby={nameError ? 'webhook-name-error' : undefined}
              onChange={(e) => setWhName(e.target.value)}
              onBlur={() => setTouchedFields((prev) => ({ ...prev, name: true }))}
              placeholder={t('audit.field_name_placeholder')}
              className="w-full rounded-lg border subtle-border surface-input px-3 py-2 text-xs focus:outline-none focus:border-blue-500"
            />
            {nameError && (
              <p id="webhook-name-error" role="alert" className="text-xs text-status-danger mt-1">
                {nameError}
              </p>
            )}
          </div>

          <div>
            <label htmlFor="webhook-provider" className="block mb-1.5 text-xs font-medium text-subtle">
              {t('audit.field_provider')}
            </label>
            <select
              id="webhook-provider"
              name="provider_type"
              value={whProvider}
              onChange={(e) => setWhProvider(e.target.value as 'generic' | 'slack' | 'telegram' | 'teams')}
              className="w-full rounded-lg border subtle-border surface-input px-3 py-2 text-xs"
            >
              <option value="slack">{t('audit.provider_slack')}</option>
              <option value="telegram">{t('audit.provider_telegram')}</option>
              <option value="teams">{t('audit.provider_teams')}</option>
              <option value="generic">{t('audit.provider_generic')}</option>
            </select>
          </div>

          <div>
            <label htmlFor="webhook-url" className="block mb-1.5 text-xs font-medium text-subtle">
              {t('audit.field_url')} {!editingWebhook && <span aria-hidden="true">*</span>}
            </label>
            <input
              id="webhook-url"
              name="webhook_url"
              type="url"
              required={!editingWebhook}
              autoComplete="off"
              autoCorrect="off"
              autoCapitalize="none"
              spellCheck={false}
              value={whURL}
              aria-invalid={urlError ? 'true' : undefined}
              aria-describedby={urlError ? 'webhook-url-error webhook-url-help' : 'webhook-url-help'}
              onChange={(e) => setWhURL(e.target.value)}
              onBlur={() => setTouchedFields((prev) => ({ ...prev, url: true }))}
              placeholder={
                editingWebhook
                  ? t('audit.field_url_keep_placeholder')
                  : whProvider === 'telegram'
                  ? 'https://api.telegram.org/bot<token>/sendMessage'
                  : 'https://hooks.slack.com/services/...'
              }
              className="w-full rounded-lg border subtle-border surface-input px-3 py-2 text-xs font-mono focus:outline-none focus:border-blue-500"
            />
            <p id="webhook-url-help" className="text-[11px] text-dim mt-1">
              {editingWebhook ? t('audit.field_url_edit_hint') : t('audit.field_url_hint')}
            </p>
            {urlError && (
              <p id="webhook-url-error" role="alert" className="text-xs text-status-danger mt-1">
                {urlError}
              </p>
            )}
          </div>

          {whProvider === 'telegram' && (
            <div>
              <label htmlFor="webhook-chat-id" className="block mb-1.5 text-xs font-medium text-subtle">
                {t('audit.field_telegram_chat_id')} <span aria-hidden="true">*</span>
              </label>
              <input
                id="webhook-chat-id"
                name="telegram_chat_id"
                type="text"
                required
                autoComplete="off"
                autoCorrect="off"
                autoCapitalize="none"
                spellCheck={false}
                value={whChatID}
                aria-invalid={chatIDError ? 'true' : undefined}
                aria-describedby={chatIDError ? 'webhook-chat-error' : undefined}
                onChange={(e) => setWhChatID(e.target.value)}
                onBlur={() => setTouchedFields((prev) => ({ ...prev, chatID: true }))}
                placeholder="-1001234567890"
                className="w-full rounded-lg border subtle-border surface-input px-3 py-2 text-xs font-mono focus:outline-none focus:border-blue-500"
              />
              {chatIDError && (
                <p id="webhook-chat-error" role="alert" className="text-xs text-status-danger mt-1">
                  {chatIDError}
                </p>
              )}
            </div>
          )}

          {whProvider === 'generic' && (
            <div className="space-y-2">
              <label htmlFor="webhook-secret" className="block text-xs font-medium text-subtle">
                {t('audit.field_signing_secret')}
              </label>
              <div className="relative">
                <input
                  id="webhook-secret"
                  name="signing_secret"
                  type={showWhSecret ? 'text' : 'password'}
                  autoComplete="new-password"
                  value={whSecret}
                  disabled={clearWhSecret}
                  onChange={(e) => setWhSecret(e.target.value)}
                  placeholder={editingWebhook?.has_signing_secret ? t('audit.field_secret_keep_placeholder') : t('audit.field_secret_placeholder')}
                  className="w-full rounded-lg border subtle-border surface-input pl-3 pr-9 py-2 text-xs font-mono focus:outline-none focus:border-blue-500 disabled:opacity-50"
                />
                <button
                  type="button"
                  aria-pressed={showWhSecret}
                  aria-label={showWhSecret ? t('auth.hide_password') : t('auth.show_password')}
                  onClick={() => setShowWhSecret((s) => !s)}
                  className="absolute right-2.5 top-2 text-subtle hover:text-current"
                >
                  {showWhSecret ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
                </button>
              </div>
              {editingWebhook?.has_signing_secret && (
                <label className="inline-flex items-center gap-2 text-xs text-subtle">
                  <input
                    type="checkbox"
                    checked={clearWhSecret}
                    onChange={(e) => {
                      setClearWhSecret(e.target.checked);
                      if (e.target.checked) setWhSecret('');
                    }}
                  />
                  <span>{t('audit.clear_signing_secret')}</span>
                </label>
              )}
            </div>
          )}

          <fieldset className="border subtle-border rounded-lg p-3 space-y-2">
            <legend className="px-1 text-xs font-medium text-subtle">
              {t('audit.field_events')} <span aria-hidden="true">*</span>
            </legend>
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
              {EVENT_PRESETS.map((preset) => {
                const inputId = `wh-ev-${preset.id}`;
                return (
                  <div key={preset.id} className="flex items-center gap-2">
                    <input
                      id={inputId}
                      type="checkbox"
                      checked={whEvents.includes(preset.id)}
                      onChange={() => toggleEventPreset(preset.id)}
                    />
                    <label htmlFor={inputId} className="text-xs text-current cursor-pointer">
                      {t(preset.labelKey)}
                    </label>
                  </div>
                );
              })}
            </div>
            {eventsError && (
              <p role="alert" className="text-xs text-status-danger">
                {eventsError}
              </p>
            )}
          </fieldset>

          <div className="flex items-center gap-2">
            <input
              id="webhook-active"
              name="is_active"
              type="checkbox"
              checked={whActive}
              onChange={(e) => setWhActive(e.target.checked)}
            />
            <label htmlFor="webhook-active" className="text-xs text-current cursor-pointer">
              {t('audit.field_active')}
            </label>
          </div>

          <div className="flex justify-end gap-2 border-t subtle-border pt-3">
            <button
              type="button"
              disabled={saveWebhookMutation.isPending}
              onClick={() => setIsWebhookModalOpen(false)}
              className="rounded-lg border subtle-border px-3 py-2 text-xs font-medium hover:bg-slate-500/10 disabled:opacity-50"
            >
              {t('common.cancel')}
            </button>
            <button
              type="submit"
              disabled={saveWebhookMutation.isPending}
              className="rounded-lg bg-blue-600 hover:bg-blue-500 text-white px-4 py-2 text-xs font-semibold disabled:opacity-50"
            >
              {saveWebhookMutation.isPending ? t('common.loading') : t('common.save')}
            </button>
          </div>
        </form>
      </Modal>
    </div>
  );
};
