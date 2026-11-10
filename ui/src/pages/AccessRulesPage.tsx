import React, { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { Info, Plus, Trash2, X, ShieldAlert, ShieldCheck } from 'lucide-react';

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
      if (!res.ok) {
        return {
          access_rules: [
            {
              id: 'rule_1',
              subject_type: 'user',
              subject_id: 'usr_nam',
              subject_name: 'Nam Nguyen',
              action: 'DENY',
              target_type: 'cidr_ip',
              target_value: '10.50.99.0/24',
              target_name: '10.50.99.0/24 (Payment Gate)',
              rule_nature: 'user_exclusion',
            },
            {
              id: 'rule_2',
              subject_type: 'user',
              subject_id: 'usr_thienvu',
              subject_name: 'Thien Vu',
              action: 'ALLOW',
              target_type: 'cidr_ip',
              target_value: '10.100.1.5/32',
              target_name: '10.100.1.5/32 (db-primary-prod)',
              rule_nature: 'direct_grant',
            },
            {
              id: 'rule_3',
              subject_type: 'user_group',
              subject_id: 'ug_devops',
              subject_name: 'DevOps',
              action: 'ALLOW',
              target_type: 'resource_group',
              target_value: 'rg_prod',
              target_name: 'Production VPC, Engineering Subnet',
              rule_nature: 'group_policy',
            },
          ],
        };
      }
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
      if (!res.ok) throw new Error('Failed to create rule');
      return res.json();
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['access_rules'] });
      setIsAddOpen(false);
      setSubjectName('');
      setTargetValue('');
    },
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
          <strong className="text-white">{t('access.guidance_title')}: </strong>
          {t('access.guidance_desc')}
        </div>
      </div>

      {/* Access Rules Table */}
      <div className="border subtle-border rounded-xl overflow-hidden surface-card shadow-xs">
        <div className="p-3 border-b subtle-border flex items-center justify-between">
          <span className="font-semibold text-xs text-white">{t('access.title')} ({rules.length})</span>
          <button
            onClick={() => setIsAddOpen(true)}
            className="bg-blue-600 hover:bg-blue-500 text-white font-medium px-3 py-1.5 rounded-lg transition-colors flex items-center gap-1.5 shadow-xs text-xs"
          >
            <Plus className="w-3.5 h-3.5" />
            {t('access.add_rule')}
          </button>
        </div>

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
              {rules.map((r) => (
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
                      <span className="text-white flex items-center gap-1.5">
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
                  <td className="py-2.5 px-3.5 font-mono text-subtle">{r.target_name || r.target_value}</td>
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
                      onClick={() => deleteRuleMutation.mutate(r.id)}
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

      {/* MODAL: ADD ACCESS RULE */}
      {isAddOpen && (
        <div className="fixed inset-0 bg-black/60 z-50 flex items-center justify-center p-4">
          <div className="surface-card border subtle-border rounded-xl max-w-sm w-full p-5 space-y-4 shadow-2xl">
            <div className="flex items-center justify-between border-b subtle-border pb-2.5">
              <h3 className="font-bold text-sm text-white">{t('access.add_rule')}</h3>
              <button onClick={() => setIsAddOpen(false)} className="text-subtle hover:text-current">
                <X className="w-4 h-4" />
              </button>
            </div>

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
                {t('common.save')}
              </button>
              <button
                onClick={() => setIsAddOpen(false)}
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
