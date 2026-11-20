import { useToast } from '../components/ToastProvider';
import React, { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { ChevronDown, Globe, Plus, Trash2, Search, Filter, X } from 'lucide-react';
import { Modal } from '../components/Modal';
import { useConfirmAction } from '../components/ConfirmDialog';
import { CopyButton } from '../components/CopyButton';
import { EmptyState } from '../components/EmptyState';
import { TableQueryState } from '../components/TableQueryState';
import { TableActionLegend } from '../components/TableActionLegend';
import { QueryErrorBanner } from '../components/QueryErrorBanner';
import { isValidDNSOverrideTarget, type DNSOverrideRecordType } from '../lib/dnsRecordTargets';

interface DNSSettingsDTO {
  id: number;
  magic_dns_suffix: string;
  upstream_resolvers: string;
  is_magic_dns_enabled: boolean;
}

interface DNSOverrideItem {
  id: string;
  hostname: string;
  record_type: string;
  target_address: string;
}

export const DNSEnginePage: React.FC = () => {
  const { t } = useTranslation();
  const toast = useToast();
  const queryClient = useQueryClient();
  const [isAddOverrideOpen, setIsAddOverrideOpen] = useState(false);
  const { requestConfirmation, confirmationDialog } = useConfirmAction();
  const [searchQuery, setSearchQuery] = useState('');
  const [typeFilter, setTypeFilter] = useState<string>('all');

  // Form states for settings
  const [magicSuffix, setMagicSuffix] = useState('.strongnet');
  const [upstreams, setUpstreams] = useState('1.1.1.1, 8.8.8.8');
  const [settingsDirty, setSettingsDirty] = useState({ suffix: false, upstreams: false });

  // Form states for Add Override
  const [overrideHost, setOverrideHost] = useState('');
  const [overrideType, setOverrideType] = useState<DNSOverrideRecordType>('A');
  const [overrideTarget, setOverrideTarget] = useState('');
  const isTargetValid = isValidDNSOverrideTarget(overrideType, overrideTarget);
  const showTargetError = overrideTarget.trim().length > 0 && !isTargetValid;

  // Query DNS Settings
  const { data: dnsSettingsData, isError: dnsSettingsError, refetch: refetchDNSSettings } = useQuery<{ settings?: DNSSettingsDTO; dns_settings?: DNSSettingsDTO }>({
    queryKey: ['dns_settings'],
    queryFn: async () => {
      const res = await fetch('/api/settings/dns', {
        headers: { Authorization: `Bearer ${localStorage.getItem('token') || ''}` },
      });
      if (!res.ok) throw new Error('Failed to fetch DNS settings');
      return res.json();
    },
  });

  useEffect(() => {
    const current = dnsSettingsData?.settings || dnsSettingsData?.dns_settings;
    if (!current) return;
    if (!settingsDirty.suffix) setMagicSuffix(current.magic_dns_suffix || '.internal');
    if (!settingsDirty.upstreams) setUpstreams(current.upstream_resolvers || '1.1.1.1, 8.8.8.8');
  }, [dnsSettingsData, settingsDirty.suffix, settingsDirty.upstreams]);

  // Query DNS Overrides
  const { data: overridesData, isPending: overridesPending, isError: dnsOverridesError, refetch: refetchOverrides } = useQuery<{ dns_overrides: DNSOverrideItem[] }>({
    queryKey: ['dns_overrides'],
    queryFn: async () => {
      const res = await fetch('/api/dns/overrides', {
        headers: { Authorization: `Bearer ${localStorage.getItem('token') || ''}` },
      });
      if (!res.ok) throw new Error('Failed to fetch DNS overrides');
      return res.json();
    },
  });

  const overrides = overridesData?.dns_overrides || [];

  const filteredOverrides = overrides.filter((o) => {
    if (typeFilter !== 'all' && o.record_type?.toUpperCase() !== typeFilter) return false;
    if (!searchQuery.trim()) return true;
    const q = searchQuery.toLowerCase();
    return o.hostname.toLowerCase().includes(q) || o.target_address.toLowerCase().includes(q);
  });

  // Update Settings Mutation
  const updateSettingsMutation = useMutation({
    mutationFn: async (payload: { magic_dns_suffix?: string; upstream_resolvers?: string }) => {
      const res = await fetch('/api/settings/dns', {
        method: 'PUT',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${localStorage.getItem('token') || ''}`,
        },
        body: JSON.stringify(payload),
      });
      if (!res.ok) throw new Error('Failed to update DNS settings');
      return res.json();
    },
    onSuccess: (_, variables) => {
      setSettingsDirty(previous => ({
        suffix: variables.magic_dns_suffix === undefined ? previous.suffix : false,
        upstreams: variables.upstream_resolvers === undefined ? previous.upstreams : false,
      }));
      queryClient.invalidateQueries({ queryKey: ['dns_settings'] });
      toast.success(t('common.operation_saved'));
    },
  });

  // Create Override Mutation
  const createOverrideMutation = useMutation({
    mutationFn: async () => {
      const res = await fetch('/api/dns/overrides', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${localStorage.getItem('token') || ''}`,
        },
        body: JSON.stringify({
          hostname: overrideHost,
          record_type: overrideType,
          target_address: overrideTarget.trim(),
        }),
      });
      if (!res.ok) {
        const errData = await res.json().catch(() => ({}));
        throw new Error(errData.error || t('common.error_occurred') || 'Failed to create DNS override');
      }
      return res.json();
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['dns_overrides'] });
      toast.success(t('common.operation_created'));
      setIsAddOverrideOpen(false);
      createOverrideMutation.reset();
      setOverrideHost('');
      setOverrideTarget('');
    },
  });

  // Delete Override Mutation
  const deleteOverrideMutation = useMutation({
    mutationFn: async (id: string) => {
      const res = await fetch(`/api/dns/overrides/${id}`, {
        method: 'DELETE',
        headers: { Authorization: `Bearer ${localStorage.getItem('token') || ''}` },
      });
      if (!res.ok) throw new Error('Failed to delete override');
      return res.json();
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['dns_overrides'] });
      toast.success(t('common.operation_deleted'));
    },
  });

  return (
    <div className="space-y-5">
      {(dnsSettingsError || dnsOverridesError) && (
        <QueryErrorBanner onRetry={() => { void refetchDNSSettings(); void refetchOverrides(); }} />
      )}
      {/* Cards Grid */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        {/* MagicDNS Base Domain */}
        <div className="border subtle-border rounded-xl p-4 surface-card space-y-3 shadow-xs">
          <div className="border-b subtle-border pb-2">
            <h2 className="font-bold text-xs text-slate-900 dark:text-white">{t('dns.magic_dns_title')}</h2>
            <div className="text-[11px] text-dim">{t('dns.magic_dns_desc')}</div>
          </div>

          <div className="space-y-2 text-xs">
            <div>
              <label className="text-subtle text-[11px] block mb-1">{t('dns.base_domain_suffix')}</label>
              <div className="flex gap-2">
                <input
                  type="text"
                  value={magicSuffix}
                  onChange={(e) => { setMagicSuffix(e.target.value); setSettingsDirty(previous => ({ ...previous, suffix: true })); }}
                  placeholder="e.g. .strongnet or .corp.internal"
                  className="flex-1 min-w-0 surface-input border subtle-border rounded-lg px-2.5 py-1.5 font-mono text-xs focus:outline-none focus:border-blue-500"
                />
                <button
                  onClick={() => updateSettingsMutation.mutate({ magic_dns_suffix: magicSuffix })}
                  disabled={updateSettingsMutation.isPending}
                  className="bg-blue-600 hover:bg-blue-500 text-white font-medium px-3 py-1.5 rounded-lg transition-colors text-xs"
                >
                  {t('common.save')}
                </button>
              </div>
            </div>
            <div className="text-[11px] text-subtle">
              {t('dns.reachable_at')}:{' '}
              <code className="font-mono text-accent">[device-name]{magicSuffix}</code>
            </div>
          </div>
        </div>

        {/* Upstream resolvers are advanced settings; keep the common DNS setup scannable. */}
        <details className="group border subtle-border rounded-xl p-4 surface-card shadow-xs">
          <summary className="list-none cursor-pointer flex items-start justify-between gap-3 rounded-md focus-visible:outline-2 focus-visible:outline-blue-500">
            <span>
              <span className="block font-bold text-xs text-slate-900 dark:text-white">{t('dns.upstream_title')}</span>
              <span className="block text-[11px] text-dim mt-1">{t('dns.upstream_desc')}</span>
            </span>
            <ChevronDown aria-hidden="true" className="w-4 h-4 mt-0.5 shrink-0 text-subtle transition-transform group-open:rotate-180" />
          </summary>

          <div className="space-y-2 text-xs border-t subtle-border pt-3 mt-3">
            <div>
              <label className="text-subtle text-[11px] block mb-1">{t('dns.ip_resolvers')}</label>
              <div className="flex gap-2">
                <input
                  type="text"
                  value={upstreams}
                  onChange={(e) => { setUpstreams(e.target.value); setSettingsDirty(previous => ({ ...previous, upstreams: true })); }}
                  placeholder="e.g. 1.1.1.1, 8.8.8.8, 9.9.9.9"
                  className="flex-1 min-w-0 surface-input border subtle-border rounded-lg px-2.5 py-1.5 font-mono text-xs focus:outline-none focus:border-blue-500"
                />
                <button
                  onClick={() => updateSettingsMutation.mutate({ upstream_resolvers: upstreams })}
                  disabled={updateSettingsMutation.isPending}
                  className="bg-blue-600 hover:bg-blue-500 text-white font-medium px-3 py-1.5 rounded-lg transition-colors text-xs"
                >
                  {t('common.save')}
                </button>
              </div>
            </div>
            <div className="flex flex-wrap items-center gap-2 text-[10px] text-subtle">
              <span>{t('dns.suggestions')}:</span>
              <span className="bg-slate-500/10 border subtle-border px-1.5 py-0.5 rounded font-mono">1.1.1.1 (Cloudflare)</span>
              <span className="bg-slate-500/10 border subtle-border px-1.5 py-0.5 rounded font-mono">8.8.8.8 (Google)</span>
            </div>
          </div>
        </details>
      </div>

      {/* Custom DNS Overrides */}
      <div className="space-y-2">
        <div className="flex flex-col lg:flex-row lg:items-center justify-between gap-3">
          <div>
            <span className="font-semibold text-xs text-slate-900 dark:text-white tracking-tight">
              {t('dns.custom_overrides')} ({overrides.length})
            </span>
            <div className="text-[11px] text-dim">{t('dns.custom_overrides_desc')}</div>
          </div>
          <div className="flex flex-col sm:flex-row items-stretch sm:items-center gap-2 w-full lg:w-auto lg:ml-auto">
            <div className="relative flex-1 min-w-0 sm:w-48 md:w-56">
              <Search className="w-3.5 h-3.5 text-subtle absolute left-2.5 top-1/2 -translate-y-1/2" />
              <input
                type="text"
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                placeholder={t('dns.search_placeholder')}
                className="w-full surface-input border subtle-border rounded-lg pl-8 pr-2.5 py-1.5 text-xs focus:outline-none focus:border-blue-500 font-sans"
              />
            </div>
            <button
              type="button"
              onClick={() => setIsAddOverrideOpen(true)}
              className="bg-blue-600 hover:bg-blue-500 text-white font-medium px-3 py-1.5 rounded-lg transition-colors flex items-center justify-center gap-1.5 shadow-xs text-xs shrink-0 whitespace-nowrap"
            >
              <Plus className="w-3.5 h-3.5" />
              {t('dns.add_override')}
            </button>
          </div>
        </div>

        {/* Record Type Filter Chips */}
        <div className="flex flex-wrap items-center gap-1.5 text-[11px] px-1">
          <span className="text-subtle font-medium flex items-center gap-1 mr-1">
            <Filter className="w-3 h-3" />
            {t('dns.record_type_filter')}:
          </span>
          {(['all', 'A', 'CNAME'] as const).map((rType) => {
            const count = rType === 'all' ? overrides.length : overrides.filter((o) => o.record_type?.toUpperCase() === rType).length;
            const isSelected = typeFilter === rType;
            return (
              <button
                key={rType}
                type="button"
                onClick={() => setTypeFilter(rType)}
                className={`px-2.5 py-1 rounded-md border subtle-border transition-colors uppercase font-mono text-[11px] ${
                  isSelected
                    ? 'bg-blue-600/15 border-blue-500/40 text-accent font-semibold'
                    : 'text-subtle hover:text-current hover:bg-slate-500/10'
                }`}
              >
                {rType} {count > 0 && <span className="opacity-75">({count})</span>}
              </button>
            );
          })}

          {(typeFilter !== 'all' || searchQuery) && (
            <button
              type="button"
              onClick={() => {
                setTypeFilter('all');
                setSearchQuery('');
              }}
              className="ml-auto text-status-danger hover:text-rose-700 dark:hover:text-rose-400 flex items-center gap-1 text-[11px] font-medium"
            >
              <X className="w-3 h-3" /> {t('common.reset_filters')}
            </button>
          )}
        </div>

        <TableActionLegend actions={[{ icon: Trash2, label: t('common.delete') }]} />
        <div className="border subtle-border rounded-xl overflow-hidden surface-card shadow-xs">
          <div className="overflow-x-auto">
            <table className="w-full text-left font-mono text-xs min-w-[500px]">
              <thead className="border-b subtle-border surface-header text-subtle font-sans font-semibold text-[11px]">
                <tr>
                  <th className="py-2.5 px-3.5">{t('dns.hostname')}</th>
                  <th className="py-2.5 px-3.5">{t('dns.record_type')}</th>
                  <th className="py-2.5 px-3.5">{t('dns.points_to')}</th>
                  <th className="py-2.5 px-3.5 text-right font-sans">{t('common.actions')}</th>
                </tr>
              </thead>
              <tbody className="divide-y subtle-border">
                {overridesPending || (dnsOverridesError && !overridesData) ? (
                  <TableQueryState columns={4} error={dnsOverridesError} onRetry={() => void refetchOverrides()} />
                ) : filteredOverrides.length === 0 ? (
                  <tr>
                    <td colSpan={4} className="py-8">
                      <EmptyState
                        icon={Globe}
                        title={searchQuery || typeFilter !== 'all' ? t('dns.no_matching_records') : t('dns.no_custom_records')}
                        description={searchQuery || typeFilter !== 'all' ? t('dns.no_matching_records_description') : t('dns.no_custom_records_description')}
                        actionLabel={searchQuery || typeFilter !== 'all' ? t('common.clear_filters') : t('dns.add_override')}
                        actionIcon={searchQuery || typeFilter !== 'all' ? X : Plus}
                        onAction={searchQuery || typeFilter !== 'all' ? () => { setSearchQuery(''); setTypeFilter('all'); } : () => setIsAddOverrideOpen(true)}
                        className="border-none shadow-none bg-transparent"
                      />
                    </td>
                  </tr>
                ) : (
                  filteredOverrides.map((rec) => (
                    <tr key={rec.id} className="hover-row transition-colors">
                      <td className="py-2.5 px-3.5 font-sans font-medium text-accent">
                        <div className="flex items-center gap-2">
                          <Globe className="w-3.5 h-3.5 text-subtle shrink-0" />
                          <span>{rec.hostname}</span>
                          <CopyButton value={rec.hostname} label={`hostname ${rec.hostname}`} />
                        </div>
                      </td>
                      <td className="py-2.5 px-3.5 text-subtle">
                        <span className="border border-slate-500/20 bg-slate-500/10 px-1.5 py-0.5 rounded text-[10px] font-mono">
                          {rec.record_type}
                        </span>
                      </td>
                      <td className="py-2.5 px-3.5 text-slate-900 dark:text-white font-mono">
                        <div className="flex items-center gap-1.5">
                          <span>{rec.target_address}</span>
                          <CopyButton value={rec.target_address} label={`target ${rec.target_address}`} />
                        </div>
                      </td>
                      <td className="py-2.5 px-3.5 text-right font-sans">
                        <button
                          type="button"
                          onClick={() => requestConfirmation({
                            title: t('confirm.delete_dns_title'),
                            description: t('confirm.delete_dns_description', { name: rec.hostname, target: rec.target_address }),
                            confirmLabel: t('common.delete'),
                            onConfirm: () => deleteOverrideMutation.mutateAsync(rec.id),
                          })}
                          className="p-1.5 hover:bg-rose-500/10 rounded-md text-status-danger transition-colors"
                          title={t('common.delete')} data-tooltip={t('common.delete')}
                          aria-label={`${t('common.delete')} ${rec.hostname}`}
                        >
                          <Trash2 className="w-3.5 h-3.5" />
                        </button>
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        </div>
      </div>

      {/* MODAL: ADD DNS OVERRIDE */}
      <Modal
        isOpen={isAddOverrideOpen}
        onClose={() => {
          setIsAddOverrideOpen(false);
          createOverrideMutation.reset();
        }}
        title={t('dns.add_override')}
        error={(createOverrideMutation.error as Error)?.message}
      >
        <div className="space-y-3 text-xs">
          <div>
            <label htmlFor="dns-override-hostname" className="text-subtle block mb-1">{t('dns.hostname')}</label>
            <input
              id="dns-override-hostname"
              type="text"
              value={overrideHost}
              onChange={(e) => setOverrideHost(e.target.value)}
              placeholder="e.g. gitlab.internal or *.corp.internal"
              className="w-full surface-input border subtle-border rounded-lg px-2.5 py-1.5 font-mono text-xs focus:outline-none focus:border-blue-500"
            />
            <p className="mt-1 text-[11px] text-dim">
              {t('dns.override_exact_note')}
            </p>
          </div>

          <div>
            <label htmlFor="dns-override-type" className="text-subtle block mb-1">{t('dns.record_type')}</label>
            <select
              id="dns-override-type"
              value={overrideType}
              onChange={(e) => { setOverrideType(e.target.value as DNSOverrideRecordType); setOverrideTarget(''); }}
              className="w-full surface-input border subtle-border rounded-lg px-2.5 py-1.5 text-xs focus:outline-none"
            >
              <option value="A">{t('dns.a_record_option')}</option>
              <option value="CNAME">{t('dns.cname_record_option')}</option>
            </select>
          </div>

          <div>
            <label htmlFor="dns-override-target" className="text-subtle block mb-1">{t(overrideType === 'A' ? 'dns.a_target_label' : 'dns.cname_target_label')}</label>
            <input
              type="text"
              id="dns-override-target"
              value={overrideTarget}
              onChange={(e) => setOverrideTarget(e.target.value)}
              placeholder={t(overrideType === 'A' ? 'dns.a_target_example' : 'dns.cname_target_example')}
              aria-invalid={showTargetError}
              aria-describedby={showTargetError ? 'dns-override-target-error' : undefined}
              className="w-full surface-input border subtle-border rounded-lg px-2.5 py-1.5 font-mono text-xs focus:outline-none focus:border-blue-500"
            />
            {showTargetError && <p id="dns-override-target-error" role="alert" className="mt-1 text-[11px] text-status-danger">{t(overrideType === 'A' ? 'dns.invalid_target_a' : 'dns.invalid_target_cname')}</p>}
          </div>
        </div>

        <div className="flex gap-2 pt-2 border-t subtle-border">
          <button
            onClick={() => createOverrideMutation.mutate()}
            disabled={!overrideHost.trim() || !isTargetValid || createOverrideMutation.isPending}
            className="flex-1 bg-blue-600 enabled:hover:bg-blue-500 disabled:bg-slate-200 disabled:text-slate-600 dark:disabled:bg-slate-800 dark:disabled:text-slate-400 disabled:shadow-none text-white font-medium py-1.5 rounded-lg shadow-xs transition-colors text-xs"
          >
            {createOverrideMutation.isPending ? t('common.saving') : t('common.save')}
          </button>
          <button
            onClick={() => {
              setIsAddOverrideOpen(false);
              createOverrideMutation.reset();
            }}
            className="px-3 py-1.5 border subtle-border rounded-lg hover:bg-slate-500/10 text-xs"
          >
            {t('common.cancel')}
          </button>
        </div>
      </Modal>
      {confirmationDialog}
    </div>
  );
};
