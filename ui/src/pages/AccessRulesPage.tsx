import React, { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { Info, Plus, Trash2, Search, ShieldAlert, ShieldCheck, Filter, X } from 'lucide-react';
import { Modal } from '../components/Modal';
import { CopyButton } from '../components/CopyButton';
import { EmptyState } from '../components/EmptyState';

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

export const AccessRulesPage: React.FC = () => {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const [isAddOpen, setIsAddOpen] = useState(false);
  const [searchQuery, setSearchQuery] = useState('');
  const [actionFilter, setActionFilter] = useState<'all' | 'ALLOW' | 'DENY'>('all');

  // Form states
  const [subjectType, setSubjectType] = useState<'user_group' | 'user'>('user_group');
  const [subjectName, setSubjectName] = useState('');
  const [action, setAction] = useState<'ALLOW' | 'DENY'>('ALLOW');
  const [targetType, setTargetType] = useState<'resource_group' | 'cidr_ip'>('resource_group');
  const [targetValue, setTargetValue] = useState('');

  const { data } = useQuery<{ access_rules: AccessRuleItem[] }>({
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
          subject_id: subjectName,
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
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['access_rules'] });
      setIsAddOpen(false);
      createRuleMutation.reset();
      setSubjectName('');
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
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['access_rules'] });
    },
  });

  return (
    <div className="space-y-4">
      {/* Policy Evaluation Order guidance card */}
      <div className="border subtle-border rounded-xl p-3 surface-card flex items-start gap-2.5 text-xs text-subtle shadow-xs">
        <Info className="w-4 h-4 text-blue-400 shrink-0 mt-0.5" />
        <div>
          <strong className="text-slate-900 dark:text-white">{t('access.guidance_title')}: </strong>
          {t('access.guidance_desc')}
        </div>
      </div>

      {/* Access Rules Table */}
      <div className="space-y-2">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <span className="font-semibold text-xs text-slate-900 dark:text-white">
            {t('access.title')} ({rules.length})
          </span>
          <div className="flex items-center gap-2 flex-1 max-w-sm ml-auto">
            <div className="relative w-full">
              <Search className="w-3.5 h-3.5 text-subtle absolute left-2.5 top-1/2 -translate-y-1/2" />
              <input
                type="text"
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                placeholder="Search rules by subject, target, action..."
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
            Action:
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
                      ? 'bg-rose-500/15 border-rose-500/40 text-rose-500 font-semibold'
                      : 'bg-emerald-500/15 border-emerald-500/40 text-emerald-500 font-semibold'
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
              className="ml-auto text-rose-500 hover:text-rose-400 flex items-center gap-1 text-[11px] font-medium"
            >
              <X className="w-3 h-3" /> Reset filters
            </button>
          )}
        </div>

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
                {filteredRules.length === 0 ? (
                  <tr>
                    <td colSpan={5} className="py-8">
                      <EmptyState
                        icon={ShieldCheck}
                        title={searchQuery || actionFilter !== 'all' ? 'No matching access rules' : 'No access rules configured yet'}
                        description={searchQuery || actionFilter !== 'all' ? `No access rules match the active filter criteria.` : "SwanPanel enforces zero-trust: all non-explicit traffic is blocked by default. Create your first rule to grant access."}
                        actionLabel={searchQuery || actionFilter !== 'all' ? 'Clear Filters' : t('access.add_rule')}
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
                          <span className="text-rose-400 flex items-center gap-1.5">
                            <ShieldAlert className="w-3.5 h-3.5" />
                            {r.subject_name}
                          </span>
                        ) : (
                          <span className="text-slate-900 dark:text-white flex items-center gap-1.5">
                            <ShieldCheck className="w-3.5 h-3.5 text-blue-400" />
                            {r.subject_name}
                          </span>
                        )}
                      </td>
                      <td className="py-2.5 px-3.5">
                        {r.action === 'DENY' ? (
                          <span className="bg-rose-500/10 text-rose-500 border border-rose-500/20 font-bold px-2 py-0.5 rounded-md text-[10px] font-mono">
                            DENY
                          </span>
                        ) : (
                          <span className="bg-emerald-500/10 text-emerald-500 border border-emerald-500/20 font-bold px-2 py-0.5 rounded-md text-[10px] font-mono">
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
                          <span className="text-rose-400 text-[10px] border border-rose-500/30 px-1.5 py-0.5 rounded font-mono">
                            {t('access.user_exclusion')}
                          </span>
                        ) : r.rule_nature === 'direct_grant' ? (
                          <span className="text-blue-400 text-[10px] border border-blue-500/30 px-1.5 py-0.5 rounded font-mono">
                            {t('access.direct_grant')}
                          </span>
                        ) : (
                          <span className="text-subtle text-[11px] font-mono">{t('access.group_policy')}</span>
                        )}
                      </td>
                      <td className="py-2.5 px-3.5 text-right font-sans">
                        <button
                          type="button"
                          onClick={() => deleteRuleMutation.mutate(r.id)}
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
            <label className="text-subtle block mb-1">Subject Type</label>
            <div className="grid grid-cols-2 gap-2">
              <label
                onClick={() => setSubjectType('user_group')}
                className={`p-2 border rounded-lg cursor-pointer text-center font-medium ${
                  subjectType === 'user_group' ? 'border-blue-500 bg-blue-600/10 text-white' : 'subtle-border surface-header text-subtle'
                }`}
              >
                User Group
              </label>
              <label
                onClick={() => setSubjectType('user')}
                className={`p-2 border rounded-lg cursor-pointer text-center font-medium ${
                  subjectType === 'user' ? 'border-blue-500 bg-blue-600/10 text-white' : 'subtle-border surface-header text-subtle'
                }`}
              >
                Specific User
              </label>
            </div>
          </div>

          <div>
            <label className="text-subtle block mb-1">Subject Identifier (Group Name or User Email)</label>
            <input
              type="text"
              value={subjectName}
              onChange={(e) => setSubjectName(e.target.value)}
              placeholder="e.g. DevOps or nam@company.com"
              className="w-full surface-input border subtle-border rounded-lg px-2.5 py-1.5 text-xs focus:outline-none focus:border-blue-500"
            />
          </div>

          <div>
            <label className="text-subtle block mb-1">{t('access.action')}</label>
            <div className="grid grid-cols-2 gap-2">
              <label
                onClick={() => setAction('ALLOW')}
                className={`p-2 border rounded-lg cursor-pointer text-center font-bold font-mono ${
                  action === 'ALLOW' ? 'border-emerald-500 bg-emerald-600/10 text-emerald-400' : 'subtle-border surface-header text-subtle'
                }`}
              >
                ALLOW
              </label>
              <label
                onClick={() => setAction('DENY')}
                className={`p-2 border rounded-lg cursor-pointer text-center font-bold font-mono ${
                  action === 'DENY' ? 'border-rose-500 bg-rose-600/10 text-rose-400' : 'subtle-border surface-header text-subtle'
                }`}
              >
                DENY
              </label>
            </div>
          </div>

          <div>
            <label className="text-subtle block mb-1">Destination Target Type</label>
            <select
              value={targetType}
              onChange={(e) => setTargetType(e.target.value as any)}
              className="w-full surface-input border subtle-border rounded-lg px-2.5 py-1.5 text-xs focus:outline-none"
            >
              <option value="resource_group">Resource Group / Subnet</option>
              <option value="cidr_ip">Specific IP or CIDR (e.g. 10.50.99.0/24)</option>
            </select>
          </div>

          <div>
            <label className="text-subtle block mb-1">{t('access.target')}</label>
            <input
              type="text"
              value={targetValue}
              onChange={(e) => setTargetValue(e.target.value)}
              placeholder="e.g. Production VPC or 10.100.1.5/32"
              className="w-full surface-input border subtle-border rounded-lg px-2.5 py-1.5 font-mono text-xs focus:outline-none focus:border-blue-500"
            />
          </div>
        </div>

        <div className="flex gap-2 pt-2 border-t subtle-border">
          <button
            onClick={() => createRuleMutation.mutate()}
            disabled={!subjectName || !targetValue || createRuleMutation.isPending}
            className="flex-1 bg-blue-600 hover:bg-blue-500 disabled:opacity-50 text-white font-medium py-1.5 rounded-lg shadow-xs transition-colors text-xs"
          >
            {createRuleMutation.isPending ? 'Saving...' : t('common.save')}
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
    </div>
  );
};
