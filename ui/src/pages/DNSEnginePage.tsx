import React, { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { Globe, Plus, Trash2, CheckCircle, Search, Filter, X } from 'lucide-react';
import { Modal } from '../components/Modal';
import { CopyButton } from '../components/CopyButton';
import { EmptyState } from '../components/EmptyState';

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
  const queryClient = useQueryClient();
  const [isAddOverrideOpen, setIsAddOverrideOpen] = useState(false);
  const [toastMessage, setToastMessage] = useState('');
  const [searchQuery, setSearchQuery] = useState('');
  const [typeFilter, setTypeFilter] = useState<string>('all');

  // Form states for settings
  const [magicSuffix, setMagicSuffix] = useState('.strongnet');
  const [upstreams, setUpstreams] = useState('1.1.1.1, 8.8.8.8');

  // Form states for Add Override
  const [overrideHost, setOverrideHost] = useState('');
  const [overrideType, setOverrideType] = useState('A');
  const [overrideTarget, setOverrideTarget] = useState('');

  // Query DNS Settings
  useQuery<{ dns_settings: DNSSettingsDTO }>({
    queryKey: ['dns_settings'],
    queryFn: async () => {
      const res = await fetch('/api/settings/dns', {
        headers: { Authorization: `Bearer ${localStorage.getItem('token') || ''}` },
      });
      if (!res.ok) throw new Error('Failed to fetch DNS settings');
      const data = await res.json();
      const s = data.settings || data.dns_settings;
      if (s) {
        setMagicSuffix(s.magic_dns_suffix || '.internal');
        setUpstreams(s.upstream_resolvers || '1.1.1.1, 8.8.8.8');
      }
      return data;
    },
  });

  // Query DNS Overrides
  const { data: overridesData } = useQuery<{ dns_overrides: DNSOverrideItem[] }>({
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
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['dns_settings'] });
      setToastMessage('DNS configuration updated successfully');
      setTimeout(() => setToastMessage(''), 3000);
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
          target_address: overrideTarget,
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
    },
  });

  return (
    <div className="space-y-5">
      {/* Toast Notification */}
      {toastMessage && (
        <div className="border border-emerald-500/30 bg-emerald-500/10 text-emerald-400 p-2.5 rounded-lg flex items-center gap-2 text-xs">
          <CheckCircle className="w-4 h-4 shrink-0" />
          {toastMessage}
        </div>
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
                  onChange={(e) => setMagicSuffix(e.target.value)}
                  placeholder="e.g. .strongnet or .corp.internal"
                  className="flex-1 surface-input border subtle-border rounded-lg px-2.5 py-1.5 font-mono text-xs focus:outline-none focus:border-blue-500"
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
              <code className="font-mono text-blue-400">[device-name]{magicSuffix}</code>
            </div>
          </div>
        </div>

        {/* Upstream Resolvers */}
        <div className="border subtle-border rounded-xl p-4 surface-card space-y-3 shadow-xs">
          <div className="border-b subtle-border pb-2">
            <h2 className="font-bold text-xs text-slate-900 dark:text-white">{t('dns.upstream_title')}</h2>
            <div className="text-[11px] text-dim">{t('dns.upstream_desc')}</div>
          </div>

          <div className="space-y-2 text-xs">
            <div>
              <label className="text-subtle text-[11px] block mb-1">{t('dns.ip_resolvers')}</label>
              <div className="flex gap-2">
                <input
                  type="text"
                  value={upstreams}
                  onChange={(e) => setUpstreams(e.target.value)}
                  placeholder="e.g. 1.1.1.1, 8.8.8.8, 9.9.9.9"
                  className="flex-1 surface-input border subtle-border rounded-lg px-2.5 py-1.5 font-mono text-xs focus:outline-none focus:border-blue-500"
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
            <div className="flex items-center gap-2 text-[10px] text-subtle">
              <span>{t('dns.suggestions')}:</span>
              <span className="bg-slate-500/10 border subtle-border px-1.5 py-0.5 rounded font-mono">1.1.1.1 (Cloudflare)</span>
              <span className="bg-slate-500/10 border subtle-border px-1.5 py-0.5 rounded font-mono">8.8.8.8 (Google)</span>
            </div>
          </div>
        </div>
      </div>

      {/* Custom DNS Overrides */}
      <div className="space-y-2">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <span className="font-semibold text-xs text-slate-900 dark:text-white tracking-tight">
              {t('dns.custom_overrides')} ({overrides.length})
            </span>
            <div className="text-[11px] text-dim">{t('dns.custom_overrides_desc')}</div>
          </div>
          <div className="flex items-center gap-2 flex-1 max-w-sm ml-auto">
            <div className="relative w-full">
              <Search className="w-3.5 h-3.5 text-subtle absolute left-2.5 top-1/2 -translate-y-1/2" />
              <input
                type="text"
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                placeholder="Search DNS records by host or IP..."
                className="w-full surface-input border subtle-border rounded-lg pl-8 pr-2.5 py-1.5 text-xs focus:outline-none focus:border-blue-500 font-sans"
              />
            </div>
            <button
              type="button"
              onClick={() => setIsAddOverrideOpen(true)}
              className="bg-blue-600 hover:bg-blue-500 text-white font-medium px-3 py-1.5 rounded-lg transition-colors flex items-center gap-1.5 shadow-xs text-xs shrink-0"
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
            Type:
          </span>
          {(['all', 'A', 'AAAA', 'CNAME', 'TXT'] as const).map((rType) => {
            const count = rType === 'all' ? overrides.length : overrides.filter((o) => o.record_type?.toUpperCase() === rType).length;
            const isSelected = typeFilter === rType;
            return (
              <button
                key={rType}
                type="button"
                onClick={() => setTypeFilter(rType)}
                className={`px-2 py-0.5 rounded-md border subtle-border transition-colors uppercase font-mono text-[10px] ${
                  isSelected
                    ? 'bg-blue-600/15 border-blue-500/40 text-blue-500 font-semibold'
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
              className="ml-auto text-rose-500 hover:text-rose-400 flex items-center gap-1 text-[11px] font-medium"
            >
              <X className="w-3 h-3" /> Reset filters
            </button>
          )}
        </div>

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
                {filteredOverrides.length === 0 ? (
                  <tr>
                    <td colSpan={4} className="py-8">
                      <EmptyState
                        icon={Globe}
                        title={searchQuery || typeFilter !== 'all' ? 'No matching DNS records' : 'No custom DNS overrides'}
                        description={searchQuery || typeFilter !== 'all' ? `No DNS records match the filter criteria.` : "Define custom authoritative local DNS overrides mapping names to specific internal IPs."}
                        actionLabel={searchQuery || typeFilter !== 'all' ? 'Clear Filters' : t('dns.add_override')}
                        actionIcon={searchQuery || typeFilter !== 'all' ? X : Plus}
                        onAction={searchQuery || typeFilter !== 'all' ? () => { setSearchQuery(''); setTypeFilter('all'); } : () => setIsAddOverrideOpen(true)}
                        className="border-none shadow-none bg-transparent"
                      />
                    </td>
                  </tr>
                ) : (
                  filteredOverrides.map((rec) => (
                    <tr key={rec.id} className="hover-row transition-colors">
                      <td className="py-2.5 px-3.5 font-sans font-medium text-blue-400">
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
                          onClick={() => deleteOverrideMutation.mutate(rec.id)}
                          className="p-1.5 hover:bg-rose-500/10 rounded-md text-rose-500 transition-colors"
                          title={t('common.delete')}
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
            <label className="text-subtle block mb-1">{t('dns.hostname')}</label>
            <input
              type="text"
              value={overrideHost}
              onChange={(e) => setOverrideHost(e.target.value)}
              placeholder="e.g. gitlab.internal"
              className="w-full surface-input border subtle-border rounded-lg px-2.5 py-1.5 font-mono text-xs focus:outline-none focus:border-blue-500"
            />
          </div>

          <div>
            <label className="text-subtle block mb-1">{t('dns.record_type')}</label>
            <select
              value={overrideType}
              onChange={(e) => setOverrideType(e.target.value)}
              className="w-full surface-input border subtle-border rounded-lg px-2.5 py-1.5 text-xs focus:outline-none"
            >
              <option value="A">A (IPv4 Address)</option>
              <option value="CNAME">CNAME (Alias)</option>
            </select>
          </div>

          <div>
            <label className="text-subtle block mb-1">{t('dns.points_to')}</label>
            <input
              type="text"
              value={overrideTarget}
              onChange={(e) => setOverrideTarget(e.target.value)}
              placeholder="e.g. 192.168.1.100"
              className="w-full surface-input border subtle-border rounded-lg px-2.5 py-1.5 font-mono text-xs focus:outline-none focus:border-blue-500"
            />
          </div>
        </div>

        <div className="flex gap-2 pt-2 border-t subtle-border">
          <button
            onClick={() => createOverrideMutation.mutate()}
            disabled={!overrideHost || !overrideTarget || createOverrideMutation.isPending}
            className="flex-1 bg-blue-600 hover:bg-blue-500 disabled:opacity-50 text-white font-medium py-1.5 rounded-lg shadow-xs transition-colors text-xs"
          >
            {createOverrideMutation.isPending ? 'Saving...' : t('common.save')}
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
    </div>
  );
};
