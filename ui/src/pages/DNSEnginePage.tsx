import React, { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { Globe, Plus, Trash2, X, CheckCircle } from 'lucide-react';

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
      if (!res.ok) {
        return {
          dns_settings: {
            id: 1,
            magic_dns_suffix: '.strongnet',
            upstream_resolvers: '1.1.1.1, 8.8.8.8',
            is_magic_dns_enabled: true,
          },
        };
      }
      const data = await res.json();
      if (data.dns_settings) {
        setMagicSuffix(data.dns_settings.magic_dns_suffix);
        setUpstreams(data.dns_settings.upstream_resolvers);
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
      if (!res.ok) {
        return {
          dns_overrides: [
            { id: 'dns_1', hostname: 'gitlab.internal', record_type: 'A', target_address: '192.168.1.100' },
            { id: 'dns_2', hostname: 'k8s-cluster.corp', record_type: 'A', target_address: '172.16.0.10' },
          ],
        };
      }
      return res.json();
    },
  });

  const overrides = overridesData?.dns_overrides || [];

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
      if (!res.ok) throw new Error('Failed to create DNS override');
      return res.json();
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['dns_overrides'] });
      setIsAddOverrideOpen(false);
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
            <h2 className="font-bold text-xs text-white">{t('dns.magic_dns_title')}</h2>
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
            <h2 className="font-bold text-xs text-white">{t('dns.upstream_title')}</h2>
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
        <div className="flex items-center justify-between">
          <div>
            <span className="font-semibold text-xs text-white tracking-tight">{t('dns.custom_overrides')} ({overrides.length})</span>
            <div className="text-[11px] text-dim">{t('dns.custom_overrides_desc')}</div>
          </div>
          <button
            onClick={() => setIsAddOverrideOpen(true)}
            className="bg-blue-600 hover:bg-blue-500 text-white font-medium px-3 py-1.5 rounded-lg transition-colors flex items-center gap-1.5 shadow-xs text-xs"
          >
            <Plus className="w-3.5 h-3.5" />
            {t('dns.add_override')}
          </button>
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
                {overrides.map((rec) => (
                  <tr key={rec.id} className="hover-row transition-colors">
                    <td className="py-2.5 px-3.5 font-sans font-medium text-blue-400 flex items-center gap-2">
                      <Globe className="w-3.5 h-3.5 text-subtle" />
                      {rec.hostname}
                    </td>
                    <td className="py-2.5 px-3.5 text-subtle">{rec.record_type}</td>
                    <td className="py-2.5 px-3.5 text-white">{rec.target_address}</td>
                    <td className="py-2.5 px-3.5 text-right font-sans">
                      <button
                        onClick={() => deleteOverrideMutation.mutate(rec.id)}
                        className="p-1.5 hover:bg-rose-500/10 rounded-md text-rose-500 transition-colors"
                        title={t('common.delete')}
                      >
                        <Trash2 className="w-3.5 h-3.5" />
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      </div>

      {/* MODAL: ADD DNS OVERRIDE */}
      {isAddOverrideOpen && (
        <div className="fixed inset-0 bg-black/60 z-50 flex items-center justify-center p-4">
          <div className="surface-card border subtle-border rounded-xl max-w-sm w-full p-5 space-y-4 shadow-2xl">
            <div className="flex items-center justify-between border-b subtle-border pb-2.5">
              <h3 className="font-bold text-sm text-white">{t('dns.add_override')}</h3>
              <button onClick={() => setIsAddOverrideOpen(false)} className="text-subtle hover:text-current">
                <X className="w-4 h-4" />
              </button>
            </div>

            <div className="space-y-3 text-xs">
              <div>
                <label className="text-subtle block mb-1">{t('dns.hostname')}</label>
                <input
                  type="text"
                  value={overrideHost}
                  onChange={(e) => setOverrideHost(e.target.value)}
                  placeholder="e.g. gitlab.internal"
                  className="w-full surface-input border subtle-border rounded-lg px-2.5 py-1.5 font-mono text-xs focus:outline-none"
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
                  className="w-full surface-input border subtle-border rounded-lg px-2.5 py-1.5 font-mono text-xs focus:outline-none"
                />
              </div>
            </div>

            <div className="flex gap-2 pt-2 border-t subtle-border">
              <button
                onClick={() => createOverrideMutation.mutate()}
                disabled={!overrideHost || !overrideTarget || createOverrideMutation.isPending}
                className="flex-1 bg-blue-600 hover:bg-blue-500 disabled:opacity-50 text-white font-medium py-1.5 rounded-lg shadow-xs transition-colors text-xs"
              >
                {t('common.save')}
              </button>
              <button
                onClick={() => setIsAddOverrideOpen(false)}
                className="px-3 py-1.5 border subtle-border rounded-lg hover:bg-slate-500/10 text-xs"
              >
                {t('common.cancel')}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};
