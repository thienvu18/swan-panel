import React, { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { Key, Zap, Plus, Trash2, ShieldCheck, Search, Eye, EyeOff, X } from 'lucide-react';
import { Modal } from '../components/Modal';
import { CopyButton } from '../components/CopyButton';
import { EmptyState } from '../components/EmptyState';

interface OIDCProviderItem {
  id: string;
  name: string;
  provider_type: string;
  issuer_url: string;
  client_id: string;
  is_active: boolean;
}

export const OIDCPage: React.FC = () => {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const [isAddOpen, setIsAddOpen] = useState(false);
  const [testResult, setTestResult] = useState<string | null>(null);
  const [searchQuery, setSearchQuery] = useState('');
  const [showSecret, setShowSecret] = useState(false);

  // Form states
  const [name, setName] = useState('');
  const [providerType, setProviderType] = useState('google');
  const [issuerURL, setIssuerURL] = useState('');
  const [clientID, setClientID] = useState('');
  const [clientSecret, setClientSecret] = useState('');

  const redirectURI = `${window.location.origin}/api/auth/oidc/callback`;

  const { data } = useQuery<{ providers: OIDCProviderItem[] }>({
    queryKey: ['oidc_providers'],
    queryFn: async () => {
      const res = await fetch('/api/oidc/providers', {
        headers: { Authorization: `Bearer ${localStorage.getItem('token') || ''}` },
      });
      if (!res.ok) throw new Error('Failed to fetch OIDC providers');
      return res.json();
    },
  });

  const providers = data?.providers || [];

  const filteredProviders = providers.filter((p) => {
    if (!searchQuery.trim()) return true;
    const q = searchQuery.toLowerCase();
    return p.name.toLowerCase().includes(q) || p.issuer_url.toLowerCase().includes(q) || p.client_id.toLowerCase().includes(q);
  });

  const createProviderMutation = useMutation({
    mutationFn: async () => {
      const res = await fetch('/api/oidc/providers', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${localStorage.getItem('token') || ''}`,
        },
        body: JSON.stringify({
          name,
          provider_type: providerType,
          issuer_url: issuerURL,
          client_id: clientID,
          client_secret: clientSecret,
        }),
      });
      if (!res.ok) {
        const errData = await res.json().catch(() => ({}));
        throw new Error(errData.error || t('common.error_occurred') || 'Failed to create provider');
      }
      return res.json();
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['oidc_providers'] });
      setIsAddOpen(false);
      createProviderMutation.reset();
      setName('');
      setIssuerURL('');
      setClientID('');
      setClientSecret('');
    },
  });

  const deleteProviderMutation = useMutation({
    mutationFn: async (id: string) => {
      const res = await fetch(`/api/oidc/providers/${id}`, {
        method: 'DELETE',
        headers: { Authorization: `Bearer ${localStorage.getItem('token') || ''}` },
      });
      if (!res.ok) throw new Error('Failed to delete provider');
      return res.json();
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['oidc_providers'] });
    },
  });

  const handleTestConnection = async (id: string) => {
    try {
      const res = await fetch(`/api/oidc/providers/${id}/test`, {
        method: 'POST',
        headers: { Authorization: `Bearer ${localStorage.getItem('token') || ''}` },
      });
      const resData = await res.json();
      if (resData.success) {
        setTestResult(`Connection handshake verified successfully with ${resData.authorization_endpoint}`);
      } else {
        setTestResult(`Discovery test warning: ${resData.message}`);
      }
    } catch (err: any) {
      setTestResult(`Discovery test failed: ${err.message}`);
    }
    setTimeout(() => setTestResult(null), 5000);
  };

  return (
    <div className="space-y-4">
      {/* Redirect URI Guidance & Copy Box */}
      <div className="border subtle-border rounded-xl p-3.5 surface-card flex flex-wrap items-center justify-between gap-3 shadow-xs">
        <div>
          <div className="font-semibold text-xs text-slate-900 dark:text-white">{t('oidc.redirect_uri')}</div>
          <div className="text-[11px] text-dim">
            Register this callback URL inside your Google Cloud Console or Microsoft Azure App Registration.
          </div>
        </div>
        <div className="flex items-center gap-2">
          <code className="bg-slate-500/10 border subtle-border px-2.5 py-1 rounded text-xs font-mono text-blue-400">
            {redirectURI}
          </code>
          <CopyButton value={redirectURI} label="OIDC Redirect URI" />
        </div>
      </div>

      {testResult && (
        <div className="border border-blue-500/30 bg-blue-500/10 text-blue-300 p-2.5 rounded-lg text-xs flex items-center gap-2">
          <ShieldCheck className="w-4 h-4 shrink-0 text-blue-400" />
          {testResult}
        </div>
      )}

      {/* Providers Table */}
      <div className="border subtle-border rounded-xl overflow-hidden surface-card shadow-xs">
        <div className="p-3 border-b subtle-border flex flex-wrap items-center justify-between gap-3">
          <span className="font-semibold text-xs text-slate-900 dark:text-white">
            {t('oidc.title')} ({providers.length})
          </span>
          <div className="flex items-center gap-2 flex-1 max-w-sm ml-auto">
            <div className="relative w-full">
              <Search className="w-3.5 h-3.5 text-subtle absolute left-2.5 top-1/2 -translate-y-1/2" />
              <input
                type="text"
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                placeholder="Search SSO providers..."
                className="w-full surface-input border subtle-border rounded-lg pl-8 pr-2.5 py-1.5 text-xs focus:outline-none focus:border-blue-500"
              />
            </div>
            <button
              type="button"
              data-action="add-oidc"
              onClick={() => setIsAddOpen(true)}
              className="bg-blue-600 hover:bg-blue-500 text-white font-medium px-3 py-1.5 rounded-lg transition-colors flex items-center gap-1.5 shadow-xs text-xs shrink-0"
            >
              <Plus className="w-3.5 h-3.5" />
              {t('oidc.add_provider')}
            </button>
          </div>
        </div>

        <div className="overflow-x-auto">
          <table className="w-full text-left text-xs min-w-[700px]">
            <thead className="border-b subtle-border surface-header text-subtle font-semibold text-[11px]">
              <tr>
                <th className="py-2.5 px-3.5">{t('oidc.idp_name')}</th>
                <th className="py-2.5 px-3.5">{t('oidc.protocol')}</th>
                <th className="py-2.5 px-3.5">{t('oidc.discovery_url')}</th>
                <th className="py-2.5 px-3.5">{t('oidc.client_id')}</th>
                <th className="py-2.5 px-3.5">{t('oidc.status')}</th>
                <th className="py-2.5 px-3.5 text-right">{t('common.actions')}</th>
              </tr>
            </thead>
            <tbody className="divide-y subtle-border">
              {filteredProviders.length === 0 ? (
                <tr>
                  <td colSpan={6} className="py-8">
                    <EmptyState
                      icon={Key}
                      title={searchQuery ? 'No matching providers' : 'No OIDC identity providers configured'}
                      description={searchQuery ? `No SSO providers match '${searchQuery}'.` : 'Connect Google Workspace, Microsoft Entra ID, or Okta for federated zero-trust authentication.'}
                      actionLabel={searchQuery ? 'Clear Search' : t('oidc.add_provider')}
                      actionIcon={searchQuery ? X : Plus}
                      onAction={searchQuery ? () => setSearchQuery('') : () => setIsAddOpen(true)}
                      className="border-none shadow-none bg-transparent"
                    />
                  </td>
                </tr>
              ) : (
                filteredProviders.map((p) => (
                  <tr key={p.id} className="hover-row transition-colors">
                    <td className="py-2.5 px-3.5 font-medium text-slate-900 dark:text-white flex items-center gap-2">
                      <Key className="w-3.5 h-3.5 text-blue-400 shrink-0" />
                      <span>{p.name}</span>
                    </td>
                    <td className="py-2.5 px-3.5 text-subtle">OpenID Connect</td>
                    <td className="py-2.5 px-3.5 font-mono text-[11px] text-subtle">
                      <div className="flex items-center gap-1.5">
                        <span className="truncate max-w-[200px]">{p.issuer_url}</span>
                        <CopyButton value={p.issuer_url} label={`Issuer URL ${p.name}`} />
                      </div>
                    </td>
                    <td className="py-2.5 px-3.5 font-mono text-[11px] text-subtle">
                      <div className="flex items-center gap-1.5">
                        <span className="truncate max-w-[150px]">{p.client_id}</span>
                        <CopyButton value={p.client_id} label={`Client ID ${p.name}`} />
                      </div>
                    </td>
                    <td className="py-2.5 px-3.5">
                      <span className="bg-emerald-500/10 text-emerald-500 border border-emerald-500/20 px-2 py-0.5 rounded-md text-[10px] font-mono font-medium">
                        Active
                      </span>
                    </td>
                    <td className="py-2.5 px-3.5 text-right space-x-1">
                      <button
                        type="button"
                        onClick={() => handleTestConnection(p.id)}
                        className="p-1.5 hover:bg-blue-500/10 rounded-md text-blue-500 transition-colors inline-block"
                        title={t('oidc.test_handshake')}
                      >
                        <Zap className="w-3.5 h-3.5" />
                      </button>
                      <button
                        type="button"
                        onClick={() => deleteProviderMutation.mutate(p.id)}
                        className="p-1.5 hover:bg-rose-500/10 rounded-md text-rose-500 transition-colors inline-block"
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

      {/* MODAL: ADD OIDC PROVIDER */}
      <Modal
        isOpen={isAddOpen}
        onClose={() => {
          setIsAddOpen(false);
          createProviderMutation.reset();
        }}
        title={t('oidc.add_provider')}
        error={(createProviderMutation.error as Error)?.message}
      >
        <div className="space-y-3 text-xs">
          <div>
            <label className="text-subtle block mb-1">Provider Type</label>
            <select
              value={providerType}
              onChange={(e) => {
                const val = e.target.value;
                setProviderType(val);
                if (val === 'google') setIssuerURL('https://accounts.google.com');
                if (val === 'microsoft') setIssuerURL('https://login.microsoftonline.com/common/v2.0');
              }}
              className="w-full surface-input border subtle-border rounded-lg px-2.5 py-1.5 text-xs focus:outline-none"
            >
              <option value="google">Google Workspace</option>
              <option value="microsoft">Microsoft Entra ID (Azure AD)</option>
              <option value="keycloak">Keycloak</option>
              <option value="custom">Custom OIDC Provider</option>
            </select>
          </div>

          <div>
            <label className="text-subtle block mb-1">{t('oidc.idp_name')}</label>
            <input
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="e.g. Corporate Google SSO"
              className="w-full surface-input border subtle-border rounded-lg px-2.5 py-1.5 text-xs focus:outline-none focus:border-blue-500"
            />
          </div>

          <div>
            <label className="text-subtle block mb-1">{t('oidc.discovery_url')}</label>
            <input
              type="text"
              value={issuerURL}
              onChange={(e) => setIssuerURL(e.target.value)}
              placeholder="e.g. https://accounts.google.com"
              className="w-full surface-input border subtle-border rounded-lg px-2.5 py-1.5 font-mono text-xs focus:outline-none focus:border-blue-500"
            />
          </div>

          <div>
            <label className="text-subtle block mb-1">{t('oidc.client_id')}</label>
            <input
              type="text"
              value={clientID}
              onChange={(e) => setClientID(e.target.value)}
              placeholder="Client ID from IdP console"
              className="w-full surface-input border subtle-border rounded-lg px-2.5 py-1.5 font-mono text-xs focus:outline-none focus:border-blue-500"
            />
          </div>

          <div>
            <label className="text-subtle block mb-1">{t('oidc.client_secret')}</label>
            <div className="relative">
              <input
                type={showSecret ? 'text' : 'password'}
                value={clientSecret}
                onChange={(e) => setClientSecret(e.target.value)}
                placeholder="Client Secret"
                className="w-full surface-input border subtle-border rounded-lg pl-2.5 pr-8 py-1.5 font-mono text-xs focus:outline-none focus:border-blue-500"
              />
              <button
                type="button"
                onClick={() => setShowSecret(!showSecret)}
                className="absolute right-2 top-1/2 -translate-y-1/2 text-subtle hover:text-current p-1 rounded"
                title={showSecret ? 'Hide secret' : 'Show secret'}
                aria-label={showSecret ? 'Hide secret' : 'Show secret'}
              >
                {showSecret ? <EyeOff className="w-3.5 h-3.5" /> : <Eye className="w-3.5 h-3.5" />}
              </button>
            </div>
          </div>
        </div>

        <div className="flex gap-2 pt-2 border-t subtle-border">
          <button
            type="button"
            onClick={() => createProviderMutation.mutate()}
            disabled={!name || !issuerURL || !clientID || createProviderMutation.isPending}
            className="flex-1 bg-blue-600 hover:bg-blue-500 disabled:opacity-50 text-white font-medium py-1.5 rounded-lg shadow-xs transition-colors text-xs"
          >
            {createProviderMutation.isPending ? 'Saving...' : t('common.save')}
          </button>
          <button
            type="button"
            onClick={() => {
              setIsAddOpen(false);
              createProviderMutation.reset();
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
