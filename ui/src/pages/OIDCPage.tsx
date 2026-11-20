import { useToast } from '../components/ToastProvider';
import React, { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { Key, Zap, Plus, Trash2, ShieldCheck, Search, Eye, EyeOff, X, Pencil } from 'lucide-react';
import { Modal } from '../components/Modal';
import { useConfirmAction } from '../components/ConfirmDialog';
import { CopyButton } from '../components/CopyButton';
import { EmptyState } from '../components/EmptyState';
import { TableQueryState } from '../components/TableQueryState';
import { TableActionLegend } from '../components/TableActionLegend';
import { QueryErrorBanner } from '../components/QueryErrorBanner';

interface OIDCProviderItem {
  id: string;
  name: string;
  provider_type: string;
  issuer_url: string;
  client_id: string;
  is_active: boolean;
  authorization_url?: string;
  token_url?: string;
  jwks_url?: string;
  userinfo_url?: string;
  claim_mapping?: string;
}

export const OIDCPage: React.FC = () => {
  const { t } = useTranslation();
  const toast = useToast();
  const queryClient = useQueryClient();
  const [isAddOpen, setIsAddOpen] = useState(false);
  const { requestConfirmation, confirmationDialog } = useConfirmAction();
  const [testResult, setTestResult] = useState<string | null>(null);
  const [testStatus, setTestStatus] = useState<'success' | 'warning' | 'error'>('success');
  const [searchQuery, setSearchQuery] = useState('');
  const [showSecret, setShowSecret] = useState(false);
  const [editProvider, setEditProvider] = useState<string | null>(null);
  const [providerActive, setProviderActive] = useState(true);
  const [authorizationURL, setAuthorizationURL] = useState('');
  const [tokenURL, setTokenURL] = useState('');
  const [jwksURL, setJWKSURL] = useState('');
  const [userinfoURL, setUserinfoURL] = useState('');
  const [claimMapping, setClaimMapping] = useState('');

  // Form states
  const [name, setName] = useState('');
  const [providerType, setProviderType] = useState('google');
  const [issuerURL, setIssuerURL] = useState('');
  const [clientID, setClientID] = useState('');
  const [clientSecret, setClientSecret] = useState('');

  const clearForm = () => {
    setEditProvider(null);
    setName(''); setProviderType('google'); setIssuerURL('https://accounts.google.com');
    setClientID(''); setClientSecret(''); setProviderActive(true);
    setAuthorizationURL(''); setTokenURL(''); setJWKSURL('');
    setUserinfoURL(''); setClaimMapping('');
  };
  const editForm = (p: OIDCProviderItem) => {
    setEditProvider(p.id); setName(p.name); setProviderType(p.provider_type);
    setIssuerURL(p.issuer_url); setClientID(p.client_id); setClientSecret('');
    setProviderActive(p.is_active);
    setAuthorizationURL(p.authorization_url || ''); setTokenURL(p.token_url || '');
    setJWKSURL(p.jwks_url || ''); setUserinfoURL(p.userinfo_url || '');
    setClaimMapping(p.claim_mapping || '');
    setIsAddOpen(true);
  };
  const { data, isPending: providersPending, isError: providersError, refetch: refetchProviders } = useQuery<{
    providers: OIDCProviderItem[];
    redirect_uri: string;
    external_origin_configured: boolean;
    production: boolean;
    redirect_uri_valid: boolean;
  }>({
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
  const redirectURI = data?.redirect_uri || '';
  const redirectReady = Boolean(data?.redirect_uri_valid && redirectURI);
  const isLocalPreview = Boolean(redirectReady && !data?.external_origin_configured);

  const filteredProviders = providers.filter((p) => {
    if (!searchQuery.trim()) return true;
    const q = searchQuery.toLowerCase();
    return p.name.toLowerCase().includes(q) || p.issuer_url.toLowerCase().includes(q) || p.client_id.toLowerCase().includes(q);
  });

  const createProviderMutation = useMutation({
    mutationFn: async () => {
      const res = await fetch(editProvider ? `/api/oidc/providers/${editProvider}` : '/api/oidc/providers', {
        method: editProvider ? 'PUT' : 'POST',
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
          authorization_url: providerType === 'custom' ? authorizationURL.trim() : '',
          token_url: providerType === 'custom' ? tokenURL.trim() : '',
          jwks_url: providerType === 'custom' ? jwksURL.trim() : '',
          userinfo_url: providerType === 'custom' ? userinfoURL.trim() : '',
          claim_mapping: providerType === 'custom' ? claimMapping.trim() : '',
          is_active: providerActive,
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
      toast.success(t('common.operation_saved'));
      setIsAddOpen(false);
      createProviderMutation.reset();
      clearForm();
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
      toast.success(t('common.operation_deleted'));
    },
  });

  const handleTestConnection = async (id: string) => {
    try {
      const res = await fetch(`/api/oidc/providers/${id}/test`, {
        method: 'POST',
        headers: { Authorization: `Bearer ${localStorage.getItem('token') || ''}` },
      });
      const resData = await res.json();
      if (res.ok && resData.success) {
        setTestStatus('success');
        setTestResult(t('oidc.discovery_verified', { endpoint: resData.authorization_endpoint || '' }));
      } else {
        setTestStatus('warning');
        setTestResult(t('oidc.discovery_warning', { message: resData.message || res.statusText }));
      }
    } catch (error: unknown) {
      setTestStatus('error');
      setTestResult(t('oidc.discovery_failed', { message: error instanceof Error ? error.message : t('common.error_occurred') }));
    }
  };

  return (
    <div className="space-y-4">
      {providersError && <QueryErrorBanner onRetry={() => void refetchProviders()} />}
      {/* Redirect URI Guidance & Copy Box */}
      <div className="border subtle-border rounded-xl p-4 surface-card space-y-3 shadow-xs">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="min-w-0">
          <div className="font-semibold text-sm text-slate-900 dark:text-white">{t('oidc.redirect_uri')}</div>
          <div className="text-xs text-dim mt-1">
            {t('oidc.redirect_uri_hint')}
          </div>
        </div>
          <div className="flex items-center gap-2 min-w-0 max-w-full">
          <code className="min-w-0 bg-slate-500/10 border subtle-border px-3 py-2 rounded text-xs font-mono text-accent break-all">
            {redirectReady ? redirectURI : t('common.unavailable')}
          </code>
          {redirectReady && <CopyButton value={redirectURI} label={t('oidc.redirect_uri_copy_label')} />}
        </div>
        </div>
        <p role={!redirectReady ? 'alert' : 'status'} className={`text-xs flex items-center gap-2 ${redirectReady && !isLocalPreview ? 'text-status-success' : 'text-status-warning'}`}>
          {redirectReady && !isLocalPreview ? t('oidc.redirect_verified')
            : isLocalPreview ? t('oidc.redirect_local_preview')
            : t('oidc.redirect_not_configured')}
        </p>
        {redirectReady && !redirectURI.startsWith('https://') && <p className="text-xs text-status-warning" role="note">{t('oidc.redirect_https_required')}</p>}
      </div>

      {testResult && (
        <div role={testStatus === 'error' ? 'alert' : 'status'}
          className={`border p-2.5 rounded-lg text-xs flex items-center gap-2 ${testStatus === 'success'
            ? 'border-emerald-500/30 bg-emerald-500/10 text-status-success'
            : 'border-amber-500/30 bg-amber-500/10 text-status-warning'}`}>
          <ShieldCheck className="w-4 h-4 shrink-0 text-accent" />
          {testResult}
        </div>
      )}

      {/* Providers Table */}
      <div className="border subtle-border rounded-xl overflow-hidden surface-card shadow-xs">
        <div className="p-3 border-b subtle-border flex flex-col items-stretch gap-3 lg:flex-row lg:items-center lg:justify-between">
          <span className="font-semibold text-xs text-slate-900 dark:text-white">
            {t('oidc.title')} ({providers.length})
          </span>
          <div className="flex flex-col sm:flex-row items-stretch gap-2 w-full min-w-0 lg:max-w-sm lg:ml-auto">
            <div className="relative flex-1 min-w-0">
              <Search className="w-3.5 h-3.5 text-subtle absolute left-2.5 top-1/2 -translate-y-1/2" />
              <input
                type="text"
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                placeholder={t('oidc.search_placeholder')}
                className="w-full surface-input border subtle-border rounded-lg pl-8 pr-2.5 py-1.5 text-xs focus:outline-none focus:border-blue-500"
              />
            </div>
            <button
              type="button"
              data-action="add-oidc"
              onClick={() => { clearForm(); setIsAddOpen(true); }}
              disabled={!redirectReady}
              className="bg-blue-600 hover:bg-blue-500 text-white font-medium px-3 py-1.5 rounded-lg transition-colors flex items-center justify-center gap-1.5 shadow-xs text-xs shrink-0 whitespace-nowrap w-full sm:w-auto"
            >
              <Plus className="w-3.5 h-3.5" />
              {t('oidc.add_provider')}
            </button>
          </div>
        </div>

        <div className="px-3"><TableActionLegend actions={[{ icon: Zap, label: t('oidc.test_handshake') }, { icon: Pencil, label: t('common.edit') }, { icon: Trash2, label: t('common.delete') }]} /></div>
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
              {providersPending || (providersError && !data) ? (
                <TableQueryState columns={6} error={providersError} onRetry={() => void refetchProviders()} />
              ) : filteredProviders.length === 0 ? (
                <tr>
                  <td colSpan={6} className="py-8">
                    <EmptyState
                      icon={Key}
                      title={searchQuery ? t('oidc.no_matching_providers') : t('oidc.no_providers')}
                      description={searchQuery ? t('oidc.no_matching_providers_description', { query: searchQuery }) : t('oidc.no_providers_description')}
                      actionLabel={searchQuery ? t('common.clear_search') : t('oidc.add_provider')}
                      actionIcon={searchQuery ? X : Plus}
                      onAction={searchQuery ? () => setSearchQuery('') : redirectReady ? () => { clearForm(); setIsAddOpen(true); } : undefined}
                      className="border-none shadow-none bg-transparent"
                    />
                  </td>
                </tr>
              ) : (
                filteredProviders.map((p) => (
                  <tr key={p.id} className="hover-row transition-colors">
                    <td className="py-2.5 px-3.5 font-medium text-slate-900 dark:text-white flex items-center gap-2">
                      <Key className="w-3.5 h-3.5 text-accent shrink-0" />
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
                      <span className={p.is_active ? 'bg-emerald-500/10 text-status-success border border-emerald-500/20 px-2 py-0.5 rounded-md text-[10px] font-mono font-medium' : 'bg-slate-500/10 text-subtle px-2 py-0.5 rounded-md text-[10px] font-mono font-medium'}>
                        {t(p.is_active ? 'common.active' : 'common.disabled')}
                      </span>
                    </td>
                    <td className="py-2.5 px-3.5 text-right space-x-1">
                      <button
                        type="button"
                        onClick={() => handleTestConnection(p.id)}
                        className="p-1.5 hover:bg-blue-500/10 rounded-md text-accent transition-colors inline-block"
                        title={t('oidc.test_handshake')} data-tooltip={t('oidc.test_handshake')}
                        aria-label={`${t('oidc.test_handshake')} ${p.name}`}
                      >
                        <Zap className="w-3.5 h-3.5" />
                      </button>
                      <button type="button" onClick={() => editForm(p)} className="p-1.5 hover:bg-blue-500/10 rounded-md text-accent transition-colors inline-block" title={t('common.edit')} data-tooltip={t('common.edit')} aria-label={`${t('common.edit')} ${p.name}`}>
                        <Pencil className="w-3.5 h-3.5" />
                      </button>
                      <button
                        type="button"
                        onClick={() => requestConfirmation({
                          title: t('confirm.delete_provider_title'),
                          description: t('confirm.delete_provider_description', { name: p.name }),
                          confirmLabel: t('common.delete'),
                          onConfirm: () => deleteProviderMutation.mutateAsync(p.id),
                        })}
                        className="p-1.5 hover:bg-rose-500/10 rounded-md text-status-danger transition-colors inline-block"
                        title={t('common.delete')} data-tooltip={t('common.delete')}
                        aria-label={`${t('common.delete')} ${p.name}`}
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
          clearForm();
        }}
        title={editProvider ? t('oidc.edit_provider') : t('oidc.add_provider')}
        error={(createProviderMutation.error as Error)?.message}
      >
        <div className="space-y-3 text-xs">
          <div>
            <label className="text-subtle block mb-1">{t('oidc.provider_type')}</label>
            <select
              value={providerType}
              onChange={(e) => {
                const val = e.target.value;
                setProviderType(val);
                if (val === 'google') setIssuerURL('https://accounts.google.com');
                if (val === 'microsoft') setIssuerURL('');
                if (val === 'keycloak' || val === 'custom') setIssuerURL('');
              }}
              className="w-full surface-input border subtle-border rounded-lg px-2.5 py-1.5 text-xs focus:outline-none"
            >
              <option value="google">Google Workspace</option>
              <option value="microsoft">Microsoft Entra ID (Azure AD)</option>
              <option value="keycloak">Keycloak</option>
              <option value="custom">{t('oidc.custom_provider')}</option>
            </select>
          </div>

          <div>
            <label className="text-subtle block mb-1">{t('oidc.idp_name')}</label>
            <input
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder={t('oidc.provider_name_placeholder')}
              className="w-full surface-input border subtle-border rounded-lg px-2.5 py-1.5 text-xs focus:outline-none focus:border-blue-500"
            />
          </div>

          <div>
            <label className="text-subtle block mb-1">{t('oidc.discovery_url')}</label>
            <input
              type="text"
              value={issuerURL}
              onChange={(e) => setIssuerURL(e.target.value)}
              placeholder={providerType === "microsoft" ? "https://login.microsoftonline.com/<tenant-id>/v2.0" : providerType === "keycloak" ? "https://sso.example.com/realms/company" : "https://accounts.google.com"}
              className="w-full surface-input border subtle-border rounded-lg px-2.5 py-1.5 font-mono text-xs focus:outline-none focus:border-blue-500"
            />
          </div>

          <div>
            <label className="text-subtle block mb-1">{t('oidc.client_id')}</label>
            <input
              type="text"
              value={clientID}
              onChange={(e) => setClientID(e.target.value)}
              placeholder={t('oidc.client_id_placeholder')}
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
                title={showSecret ? t('oidc.hide_secret') : t('oidc.show_secret')}
                aria-label={showSecret ? t('oidc.hide_secret') : t('oidc.show_secret')}
              >
                {showSecret ? <EyeOff className="w-3.5 h-3.5" /> : <Eye className="w-3.5 h-3.5" />}
              </button>
            </div>
          </div>
          {providerType === 'custom' && <>
            <p className="text-[11px] text-subtle">{t('oidc.discovery_endpoints_hint')}</p>
            {[
              [t('oidc.authorization_endpoint'), authorizationURL, setAuthorizationURL, 'https://id.example.com/oauth/authorize'],
              [t('oidc.token_endpoint'), tokenURL, setTokenURL, 'https://id.example.com/oauth/token'],
              [t('oidc.jwks_endpoint'), jwksURL, setJWKSURL, 'https://id.example.com/.well-known/jwks.json'],
              [t('oidc.userinfo_endpoint'), userinfoURL, setUserinfoURL, 'https://id.example.com/userinfo'],
            ].map(([label, value, setter, placeholder]) => (
              <div key={label as string}>
                <label className="text-subtle block mb-1">{label as string}</label>
                <input type="url" value={value as string} onChange={e => (setter as React.Dispatch<React.SetStateAction<string>>)(e.target.value)} placeholder={placeholder as string}
                  className="w-full surface-input border subtle-border rounded-lg px-2.5 py-1.5 font-mono text-xs" />
              </div>
            ))}
            <div>
              <label className="text-subtle block mb-1">{t('oidc.claim_mapping_label')}</label>
              <textarea rows={3} value={claimMapping} onChange={e => setClaimMapping(e.target.value)}
                placeholder={'{"email":"email","name":"name","email_verified":"email_verified"}'}
                className="w-full surface-input border subtle-border rounded-lg px-2.5 py-1.5 font-mono text-xs" />
              <p className="text-[10px] text-subtle">{t('oidc.immutable_subject_hint')}</p>
            </div>
          </>}
          <label className="flex gap-2 items-center text-subtle">
            <input type="checkbox" checked={providerActive} onChange={e => setProviderActive(e.target.checked)} /> {t('oidc.enabled')}
          </label>
        </div>

        <div className="flex gap-2 pt-2 border-t subtle-border">
          <button
            type="button"
            onClick={() => createProviderMutation.mutate()}
            disabled={!name.trim() || !issuerURL.trim() || !clientID.trim() || (!editProvider && !clientSecret.trim()) || createProviderMutation.isPending}
            className="flex-1 bg-blue-600 hover:bg-blue-500 disabled:opacity-50 text-white font-medium py-1.5 rounded-lg shadow-xs transition-colors text-xs"
          >
            {createProviderMutation.isPending ? t('common.saving') : t('common.save')}
          </button>
          <button
            type="button"
            onClick={() => {
              setIsAddOpen(false);
              createProviderMutation.reset();
              clearForm();
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
