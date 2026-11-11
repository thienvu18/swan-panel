import React, { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { Check, X, Shield, Trash2, Plus, Users as UsersIcon, Search } from 'lucide-react';
import { Modal } from '../components/Modal';
import { CopyButton } from '../components/CopyButton';
import { EmptyState } from '../components/EmptyState';

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
  const queryClient = useQueryClient();
  const [activeSubView, setActiveSubView] = useState<'directory' | 'usergroups'>('directory');
  const [isAddUserOpen, setIsAddUserOpen] = useState(false);
  const [isAddGroupOpen, setIsAddGroupOpen] = useState(false);
  const [searchQuery, setSearchQuery] = useState('');

  // Form states for Add User
  const [userName, setUserName] = useState('');
  const [userEmail, setUserEmail] = useState('');
  const [userPassword, setUserPassword] = useState('');
  const [userIsAdmin, setUserIsAdmin] = useState(false);

  // Form states for Add Group
  const [grpName, setGrpName] = useState('');
  const [grpDesc, setGrpDesc] = useState('');

  // 1. Query Users
  const { data: usersData } = useQuery<{ users: UserItem[] }>({
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
  const { data: groupsData } = useQuery<{ user_groups: UserGroupItem[] }>({
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
    mutationFn: async (id: string) => {
      const res = await fetch(`/api/users/${id}/approve`, {
        method: 'POST',
        headers: { Authorization: `Bearer ${localStorage.getItem('token') || ''}` },
      });
      if (!res.ok) throw new Error('Failed to approve');
      return res.json();
    },
    onSuccess: () => {
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
      queryClient.invalidateQueries({ queryKey: ['users'] });
      setIsAddUserOpen(false);
      createUserMutation.reset();
      setUserName('');
      setUserEmail('');
      setUserPassword('');
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
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['user_groups'] });
    },
  });

  return (
    <div className="space-y-4">
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

        <div className="flex items-center gap-2 flex-1 max-w-sm ml-auto">
          <div className="relative w-full">
            <Search className="w-3.5 h-3.5 text-subtle absolute left-2.5 top-1/2 -translate-y-1/2" />
            <input
              type="text"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              placeholder={activeSubView === 'directory' ? "Search users by name, email..." : "Search user groups..."}
              className="w-full surface-input border subtle-border rounded-lg pl-8 pr-2.5 py-1.5 text-xs focus:outline-none focus:border-blue-500"
            />
          </div>
          <button
            onClick={() => setIsAddUserOpen(true)}
            className="bg-blue-600 hover:bg-blue-500 text-white font-medium px-3 py-1.5 rounded-lg transition-colors flex items-center gap-1.5 shadow-xs text-xs shrink-0"
          >
            <Plus className="w-3.5 h-3.5" />
            {t('users.add_user')}
          </button>
          <button
            onClick={() => setIsAddGroupOpen(true)}
            className="border subtle-border hover:bg-slate-500/10 text-subtle hover:text-current font-medium px-2.5 py-1.5 rounded-lg transition-colors flex items-center gap-1 text-xs shrink-0"
          >
            <UsersIcon className="w-3.5 h-3.5" />
            {t('users.add_group')}
          </button>
        </div>
      </div>

      {activeSubView === 'directory' && (
        <div className="space-y-4">
          {/* OIDC Approval Queue */}
          {pendingUsers.length > 0 && (
            <div className="space-y-2">
              <span className="font-semibold text-xs text-amber-500 flex items-center gap-1.5">
                <span className="w-1.5 h-1.5 rounded-full bg-amber-500"></span>
                {t('users.approval_queue')} ({pendingUsers.length})
              </span>
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
                        <td className="py-2.5 px-3.5 text-subtle">{u.idp_provider?.name || 'OIDC Provider'}</td>
                        <td className="py-2.5 px-3.5">
                          <div className="flex items-center gap-3 text-xs">
                            <label className="flex items-center gap-1.5 cursor-pointer">
                              <input type="checkbox" defaultChecked className="rounded border-slate-600" /> Backend
                            </label>
                            <label className="flex items-center gap-1.5 cursor-pointer">
                              <input type="checkbox" className="rounded border-slate-600" /> DevOps
                            </label>
                          </div>
                        </td>
                        <td className="py-2.5 px-3.5 text-right space-x-1">
                          <button
                            onClick={() => rejectMutation.mutate(u.id)}
                            className="p-1.5 hover:bg-rose-500/10 rounded-md text-rose-500 transition-colors inline-block"
                            title={t('users.reject')}
                          >
                            <X className="w-4 h-4" />
                          </button>
                          <button
                            onClick={() => approveMutation.mutate(u.id)}
                            className="p-1.5 hover:bg-emerald-500/10 rounded-md text-emerald-500 transition-colors inline-block"
                            title={t('users.approve')}
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
                    {activeUsers.length === 0 ? (
                      <tr>
                        <td colSpan={5} className="py-8">
                          <EmptyState
                            icon={UsersIcon}
                            title={searchQuery ? 'No matching users' : 'No active users found'}
                            description={searchQuery ? `No users match '${searchQuery}'.` : "Invite team members or enable Single Sign-On to onboard users."}
                            actionLabel={searchQuery ? 'Clear Search' : t('users.add_user')}
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
                          <div className="flex items-center gap-1.5">
                            <span>{u.name}</span>
                            <span className="text-dim font-normal font-mono">({u.email})</span>
                            <CopyButton value={u.email} label={`email ${u.email}`} />
                          </div>
                        </td>
                        <td className="py-2.5 px-3.5">
                          {u.auth_type === 'local' ? (
                            <span className="border border-zinc-700 bg-zinc-800 text-zinc-300 px-1.5 py-0.5 rounded text-[10px] font-mono">
                              {t('users.local_auth')}
                            </span>
                          ) : (
                            <span className="text-subtle">{u.idp_provider?.name || 'OIDC'}</span>
                          )}
                        </td>
                        <td className="py-2.5 px-3.5">
                          {u.is_admin ? (
                            <span className="bg-blue-500/10 text-blue-500 border border-blue-500/20 px-2 py-0.5 rounded-md text-[10px] font-semibold">
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
                            : 'None'}
                        </td>
                        <td className="py-2.5 px-3.5 text-right space-x-1">
                          <button
                            type="button"
                            onClick={() => toggleRoleMutation.mutate(u.id)}
                            className={`p-1.5 rounded-md transition-colors inline-block ${
                              u.is_admin ? 'text-blue-500 hover:bg-blue-500/10' : 'text-subtle hover:text-blue-500 hover:bg-slate-500/10'
                            }`}
                            title={t('users.grant_admin')}
                          >
                            <Shield className="w-3.5 h-3.5" />
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
        <div className="border subtle-border rounded-xl overflow-hidden surface-card shadow-xs">
          <table className="w-full text-left text-xs">
            <thead className="border-b subtle-border surface-header text-subtle font-semibold text-[11px]">
              <tr>
                <th className="py-2.5 px-3.5">{t('common.name')}</th>
                <th className="py-2.5 px-3.5">{t('common.description')}</th>
                <th className="py-2.5 px-3.5">{t('users.active_members')}</th>
                <th className="py-2.5 px-3.5 text-right">{t('common.actions')}</th>
              </tr>
            </thead>
            <tbody className="divide-y subtle-border">
              {filteredGroups.length === 0 ? (
                <tr>
                  <td colSpan={4} className="py-8 text-center text-subtle font-sans text-xs">
                    {searchQuery
                      ? `No user groups match '${searchQuery}'`
                      : "No user groups configured yet. Click '+ Add Group' to create a team."}
                  </td>
                </tr>
              ) : (
                filteredGroups.map((g) => (
                  <tr key={g.id} className="hover-row">
                    <td className="py-2.5 px-3.5 font-medium text-blue-400 flex items-center gap-2">
                      <UsersIcon className="w-3.5 h-3.5 text-blue-400" />
                      {g.name}
                    </td>
                    <td className="py-2.5 px-3.5 text-subtle">{g.description || '-'}</td>
                    <td className="py-2.5 px-3.5 text-dim font-mono">{g.member_count || 0} members</td>
                    <td className="py-2.5 px-3.5 text-right">
                      <button
                        onClick={() => {
                          if (confirm(`Delete user group '${g.name}'?`)) {
                            deleteGroupMutation.mutate(g.id);
                          }
                        }}
                        className="p-1.5 hover:bg-rose-500/10 rounded-md text-subtle hover:text-rose-500 transition-colors"
                        title="Delete Group"
                        aria-label="Delete Group"
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
      )}

      {/* MODAL: CREATE LOCAL USER */}
      <Modal
        isOpen={isAddUserOpen}
        onClose={() => {
          setIsAddUserOpen(false);
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
              placeholder="e.g. Alex Contractor"
              className="w-full surface-input border subtle-border rounded-lg px-2.5 py-1.5 text-xs focus:outline-none focus:border-blue-500"
            />
          </div>

          <div>
            <label className="text-subtle block mb-1">{t('common.email')}</label>
            <input
              type="email"
              value={userEmail}
              onChange={(e) => setUserEmail(e.target.value)}
              placeholder="e.g. alex@partner.io"
              className="w-full surface-input border subtle-border rounded-lg px-2.5 py-1.5 font-mono text-xs focus:outline-none focus:border-blue-500"
            />
          </div>

          <div>
            <label className="text-subtle block mb-1">{t('common.password')}</label>
            <input
              type="password"
              value={userPassword}
              onChange={(e) => setUserPassword(e.target.value)}
              placeholder="Strong password"
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
              Grant System Administrator privilege
            </label>
          </div>
        </div>

        <div className="flex gap-2 pt-2 border-t subtle-border">
          <button
            onClick={() => createUserMutation.mutate()}
            disabled={!userName || !userEmail || !userPassword || createUserMutation.isPending}
            className="flex-1 bg-blue-600 hover:bg-blue-500 disabled:opacity-50 text-white font-medium py-1.5 rounded-lg shadow-xs transition-colors text-xs"
          >
            {createUserMutation.isPending ? 'Saving...' : t('common.save')}
          </button>
          <button
            onClick={() => {
              setIsAddUserOpen(false);
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
            <label className="text-subtle block mb-1">Group Name</label>
            <input
              type="text"
              value={grpName}
              onChange={(e) => setGrpName(e.target.value)}
              placeholder="e.g. Security Auditors"
              className="w-full surface-input border subtle-border rounded-lg px-2.5 py-1.5 text-xs focus:outline-none focus:border-blue-500"
            />
          </div>

          <div>
            <label className="text-subtle block mb-1">Description</label>
            <input
              type="text"
              value={grpDesc}
              onChange={(e) => setGrpDesc(e.target.value)}
              placeholder="e.g. External auditors with read-only access"
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
            {createGroupMutation.isPending ? 'Saving...' : t('common.save')}
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
    </div>
  );
};
