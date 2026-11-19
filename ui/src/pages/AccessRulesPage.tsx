import { useToast } from '../components/ToastProvider';
import React, { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { Info, Plus, Trash2, Search, ShieldAlert, ShieldCheck, Filter, X } from 'lucide-react';
import { Modal } from '../components/Modal';
import { useConfirmAction } from '../components/ConfirmDialog';
import { CopyButton } from '../components/CopyButton';
import { EmptyState } from '../components/EmptyState';
import { TableQueryState } from '../components/TableQueryState';
import { TableActionLegend } from '../components/TableActionLegend';
import { QueryErrorBanner } from '../components/QueryErrorBanner';
import { EffectiveAccessPanel } from '../components/EffectiveAccessPanel';

interface AccessRuleItem {
  id: string;
  subject_type: 'user_group' | 'user' | 'device';
  subject_id: string;
  subject_name: string;
  action: 'ALLOW' | 'DENY';
  target_type: 'resource_group' | 'cidr_ip';
  target_value: string;
  target_name: string;
  rule_nature: string;
}

const subjectFieldTranslations = {
  user_group: { label: 'access.select_subject_group', placeholder: 'access.select_subject_group_placeholder' },
  user: { label: 'access.select_subject_user', placeholder: 'access.select_subject_user_placeholder' },
  device: { label: 'access.select_subject_device', placeholder: 'access.select_subject_device_placeholder' },
} as const;

export const AccessRulesPage: React.FC = () => {
  const { t } = useTranslation();
  const toast = useToast();
  const queryClient = useQueryClient();
  const [isAddOpen, setIsAddOpen] = useState(false);
  const { requestConfirmation, confirmationDialog } = useConfirmAction();
  const [searchQuery, setSearchQuery] = useState('');
  const [actionFilter, setActionFilter] = useState<'all' | 'ALLOW' | 'DENY'>('all');

  // Form states
  const [subjectType, setSubjectType] = useState<'user_group' | 'user' | 'device'>('user_group');
  const subjectField = subjectFieldTranslations[subjectType];
  const [subjectID, setSubjectID] = useState('');
  const [action, setAction] = useState<'ALLOW' | 'DENY'>('ALLOW');
  const [targetType, setTargetType] = useState<'resource_group' | 'cidr_ip'>('resource_group');
  const [targetValue, setTargetValue] = useState('');

  const { data, isPending: rulesPending, isError: rulesError, refetch: refetchRules } = useQuery<{ access_rules: AccessRuleItem[] }>({
    queryKey: ['access_rules'],
    queryFn: async () => {
      const res = await fetch('/api/access-rules', {
        headers: { Authorization: `Bearer ${localStorage.getItem('token') || ''}` },
      });
      if (!res.ok) throw new Error('Failed to fetch access rules');
      return res.json();
    },
  });

  const rules = data?.access_rules || [];
  const groupsQuery = useQuery<{ user_groups: { id: string; name: string }[] }>({
    queryKey: ['user_groups'],
    queryFn: async () => {
      const response = await fetch('/api/user-groups', {
        headers: { Authorization: `Bearer ${localStorage.getItem('token') || ''}` },
      });
      if (!response.ok) throw new Error('Failed to load groups');
      return response.json();
    },
  });
  const usersQuery = useQuery<{ users: { id: string; name: string; email: string; status: string }[] }>({
    queryKey: ['users'],
    queryFn: async () => {
      const response = await fetch('/api/users', {
        headers: { Authorization: `Bearer ${localStorage.getItem('token') || ''}` },
      });
      if (!response.ok) throw new Error('Failed to load users');
      return response.json();
    },
  });
  const devicesQuery = useQuery<{ devices: { id: string; hostname: string; is_revoked: boolean }[] }>({
    queryKey: ['devices'],
    queryFn: async () => {
      const response = await fetch('/api/devices', {
        headers: { Authorization: `Bearer ${localStorage.getItem('token') || ''}` },
      });
      if (!response.ok) throw new Error('Failed to load devices');
      return response.json();
    },
  });
  const targetsQuery = useQuery<{ resource_groups: { id: string; name: string; cidr_block: string }[] }>({
    queryKey: ['resource_groups'],
    queryFn: async () => {
      const response = await fetch('/api/resource-groups', {
        headers: { Authorization: `Bearer ${localStorage.getItem('token') || ''}` },
      });
      if (!response.ok) throw new Error('Failed to load resource groups');
      return response.json();
    },
  });
  const availableSubjects = subjectType === 'user_group'
    ? (groupsQuery.data?.user_groups || []).map(group => ({ id: group.id, label: group.name }))
    : subjectType === 'user'
      ? (usersQuery.data?.users || []).filter(user => user.status === 'ACTIVE')
        .map(user => ({ id: user.id, label: `${user.name} (${user.email})` }))
      : (devicesQuery.data?.devices || []).filter(device => !device.is_revoked)
        .map(device => ({ id: device.id, label: device.hostname }));
  const selectedSubjectLabel = availableSubjects.find(subject => subject.id === subjectID)?.label || subjectID;
  const selectedTargetLabel = targetType === 'resource_group'
    ? targetsQuery.data?.resource_groups?.find(group => group.id === targetValue)?.name || targetValue
    : targetValue;

  const createRuleMutation = useMutation({
    mutationFn: async () => {
      const res = await fetch('/api/access-rules', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${localStorage.getItem('token') || ''}`,
        },
        body: JSON.stringify({
          subject_type: subjectType,
          subject_id: subjectID,
          action,
          target_type: targetType,
          target_value: targetValue,
        }),
      });
      if (!res.ok) {
        const errData = await res.json().catch(() => ({}));
        throw new Error(errData.error || t('common.error_occurred') || 'Failed to create rule');
      }
      return res.json();
    },
    onSuccess: (result: { runtime_status?: string }) => {
      queryClient.invalidateQueries({ queryKey: ['access_rules'] });
      queryClient.invalidateQueries({ queryKey: ['effective_policy'] });
      result.runtime_status === 'pending'
        ? toast.info(t('server.enforcement_pending_notice'))
        : toast.success(t('common.operation_created'));
      setIsAddOpen(false);
      createRuleMutation.reset();
      setSubjectID('');
      setTargetValue('');
    },
  });

  const filteredRules = rules.filter((r) => {
    if (actionFilter !== 'all' && r.action !== actionFilter) return false;
    if (!searchQuery.trim()) return true;
    const q = searchQuery.toLowerCase();
    return (
      r.subject_name.toLowerCase().includes(q) ||
      (r.target_name && r.target_name.toLowerCase().includes(q)) ||
      r.target_value.toLowerCase().includes(q) ||
      r.action.toLowerCase().includes(q)
    );
  });

  const deleteRuleMutation = useMutation({
    mutationFn: async (id: string) => {
      const res = await fetch(`/api/access-rules/${id}`, {
        method: 'DELETE',
        headers: { Authorization: `Bearer ${localStorage.getItem('token') || ''}` },
      });
      if (!res.ok) throw new Error('Failed to delete rule');
      return res.json();
    },
    onSuccess: (result: { runtime_status?: string }) => {
      queryClient.invalidateQueries({ queryKey: ['access_rules'] });
      queryClient.invalidateQueries({ queryKey: ['effective_policy'] });
      result.runtime_status === 'pending'
        ? toast.info(t('server.enforcement_pending_notice'))
        : toast.success(t('common.operation_deleted'));
    },
  });

  return (
    <div className="space-y-4">
      {rulesError && <QueryErrorBanner onRetry={() => void refetchRules()} />}
      {/* Policy Evaluation Order guidance card */}
      <div className="border subtle-border rounded-xl p-3 surface-card flex items-start gap-2.5 text-xs text-subtle shadow-xs">
        <Info className="w-4 h-4 text-accent shrink-0 mt-0.5" />
        <div>
          <strong className="text-slate-900 dark:text-white">{t('access.guidance_title')}: </strong>
          {t('access.guidance_desc')}
        </div>
      </div>

      <EffectiveAccessPanel devices={devicesQuery.data?.devices || []} />

      {/* Access Rules Table */}
      <div className="space-y-2">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <span className="font-semibold text-xs text-slate-900 dark:text-white">
            {t('access.title')} ({rules.length})
          </span>
          <div className="flex flex-wrap sm:flex-nowrap items-center gap-2 flex-1 w-full min-w-0 sm:w-auto sm:max-w-sm sm:ml-auto">
            <div className="relative w-full">
              <Search className="w-3.5 h-3.5 text-subtle absolute left-2.5 top-1/2 -translate-y-1/2" />
              <input
                type="text"
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                placeholder={t('access.search_placeholder')}
                className="w-full surface-input border subtle-border rounded-lg pl-8 pr-2.5 py-1.5 text-xs focus:outline-none focus:border-blue-500"
              />
            </div>
            <button
              type="button"
              onClick={() => setIsAddOpen(true)}
              className="bg-blue-600 hover:bg-blue-500 text-white font-medium px-3 py-1.5 rounded-lg transition-colors flex items-center gap-1.5 shadow-xs text-xs shrink-0"
            >
              <Plus className="w-3.5 h-3.5" />
              {t('access.add_rule')}
            </button>
          </div>
        </div>

        {/* Action Filter Chips */}
        <div className="flex flex-wrap items-center gap-1.5 text-[11px] px-1">
          <span className="text-subtle font-medium flex items-center gap-1 mr-1">
            <Filter className="w-3 h-3" />
            {t('access.action_filter')}:
          </span>
          {(['all', 'ALLOW', 'DENY'] as const).map((act) => {
            const count = act === 'all' ? rules.length : rules.filter((r) => r.action === act).length;
            const isSelected = actionFilter === act;
            return (
              <button
                key={act}
                type="button"
                onClick={() => setActionFilter(act)}
                className={`px-2 py-0.5 rounded-md border subtle-border transition-colors font-mono text-[10px] ${
                  isSelected
                    ? act === 'DENY'
                      ? 'bg-rose-500/15 border-rose-500/40 text-status-danger font-semibold'
                      : 'bg-emerald-500/15 border-emerald-500/40 text-status-success font-semibold'
                    : 'text-subtle hover:text-current hover:bg-slate-500/10'
                }`}
              >
                {act} {count > 0 && <span className="opacity-75">({count})</span>}
              </button>
            );
          })}

          {(actionFilter !== 'all' || searchQuery) && (
            <button
              type="button"
              onClick={() => {
                setActionFilter('all');
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
            <table className="w-full text-left text-xs min-w-[700px]">
              <thead className="border-b subtle-border surface-header text-subtle font-semibold text-[11px]">
                <tr>
                  <th className="py-2.5 px-3.5">{t('access.subject')}</th>
                  <th className="py-2.5 px-3.5">{t('access.action')}</th>
                  <th className="py-2.5 px-3.5">{t('access.target')}</th>
                  <th className="py-2.5 px-3.5">{t('access.rule_nature')}</th>
                  <th className="py-2.5 px-3.5 text-right">{t('common.actions')}</th>
                </tr>
              </thead>
              <tbody className="divide-y subtle-border">
                {rulesPending || (rulesError && !data) ? (
                  <TableQueryState columns={5} error={rulesError} onRetry={() => void refetchRules()} />
                ) : filteredRules.length === 0 ? (
                  <tr>
                    <td colSpan={5} className="py-8">
                      <EmptyState
                        icon={ShieldCheck}
                        title={searchQuery || actionFilter !== 'all' ? t('access.no_matching_rules') : t('access.no_rules')}
                        description={searchQuery || actionFilter !== 'all' ? t('access.no_matching_rules_description') : t('access.no_rules_description')}
                        actionLabel={searchQuery || actionFilter !== 'all' ? t('common.clear_filters') : t('access.add_rule')}
                        actionIcon={searchQuery || actionFilter !== 'all' ? X : Plus}
                        onAction={searchQuery || actionFilter !== 'all' ? () => { setSearchQuery(''); setActionFilter('all'); } : () => setIsAddOpen(true)}
                        className="border-none shadow-none bg-transparent"
                      />
                    </td>
                  </tr>
                ) : (
                  filteredRules.map((r) => (
                    <tr
                      key={r.id}
                      className={`hover-row transition-colors ${
                        r.action === 'DENY' ? 'bg-rose-500/5' : r.rule_nature === 'direct_grant' ? 'bg-blue-500/5' : ''
                      }`}
                    >
                      <td className="py-2.5 px-3.5 font-medium">
                        {r.action === 'DENY' ? (
                          <span className="text-status-danger flex items-center gap-1.5 font-medium">
                            <ShieldAlert className="w-3.5 h-3.5 text-status-danger shrink-0" />
                            {r.subject_name}
                          </span>
                        ) : (
                          <span className="text-slate-900 dark:text-white flex items-center gap-1.5 font-medium">
                            <ShieldCheck className="w-3.5 h-3.5 text-accent shrink-0" />
                            {r.subject_name}
                          </span>
                        )}
                      </td>
                      <td className="py-2.5 px-3.5">
                        {r.action === 'DENY' ? (
                          <span className="bg-rose-500/10 text-status-danger border border-rose-500/20 font-bold px-2 py-0.5 rounded-md text-[10px] font-mono">
                            DENY
                          </span>
                        ) : (
                          <span className="bg-emerald-500/10 text-status-success border border-emerald-500/20 font-bold px-2 py-0.5 rounded-md text-[10px] font-mono">
                            ALLOW
                          </span>
                        )}
                      </td>
                      <td className="py-2.5 px-3.5 font-mono text-subtle">
                        <div className="flex items-center gap-1.5">
                          <span>{r.target_name || r.target_value}</span>
                          <CopyButton value={r.target_value || r.target_name} label={`Target ${r.target_name || r.target_value}`} />
                        </div>
                      </td>
                      <td className="py-2.5 px-3.5">
                        {r.rule_nature === 'user_exclusion' ? (
                          <span className="text-status-danger text-[10px] border border-rose-500/30 bg-rose-500/10 px-1.5 py-0.5 rounded font-mono font-medium">
                            {t('access.user_exclusion')}
                          </span>
                        ) : r.rule_nature === 'direct_grant' ? (
                          <span className="text-accent text-[10px] border border-blue-500/30 px-1.5 py-0.5 rounded font-mono">
                            {t('access.direct_grant')}
                          </span>
                        ) : (
                          <span className="text-subtle text-[11px] font-mono">{t('access.group_policy')}</span>
                        )}
                      </td>
                      <td className="py-2.5 px-3.5 text-right font-sans">
                        <button
                          type="button"
                          onClick={() => requestConfirmation({
                            title: t('confirm.delete_rule_title'),
                            description: t('confirm.delete_rule_description', { action: r.action, subject: r.subject_name, target: r.target_name || r.target_value }),
                            confirmLabel: t('common.delete'),
                            onConfirm: () => deleteRuleMutation.mutateAsync(r.id),
                          })}
                          className="p-1.5 hover:bg-rose-500/10 rounded-md text-status-danger transition-colors"
                          title={t('common.delete')} data-tooltip={t('common.delete')}
                          aria-label={`${t('common.delete')} ${r.subject_name}`}
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

      {/* MODAL: ADD ACCESS RULE */}
      <Modal
        isOpen={isAddOpen}
        onClose={() => {
          setIsAddOpen(false);
          createRuleMutation.reset();
        }}
        title={t('access.add_rule')}
        error={(createRuleMutation.error as Error)?.message}
      >
        <div className="space-y-3 text-xs">
          <div>
            <label className="text-subtle block mb-1">{t('access.subject_type')}</label>
            <div className="grid grid-cols-3 gap-2">
              <button type="button" aria-pressed={subjectType === 'user_group'}
                onClick={() => { setSubjectType('user_group'); setSubjectID(''); }}
                className={`p-2 border rounded-lg cursor-pointer text-center font-medium transition-colors ${
                  subjectType === 'user_group' ? 'border-blue-500 bg-blue-500/10 text-accent font-semibold' : 'subtle-border surface-header text-subtle hover:text-current hover:bg-slate-500/10'
                }`}
              >
                {t('access.subject_group')}
              </button>
              <button type="button" aria-pressed={subjectType === 'user'}
                onClick={() => { setSubjectType('user'); setSubjectID(''); }}
                className={`p-2 border rounded-lg cursor-pointer text-center font-medium transition-colors ${
                  subjectType === 'user' ? 'border-blue-500 bg-blue-500/10 text-accent font-semibold' : 'subtle-border surface-header text-subtle hover:text-current hover:bg-slate-500/10'
                }`}
              >
                {t('access.subject_user')}
              </button>
              <button type="button" aria-pressed={subjectType === 'device'} onClick={() => { setSubjectType('device'); setSubjectID(''); }}
                className={`p-2 border rounded-lg cursor-pointer text-center font-medium transition-colors ${
                  subjectType === 'device' ? 'border-blue-500 bg-blue-500/10 text-accent font-semibold' : 'subtle-border surface-header text-subtle hover:text-current hover:bg-slate-500/10'
                }`}>
                {t('access.subject_device')}
              </button>
            </div>
          </div>

          <div>
            <label htmlFor="access-rule-subject" className="text-subtle block mb-1">{t(subjectField.label)}</label>
            <select id="access-rule-subject" value={subjectID} onChange={event => setSubjectID(event.target.value)}
              className="w-full surface-input border subtle-border rounded-lg px-2.5 py-1.5 text-xs focus:outline-none focus:border-blue-500">
              <option value="">{t(subjectField.placeholder)}</option>
              {availableSubjects.map(subject => (
                <option key={subject.id} value={subject.id}>{subject.label}</option>
              ))}
            </select>
            {(groupsQuery.isError || usersQuery.isError || devicesQuery.isError) && <p role="alert" className="text-status-danger">
              {t('access.load_choices_error')}
            </p>}
          </div>

          <div>
            <label className="text-subtle block mb-1">{t('access.action')}</label>
            <div className="grid grid-cols-2 gap-2">
              <button type="button" aria-pressed={action === 'ALLOW'}
                onClick={() => setAction('ALLOW')}
                className={`p-2 border rounded-lg cursor-pointer text-center font-bold font-mono transition-colors ${
                  action === 'ALLOW' ? 'border-emerald-500 bg-emerald-500/15 text-status-success font-semibold' : 'subtle-border surface-header text-subtle hover:text-current hover:bg-slate-500/10'
                }`}
              >
                ALLOW
              </button>
              <button type="button" aria-pressed={action === 'DENY'}
                onClick={() => setAction('DENY')}
                className={`p-2 border rounded-lg cursor-pointer text-center font-bold font-mono transition-colors ${
                  action === 'DENY' ? 'border-rose-500 bg-rose-500/15 text-status-danger font-semibold' : 'subtle-border surface-header text-subtle hover:text-current hover:bg-slate-500/10'
                }`}
              >
                DENY
              </button>
            </div>
          </div>

          <div>
            <label className="text-subtle block mb-1">{t('access.destination_type')}</label>
            <select
              value={targetType}
              onChange={(e) => { setTargetType(e.target.value as 'resource_group' | 'cidr_ip'); setTargetValue(''); }}
              className="w-full surface-input border subtle-border rounded-lg px-2.5 py-1.5 text-xs focus:outline-none"
            >
              <option value="resource_group">{t('access.destination_group')}</option>
              <option value="cidr_ip">{t('access.destination_cidr')}</option>
            </select>
          </div>

          <div>
            <label className="text-subtle block mb-1">{t('access.target')}</label>
            {targetType === 'resource_group' ? (
              <select value={targetValue} onChange={event => setTargetValue(event.target.value)}
                className="w-full surface-input border subtle-border rounded-lg px-2.5 py-1.5 text-xs focus:outline-none focus:border-blue-500">
                <option value="">{t('access.select_target_placeholder')}</option>
                {(targetsQuery.data?.resource_groups || []).map(group => (
                  <option key={group.id} value={group.id}>{group.name} ({group.cidr_block})</option>
                ))}
              </select>
            ) : (
              <input type="text" value={targetValue} onChange={event => setTargetValue(event.target.value)}
                placeholder="10.100.1.5/32" className="w-full surface-input border subtle-border rounded-lg px-2.5 py-1.5 font-mono text-xs focus:outline-none focus:border-blue-500" />
            )}
            {targetsQuery.isError && targetType === 'resource_group' && (
              <p role="alert" className="text-status-danger">{t('access.load_choices_error')}</p>
            )}
          </div>
        </div>

        <div className="flex gap-2 pt-2 border-t subtle-border">
          <button
            onClick={() => requestConfirmation({
              title: t('confirm.create_rule_title'),
              description: t('confirm.create_rule_description', { action, subject: selectedSubjectLabel, target: selectedTargetLabel }),
              confirmLabel: t('access.add_rule'),
              tone: action === 'DENY' ? 'danger' : 'warning',
              onConfirm: () => createRuleMutation.mutateAsync(),
            })}
            disabled={!subjectID || !targetValue || createRuleMutation.isPending}
            className="flex-1 bg-blue-600 hover:bg-blue-500 disabled:opacity-50 text-white font-medium py-1.5 rounded-lg shadow-xs transition-colors text-xs"
          >
            {createRuleMutation.isPending ? t('common.saving') : t('common.save')}
          </button>
          <button
            onClick={() => {
              setIsAddOpen(false);
              createRuleMutation.reset();
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
