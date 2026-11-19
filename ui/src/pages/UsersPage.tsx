import { useToast } from '../components/ToastProvider';
import React, { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { Check, X, Shield, Trash2, Plus, Users as UsersIcon, Search } from 'lucide-react';
import { Modal } from '../components/Modal';
import { useConfirmAction } from '../components/ConfirmDialog';
import { CopyButton } from '../components/CopyButton';
import { EmptyState } from '../components/EmptyState';
import { TableQueryState } from '../components/TableQueryState';
import { TableActionLegend } from '../components/TableActionLegend';

interface UserItem {
  id: string;
  email: string;
  name: string;
  auth_type: string;
  is_admin: boolean;
  status: 'PENDING' | 'ACTIVE' | 'SUSPENDED';
  user_groups?: { id: string; name: string }[];
  idp_provider?: { name: string };
}

interface UserGroupItem {
  id: string;
  name: string;
  description: string;
  member_count: number;
}

export const UsersPage: React.FC = () => {
  const { t } = useTranslation();
  const toast = useToast();
  const queryClient = useQueryClient();
  const [activeSubView, setActiveSubView] = useState<'directory' | 'usergroups'>('directory');
  const [isAddUserOpen, setIsAddUserOpen] = useState(false);
  const [isAddGroupOpen, setIsAddGroupOpen] = useState(false);
  const [searchQuery, setSearchQuery] = useState('');
  const [pendingMemberships, setPendingMemberships] = useState<Record<string, string[]>>({});
  const { requestConfirmation, confirmationDialog } = useConfirmAction();

  // Form states for Add User
  const [userName, setUserName] = useState('');
  const [userEmail, setUserEmail] = useState('');
  const [userPassword, setUserPassword] = useState('');
  const [userIsAdmin, setUserIsAdmin] = useState(false);

  // Form states for Add Group
  const [grpName, setGrpName] = useState('');
  const [grpDesc, setGrpDesc] = useState('');

  // 1. Query Users
  const { data: usersData, isPending: usersPending, isError: usersError, refetch: refetchUsers } = useQuery<{ users: UserItem[] }>({
    queryKey: ['users'],
    queryFn: async () => {
      const res = await fetch('/api/users', {
        headers: { Authorization: `Bearer ${localStorage.getItem('token') || ''}` },
      });
      if (!res.ok) throw new Error('Failed to fetch users');
      return res.json();
    },
  });

  // 2. Query Groups
  const { data: groupsData, isPending: groupsPending, isError: groupsError, refetch: refetchGroups } = useQuery<{ user_groups: UserGroupItem[] }>({
    queryKey: ['user_groups'],
    queryFn: async () => {
      const res = await fetch('/api/user-groups', {
        headers: { Authorization: `Bearer ${localStorage.getItem('token') || ''}` },
      });
      if (!res.ok) throw new Error('Failed to fetch user groups');
      return res.json();
    },
  });

  const users = usersData?.users || [];
  const userGroups = groupsData?.user_groups || [];

  const filteredUsers = users.filter((u) => {
    if (!searchQuery.trim()) return true;
    const q = searchQuery.toLowerCase();
    return (
      u.name.toLowerCase().includes(q) ||
      u.email.toLowerCase().includes(q) ||
      u.status.toLowerCase().includes(q) ||
      (u.user_groups && u.user_groups.some((g) => g.name.toLowerCase().includes(q)))
    );
  });

  const filteredGroups = userGroups.filter((g) => {
    if (!searchQuery.trim()) return true;
    const q = searchQuery.toLowerCase();
    return g.name.toLowerCase().includes(q) || g.description.toLowerCase().includes(q);
  });

  const pendingUsers = filteredUsers.filter((u) => u.status === 'PENDING');
  const activeUsers = filteredUsers.filter((u) => u.status === 'ACTIVE');

  // Approve Mutation
  const approveMutation = useMutation({
    mutationFn: async ({ id, groupIDs }: { id: string; groupIDs: string[] }) => {
      const res = await fetch(`/api/users/${id}/approve`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${localStorage.getItem('token') || ''}`,
        },
        body: JSON.stringify({ user_group_ids: groupIDs }),
      });
      if (!res.ok) throw new Error('Failed to approve');
      return res.json();
    },
    onSuccess: (result: { runtime_status?: string }) => {
      result.runtime_status === 'pending'
        ? toast.info(t('server.enforcement_pending_notice'))
        : toast.success(t('common.operation_saved'));
      queryClient.invalidateQueries({ queryKey: ['users'] });
      queryClient.invalidateQueries({ queryKey: ['dashboard_stats'] });
    },
  });

  // Reject Mutation
  const rejectMutation = useMutation({
    mutationFn: async (id: string) => {
      const res = await fetch(`/api/users/${id}/reject`, {
        method: 'POST',
        headers: { Authorization: `Bearer ${localStorage.getItem('token') || ''}` },
      });
      if (!res.ok) throw new Error('Failed to reject');
      return res.json();
    },
    onSuccess: () => {
      toast.success(t('common.operation_saved'));
      queryClient.invalidateQueries({ queryKey: ['users'] });
    },
  });

  // Toggle Admin Role Mutation
  const toggleRoleMutation = useMutation({
    mutationFn: async (id: string) => {
      const res = await fetch(`/api/users/${id}/role`, {
        method: 'PUT',
        headers: { Authorization: `Bearer ${localStorage.getItem('token') || ''}` },
      });
      if (!res.ok) throw new Error('Failed to update role');
      return res.json();
    },
    onSuccess: () => {
      toast.success(t('common.operation_saved'));
      queryClient.invalidateQueries({ queryKey: ['users'] });
    },
  });

  // Create Local User Mutation (US-03B)
  const createUserMutation = useMutation({
    mutationFn: async () => {
      const res = await fetch('/api/users', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${localStorage.getItem('token') || ''}`,
        },
        body: JSON.stringify({
          name: userName,
          email: userEmail,
          password: userPassword,
          is_admin: userIsAdmin,
        }),
      });
      if (!res.ok) {
        const errData = await res.json().catch(() => ({}));
        throw new Error(errData.error || t('common.error_occurred') || 'Failed to create user');
      }
      return res.json();
    },
    onSuccess: () => {
      toast.success(t('common.operation_created'));
      queryClient.invalidateQueries({ queryKey: ['users'] });
      setIsAddUserOpen(false);
      createUserMutation.reset();
      setUserName('');
      setUserEmail('');
      setUserPassword('');
      setUserIsAdmin(false);
    },
  });

  // Create Group Mutation
  const createGroupMutation = useMutation({
    mutationFn: async () => {
      const res = await fetch('/api/user-groups', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${localStorage.getItem('token') || ''}`,
        },
        body: JSON.stringify({ name: grpName, description: grpDesc }),
      });
      if (!res.ok) {
        const errData = await res.json().catch(() => ({}));
        throw new Error(errData.error || t('common.error_occurred') || 'Failed to create user group');
      }
      return res.json();
    },
    onSuccess: () => {
      toast.success(t('common.operation_created'));
      queryClient.invalidateQueries({ queryKey: ['user_groups'] });
      setIsAddGroupOpen(false);
      createGroupMutation.reset();
      setGrpName('');
      setGrpDesc('');
    },
  });

  // Delete Group Mutation
  const deleteGroupMutation = useMutation({
    mutationFn: async (id: string) => {
      const res = await fetch(`/api/user-groups/${id}`, {
        method: 'DELETE',
        headers: { Authorization: `Bearer ${localStorage.getItem('token') || ''}` },
      });
      if (!res.ok) throw new Error('Failed to delete user group');
      return res.json();
    },
    onSuccess: (result: { runtime_status?: string }) => {
      result.runtime_status === 'pending'
        ? toast.info(t('server.enforcement_pending_notice'))
        : toast.success(t('common.operation_deleted'));
      queryClient.invalidateQueries({ queryKey: ['user_groups'] });
    },
  });

  const deleteUserMutation = useMutation({
    mutationFn: async (id: string) => {
      const res = await fetch('/api/users/' + encodeURIComponent(id), {
        method: 'DELETE',
        headers: { Authorization: 'Bearer ' + (localStorage.getItem('token') || '') },
      });
      if (!res.ok) {
        const body = await res.json().catch(() => ({}));
        throw new Error(body.error || t('common.error_occurred'));
      }
      return res.json();
    },
    onSuccess: (result: { runtime_status?: string }) => {
      result.runtime_status === 'pending'
        ? toast.info(t('server.enforcement_pending_notice'))
        : toast.success(t('common.operation_deleted'));
      queryClient.invalidateQueries({ queryKey: ['users'] });
      queryClient.invalidateQueries({ queryKey: ['dashboard_stats'] });
      queryClient.invalidateQueries({ queryKey: ['devices'] });
    },
  });

  return (
    <div className="space-y-4">
      {(usersError || groupsError) && (
        <div role="alert" className="border border-rose-500/30 rounded-lg bg-rose-500/5 text-status-danger p-3 text-xs flex items-center justify-between gap-3">
          <span>{t('common.load_failed')}</span>
          <button type="button" onClick={() => { void refetchUsers(); void refetchGroups(); }} className="underline font-medium">
            {t('common.retry')}
          </button>
        </div>
      )}
      {/* Subnav & Action */}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex border subtle-border rounded-lg p-0.5 surface-header text-xs">
          <button
            onClick={() => setActiveSubView('directory')}
            className={`px-3 py-1.5 rounded-md font-medium transition-colors ${
              activeSubView === 'directory' ? 'bg-blue-600 text-white shadow-xs' : 'text-subtle hover:text-current'
            }`}
          >
            {t('users.directory_approvals')} ({users.length})
          </button>
          <button
            onClick={() => setActiveSubView('usergroups')}
            className={`px-3 py-1.5 rounded-md font-medium transition-colors ${
              activeSubView === 'usergroups' ? 'bg-blue-600 text-white shadow-xs' : 'text-subtle hover:text-current'
            }`}
          >
            {t('users.user_groups')} ({userGroups.length})
          </button>
        </div>

        <div className="flex flex-wrap sm:flex-nowrap items-center gap-2 flex-1 min-w-0 sm:max-w-lg sm:ml-auto">
          <div className="relative w-full">
            <Search className="w-3.5 h-3.5 text-subtle absolute left-2.5 top-1/2 -translate-y-1/2" />
            <input
              type="text"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              placeholder={activeSubView === 'directory' ? t('users.search_placeholder') : t('users.group_search_placeholder')}
              className="w-full surface-input border subtle-border rounded-lg pl-8 pr-2.5 py-1.5 text-xs focus:outline-none focus:border-blue-500"
            />
          </div>
          <button onClick={() => activeSubView === 'directory' ? setIsAddUserOpen(true) : setIsAddGroupOpen(true)}
            className="bg-blue-600 hover:bg-blue-500 text-white font-medium px-3 py-2 rounded-lg transition-colors flex items-center gap-1.5 shadow-xs text-xs shrink-0">
            <Plus className="w-4 h-4" />
            {activeSubView === 'directory' ? t('users.add_user') : t('users.add_group')}
          </button>
        </div>
      </div>

      {activeSubView === 'directory' && (
        <div className="space-y-4">
          {/* OIDC Approval Queue */}
          {pendingUsers.length > 0 && (
            <div className="space-y-2">
              <span className="font-semibold text-xs text-status-warning flex items-center gap-1.5">
                <span className="w-1.5 h-1.5 rounded-full bg-amber-500"></span>
                {t('users.approval_queue')} ({pendingUsers.length})
              </span>
              <TableActionLegend actions={[{ icon: X, label: t('users.reject') }, { icon: Check, label: t('users.approve') }]} />
              <div className="border border-amber-500/30 rounded-xl overflow-hidden surface-card shadow-xs">
                <table className="w-full text-left text-xs">
                  <thead className="border-b subtle-border surface-header text-subtle text-[11px]">
                    <tr>
                      <th className="py-2.5 px-3.5">{t('users.user_candidate')}</th>
                      <th className="py-2.5 px-3.5">{t('users.idp_source')}</th>
                      <th className="py-2.5 px-3.5">{t('users.assign_groups')}</th>
                      <th className="py-2.5 px-3.5 text-right">{t('users.decision')}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {pendingUsers.map((u) => (
                      <tr key={u.id} className="hover-row">
                        <td className="py-2.5 px-3.5">
                          <div className="font-medium text-slate-900 dark:text-white">{u.name}</div>
                          <div className="text-[11px] text-dim font-mono">{u.email}</div>
                        </td>
                        <td className="py-2.5 px-3.5 text-subtle">{u.idp_provider?.name || t('users.oidc_provider_fallback')}</td>
                        <td className="py-2.5 px-3.5">
                          <div className="flex flex-wrap items-center gap-3 text-xs">
                            {userGroups.length === 0 ? (
                              <span className="text-dim">{t('users.no_groups_available')}</span>
                            ) : userGroups.map(group => (
                              <label key={group.id} className="flex items-center gap-1.5 cursor-pointer">
                                <input type="checkbox" className="rounded border-slate-600"
                                  checked={(pendingMemberships[u.id] ?? u.user_groups?.map(g => g.id) ?? []).includes(group.id)}
                                  onChange={event => setPendingMemberships(previous => {
                                    const current = previous[u.id] ?? u.user_groups?.map(g => g.id) ?? [];
                                    return {
                                      ...previous,
                                      [u.id]: event.target.checked
                                        ? [...current.filter(id => id !== group.id), group.id]
                                        : current.filter(id => id !== group.id),
                                    };
                                  })} />
                                {group.name}
                              </label>
                            ))}
                          </div>
                        </td>
                        <td className="py-2.5 px-3.5 text-right space-x-1">
                          <button
                            onClick={() => requestConfirmation({
                              title: t('confirm.reject_user_title'),
                              description: t('confirm.reject_user_description', { name: u.name, email: u.email }),
                              confirmLabel: t('common.reject'),
                              onConfirm: () => rejectMutation.mutateAsync(u.id),
                            })}
                            className="p-1.5 hover:bg-rose-500/10 rounded-md text-status-danger transition-colors inline-block"
                            title={t('users.reject')} data-tooltip={t('users.reject')} aria-label={`${t('users.reject')} ${u.name}`}
                          >
                            <X className="w-4 h-4" />
                          </button>
                          <button
                            onClick={() => requestConfirmation({
                              title: t('confirm.approve_user_title'),
                              description: t('confirm.approve_user_description', { name: u.name, email: u.email }),
                              confirmLabel: t('common.approve'),
                              tone: 'warning',
                              onConfirm: () => approveMutation.mutateAsync({
                                id: u.id,
                                groupIDs: pendingMemberships[u.id] ?? u.user_groups?.map(group => group.id) ?? [],
                              }),
                            })}
                            className="p-1.5 hover:bg-emerald-500/10 rounded-md text-status-success transition-colors inline-block"
                            title={t('users.approve')} data-tooltip={t('users.approve')} aria-label={`${t('users.approve')} ${u.name}`}
                          >
                            <Check className="w-4 h-4" />
                          </button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          )}

          {/* Active Users Directory */}
          <div className="space-y-2">
            <span className="font-semibold text-xs text-subtle">
              {t('users.enrolled_users')} ({activeUsers.length})
            </span>
            <TableActionLegend actions={[{ icon: Shield, label: t('users.grant_admin') }, { icon: Trash2, label: t('common.delete') }]} />
            <div className="border subtle-border rounded-xl overflow-hidden surface-card shadow-xs">
              <div className="overflow-x-auto">
                <table className="w-full text-left text-xs min-w-[650px]">
                  <thead className="border-b subtle-border surface-header text-subtle font-semibold text-[11px]">
                    <tr>
                      <th className="py-2.5 px-3.5">{t('users.name_email')}</th>
                      <th className="py-2.5 px-3.5">{t('users.idp_provider')}</th>
                      <th className="py-2.5 px-3.5">{t('users.system_role')}</th>
                      <th className="py-2.5 px-3.5">{t('users.user_groups')}</th>
                      <th className="py-2.5 px-3.5 text-right">{t('common.actions')}</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y subtle-border">
                    {usersPending || (usersError && !usersData) ? (
                      <TableQueryState columns={5} error={usersError} onRetry={() => void refetchUsers()} />
                    ) : activeUsers.length === 0 ? (
                      <tr>
                        <td colSpan={5} className="py-8">
                          <EmptyState
                            icon={UsersIcon}
                            title={searchQuery ? t('users.no_matching_users') : t('users.no_active_users')}
                            description={searchQuery ? t('users.no_matching_users_description', { query: searchQuery }) : t('users.no_active_users_description')}
                            actionLabel={searchQuery ? t('common.clear_search') : t('users.add_user')}
                            actionIcon={searchQuery ? X : Plus}
                            onAction={searchQuery ? () => setSearchQuery('') : () => setIsAddUserOpen(true)}
                            className="border-none shadow-none bg-transparent"
                          />
                        </td>
                      </tr>
                    ) : (
                      activeUsers.map((u) => (
                      <tr key={u.id} className="hover-row transition-colors">
                        <td className="py-2.5 px-3.5 font-medium text-slate-900 dark:text-white">
                          <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
                            <span>{u.name}</span>
                            <span className="text-dim font-normal font-mono">({u.email})</span>
                            <CopyButton value={u.email} label={`email ${u.email}`} />
                          </div>
                        </td>
                        <td className="py-2.5 px-3.5">
                          {u.auth_type === 'local' ? (
                            <span className="border subtle-border bg-slate-100 text-slate-700 dark:bg-slate-800 dark:text-slate-200 px-1.5 py-0.5 rounded text-[10px] font-mono">
                              {t('users.local_auth')}
                            </span>
                          ) : (
                            <span className="text-subtle">{u.idp_provider?.name || 'OIDC'}</span>
                          )}
                        </td>
                        <td className="py-2.5 px-3.5">
                          {u.is_admin ? (
                            <span className="bg-blue-500/10 text-accent border border-blue-500/20 px-2 py-0.5 rounded-md text-[10px] font-semibold">
                              {t('users.admin')}
                            </span>
                          ) : (
                            <span className="text-subtle border subtle-border px-2 py-0.5 rounded-md text-[10px]">
                              {t('users.member')}
                            </span>
                          )}
                        </td>
                        <td className="py-2.5 px-3.5 text-subtle">
                          {u.user_groups && u.user_groups.length > 0
                            ? u.user_groups.map((g) => g.name).join(', ')
                            : t('common.none')}
                        </td>
                        <td className="py-2.5 px-3.5 text-right space-x-1">
                          <button
                            type="button"
                            onClick={() => requestConfirmation({
                              title: t(u.is_admin ? 'confirm.remove_admin_title' : 'confirm.grant_admin_title'),
                              description: t(u.is_admin ? 'confirm.remove_admin_description' : 'confirm.grant_admin_description', { name: u.name, email: u.email }),
                              confirmLabel: t(u.is_admin ? 'common.remove_admin' : 'common.grant_admin'),
                              tone: u.is_admin ? 'warning' : 'danger',
                              onConfirm: () => toggleRoleMutation.mutateAsync(u.id),
                            })}
                            className={`p-1.5 rounded-md transition-colors inline-block ${
                              u.is_admin ? 'text-accent hover:bg-blue-500/10' : 'text-subtle hover:text-blue-700 dark:hover:text-blue-400 hover:bg-slate-500/10'
                            }`}
                            title={t(u.is_admin ? 'common.remove_admin' : 'common.grant_admin')}
                            data-tooltip={t(u.is_admin ? 'common.remove_admin' : 'common.grant_admin')}
                            aria-label={t(u.is_admin ? 'common.remove_admin' : 'common.grant_admin') + ' ' + u.name}
                          >
                            <Shield className="w-3.5 h-3.5" />
                          </button>
                          <button type="button"
                            onClick={() => requestConfirmation({
                              title: t('confirm.delete_user_title'),
                              description: t('confirm.delete_user_description', { name: u.name, email: u.email }),
                              confirmLabel: t('common.delete'),
                              onConfirm: () => deleteUserMutation.mutateAsync(u.id),
                            })}
                            className="p-1.5 rounded-md text-status-danger hover:bg-rose-500/10"
                            title={t('common.delete')} data-tooltip={t('common.delete')}
                            aria-label={t('common.delete') + ' ' + u.name}
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
        </div>
      )}

      {/* VIEW B: USER GROUPS */}
      {activeSubView === 'usergroups' && (
        <div>
          <TableActionLegend actions={[{ icon: Trash2, label: t('common.delete') }]} />
          <div className="border subtle-border rounded-xl overflow-hidden surface-card shadow-xs">
          <div className="overflow-x-auto" role="region" tabIndex={0} aria-label={t('users.user_groups')}>
          <table className="w-full min-w-[520px] text-left text-xs">
            <thead className="border-b subtle-border surface-header text-subtle font-semibold text-[11px]">
              <tr>
                <th className="py-2.5 px-3.5">{t('common.name')}</th>
                <th className="py-2.5 px-3.5">{t('common.description')}</th>
                <th className="py-2.5 px-3.5">{t('users.active_members')}</th>
                <th className="py-2.5 px-3.5 text-right">{t('common.actions')}</th>
              </tr>
            </thead>
            <tbody className="divide-y subtle-border">
              {groupsPending || (groupsError && !groupsData) ? (
                <TableQueryState columns={4} error={groupsError} onRetry={() => void refetchGroups()} />
              ) : filteredGroups.length === 0 ? (
                <tr>
                  <td colSpan={4} className="py-8">
                    <EmptyState icon={UsersIcon}
                      title={searchQuery ? t('users.no_matching_groups', { query: searchQuery }) : t('users.no_groups_created')}
                      description={t(searchQuery ? 'users.no_matching_groups_description' : 'users.no_groups_created_description')}
                      actionLabel={searchQuery ? t('common.clear_search') : t('users.add_group')}
                      actionIcon={searchQuery ? X : Plus}
                      onAction={searchQuery ? () => setSearchQuery('') : () => setIsAddGroupOpen(true)}
                      className="border-none shadow-none bg-transparent" />
                  </td>
                </tr>
              ) : (
                filteredGroups.map((g) => (
                  <tr key={g.id} className="hover-row">
                    <td className="py-2.5 px-3.5 font-medium text-accent">
                      <div className="flex items-center gap-2">
                        <UsersIcon className="w-3.5 h-3.5 text-accent" />
                        {g.name}
                      </div>
                    </td>
                    <td className="py-2.5 px-3.5 text-subtle">{g.description || '-'}</td>
                    <td className="py-2.5 px-3.5 text-dim font-mono">{t('users.member_count', { count: g.member_count ?? 0 })}</td>
                    <td className="py-2.5 px-3.5 text-right">
                      <button
                        onClick={() => requestConfirmation({
                          title: t('confirm.delete_group_title'),
                          description: t('confirm.delete_group_description', { name: g.name }),
                          confirmLabel: t('common.delete'),
                          onConfirm: () => deleteGroupMutation.mutateAsync(g.id),
                        })}
                        className="p-1.5 hover:bg-rose-500/10 rounded-md text-subtle hover:text-rose-700 dark:hover:text-rose-400 transition-colors"
                        title={t('confirm.delete_group_title')} data-tooltip={t('confirm.delete_group_title')}
                        aria-label={t('confirm.delete_group_title')}
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
      )}

      {/* MODAL: CREATE LOCAL USER */}
      <Modal
        isOpen={isAddUserOpen}
        onClose={() => {
          setIsAddUserOpen(false);
          setUserIsAdmin(false);
          createUserMutation.reset();
        }}
        title={t('users.create_user_title')}
        error={(createUserMutation.error as Error)?.message}
      >
        <div className="space-y-3 text-xs">
          <div>
            <label className="text-subtle block mb-1">{t('common.name')}</label>
            <input
              type="text"
              value={userName}
              onChange={(e) => setUserName(e.target.value)}
              placeholder={t('users.name_placeholder')}
              className="w-full surface-input border subtle-border rounded-lg px-2.5 py-1.5 text-xs focus:outline-none focus:border-blue-500"
            />
          </div>

          <div>
            <label className="text-subtle block mb-1">{t('common.email')}</label>
            <input
              type="email"
              value={userEmail}
              onChange={(e) => setUserEmail(e.target.value)}
              placeholder={t('users.email_placeholder')}
              className="w-full surface-input border subtle-border rounded-lg px-2.5 py-1.5 font-mono text-xs focus:outline-none focus:border-blue-500"
            />
          </div>

          <div>
            <label className="text-subtle block mb-1">{t('common.password')}</label>
            <input
              type="password"
              value={userPassword}
              onChange={(e) => setUserPassword(e.target.value)}
              placeholder={t('users.password_placeholder')}
              className="w-full surface-input border subtle-border rounded-lg px-2.5 py-1.5 font-mono text-xs focus:outline-none focus:border-blue-500"
            />
          </div>

          <div className="flex items-center gap-2 pt-1">
            <input
              type="checkbox"
              id="chk-admin"
              checked={userIsAdmin}
              onChange={(e) => setUserIsAdmin(e.target.checked)}
              className="rounded border-slate-600"
            />
            <label htmlFor="chk-admin" className="text-subtle cursor-pointer">
              {t('users.admin_privilege_label')}
            </label>
          </div>
        </div>

        <div className="flex gap-2 pt-2 border-t subtle-border">
          <button
            onClick={() => {
              if (!userIsAdmin) {
                createUserMutation.mutate();
                return;
              }
              requestConfirmation({
                title: t('confirm.create_admin_title'),
                description: t('confirm.create_admin_description', { name: userName, email: userEmail }),
                confirmLabel: t('common.grant_admin'),
                onConfirm: () => createUserMutation.mutateAsync(),
              });
            }}
            disabled={!userName || !userEmail || !userPassword || createUserMutation.isPending}
            className="flex-1 bg-blue-600 hover:bg-blue-500 disabled:opacity-50 text-white font-medium py-1.5 rounded-lg shadow-xs transition-colors text-xs"
          >
            {createUserMutation.isPending ? t('common.saving') : t('common.save')}
          </button>
          <button
            onClick={() => {
              setIsAddUserOpen(false);
              setUserIsAdmin(false);
              createUserMutation.reset();
            }}
            className="px-3 py-1.5 border subtle-border rounded-lg hover:bg-slate-500/10 text-xs"
          >
            {t('common.cancel')}
          </button>
        </div>
      </Modal>

      {/* MODAL: CREATE USER GROUP */}
      <Modal
        isOpen={isAddGroupOpen}
        onClose={() => {
          setIsAddGroupOpen(false);
          createGroupMutation.reset();
        }}
        title={t('users.add_group')}
        error={(createGroupMutation.error as Error)?.message}
      >
        <div className="space-y-3 text-xs">
          <div>
            <label className="text-subtle block mb-1">{t('users.group_name_label')}</label>
            <input
              type="text"
              value={grpName}
              onChange={(e) => setGrpName(e.target.value)}
              placeholder={t('users.group_name_placeholder')}
              className="w-full surface-input border subtle-border rounded-lg px-2.5 py-1.5 text-xs focus:outline-none focus:border-blue-500"
            />
          </div>

          <div>
            <label className="text-subtle block mb-1">{t('common.description')}</label>
            <input
              type="text"
              value={grpDesc}
              onChange={(e) => setGrpDesc(e.target.value)}
              placeholder={t('users.group_description_placeholder')}
              className="w-full surface-input border subtle-border rounded-lg px-2.5 py-1.5 text-xs focus:outline-none focus:border-blue-500"
            />
          </div>
        </div>

        <div className="flex gap-2 pt-2 border-t subtle-border">
          <button
            onClick={() => createGroupMutation.mutate()}
            disabled={!grpName || createGroupMutation.isPending}
            className="flex-1 bg-blue-600 hover:bg-blue-500 disabled:opacity-50 text-white font-medium py-1.5 rounded-lg shadow-xs transition-colors text-xs"
          >
            {createGroupMutation.isPending ? t('common.saving') : t('common.save')}
          </button>
          <button
            onClick={() => {
              setIsAddGroupOpen(false);
              createGroupMutation.reset();
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
