import React, { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { Laptop, Power, ShieldAlert, Download, Activity } from 'lucide-react';
import { CopyButton } from '../components/CopyButton';
import { EmptyState } from '../components/EmptyState';

interface SessionInfo {
  ike_name: string;
  peer_identity: string;
  virtual_ip: string;
  remote_wan: string;
  uptime: string;
  cipher_suite: string;
  established_seconds: number;
}

interface DashboardStats {
  active_sessions: number;
  total_devices: number;
  total_users: number;
  pending_users: number;
  gateway_status: string;
  uptime_seconds?: number;
}

export const DashboardPage: React.FC = () => {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const [role, setRole] = useState<'admin' | 'user'>('admin');

  // 1. Fetch dashboard statistics
  const { data: statsData, isLoading: isStatsLoading } = useQuery<{ stats: DashboardStats }>({
    queryKey: ['dashboard_stats'],
    queryFn: async () => {
      const res = await fetch('/api/dashboard/stats', {
        headers: { Authorization: `Bearer ${localStorage.getItem('token') || ''}` },
      });
      if (!res.ok) throw new Error('Failed to fetch dashboard stats');
      return res.json();
    },
  });

  const stats = statsData?.stats || {
    active_sessions: 0,
    total_devices: 0,
    total_users: 0,
    pending_users: 0,
    gateway_status: 'online',
  };

  // 2. Fetch live active VPN sessions
  const { data: sessionsData, isLoading: isSessionsLoading } = useQuery<{ sessions: SessionInfo[] }>({
    queryKey: ['sessions'],
    queryFn: async () => {
      const res = await fetch('/api/sessions', {
        headers: { Authorization: `Bearer ${localStorage.getItem('token') || ''}` },
      });
      if (!res.ok) throw new Error('Failed to fetch active sessions');
      return res.json();
    },
  });

  const liveSessions = sessionsData?.sessions || [];

  // 3. Fetch user enrolled devices (when in user view)
  const { data: userDevicesData } = useQuery<{ devices: any[] }>({
    queryKey: ['devices'],
    queryFn: async () => {
      const res = await fetch('/api/devices', {
        headers: { Authorization: `Bearer ${localStorage.getItem('token') || ''}` },
      });
      if (!res.ok) throw new Error('Failed to fetch devices');
      return res.json();
    },
    enabled: role === 'user',
  });

  const userDevices = userDevicesData?.devices || [];

  // Disconnect session mutation
  const disconnectMutation = useMutation({
    mutationFn: async (ikeName: string) => {
      const res = await fetch('/api/sessions/terminate', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${localStorage.getItem('token') || ''}`,
        },
        body: JSON.stringify({ ike_name: ikeName }),
      });
      if (!res.ok) throw new Error('Failed to terminate session');
      return res.json();
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['sessions'] });
      queryClient.invalidateQueries({ queryKey: ['dashboard_stats'] });
    },
  });

  // Revoke device mutation
  const revokeDeviceMutation = useMutation({
    mutationFn: async (deviceId: string) => {
      const res = await fetch(`/api/devices/${deviceId}/revoke`, {
        method: 'POST',
        headers: { Authorization: `Bearer ${localStorage.getItem('token') || ''}` },
      });
      if (!res.ok) throw new Error('Failed to revoke device');
      return res.json();
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['devices'] });
      queryClient.invalidateQueries({ queryKey: ['dashboard_stats'] });
    },
  });

  const handleDisconnect = (ikeName: string) => {
    if (confirm(`Disconnect live tunnel '${ikeName}'?`)) {
      disconnectMutation.mutate(ikeName);
    }
  };

  const handleDownloadProfile = (deviceId: string, platform: string) => {
    window.open(
      `/api/devices/${deviceId}/config/${platform}?token=${localStorage.getItem('token') || ''}`,
      '_blank'
    );
  };

  return (
    <div className="space-y-5">
      {/* Role Toggle Switcher */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2 border subtle-border rounded-lg px-2.5 py-1 surface-header text-xs">
          <span className="text-dim">{t('dashboard.view_mode')}:</span>
          <button
            onClick={() => setRole('admin')}
            className={`font-semibold transition-colors ${
              role === 'admin' ? 'text-blue-500 underline' : 'text-subtle hover:text-current'
            }`}
          >
            {t('dashboard.admin_view')}
          </button>
          <span className="text-dim">/</span>
          <button
            onClick={() => setRole('user')}
            className={`font-semibold transition-colors ${
              role === 'user' ? 'text-blue-500 underline' : 'text-subtle hover:text-current'
            }`}
          >
            {t('dashboard.user_view')}
          </button>
        </div>
      </div>

      {/* STAT CARDS (Admin View) */}
      {role === 'admin' && (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-3.5">
          <div className="border subtle-border rounded-xl p-4 surface-card flex flex-col justify-between shadow-xs">
            <span className="text-subtle font-medium text-xs">{t('dashboard.active_sessions')}</span>
            <div className="mt-2 flex items-baseline gap-2">
              <span className="text-2xl font-bold font-mono text-emerald-500">
                {isStatsLoading ? '...' : stats.active_sessions}
              </span>
              <span className="text-[10px] text-emerald-500 font-mono">Live IPC</span>
            </div>
          </div>

          <div className="border subtle-border rounded-xl p-4 surface-card flex flex-col justify-between shadow-xs">
            <span className="text-subtle font-medium text-xs">{t('dashboard.total_devices')}</span>
            <div className="mt-2 flex items-baseline gap-2">
              <span className="text-2xl font-bold font-mono text-slate-900 dark:text-white">
                {isStatsLoading ? '...' : stats.total_devices}
              </span>
              <span className="text-[10px] text-dim">{t('dashboard.native_profiles')}</span>
            </div>
          </div>

          <div className="border subtle-border rounded-xl p-4 surface-card flex flex-col justify-between shadow-xs">
            <span className="text-subtle font-medium text-xs">{t('dashboard.total_users')}</span>
            <div className="mt-2 flex items-baseline gap-2">
              <span className="text-2xl font-bold font-mono text-slate-900 dark:text-white">
                {isStatsLoading ? '...' : stats.total_users}
              </span>
              <span className="text-[10px] text-blue-400 font-mono">SSO + Local</span>
            </div>
          </div>

          <div className="border subtle-border rounded-xl p-4 surface-card flex flex-col justify-between shadow-xs">
            <span className="text-subtle font-medium text-xs">{t('dashboard.pending_approvals')}</span>
            <div className="mt-2 flex items-baseline gap-2">
              <span className="text-2xl font-bold font-mono text-amber-500">
                {isStatsLoading ? '...' : stats.pending_users}
              </span>
              <span className="text-[10px] text-amber-500 font-mono">{t('dashboard.requires_review')}</span>
            </div>
          </div>
        </div>
      )}

      {/* ADMIN VIEW: LIVE SESSIONS TABLE */}
      {role === 'admin' ? (
        <div className="space-y-2">
          <div className="flex items-center justify-between">
            <span className="font-semibold text-xs tracking-tight">
              {t('dashboard.live_sessions')}
            </span>
            <span className="text-[11px] text-dim font-mono">
              {t('dashboard.auto_refresh_hint')}
            </span>
          </div>

          <div className="border subtle-border rounded-xl overflow-hidden surface-card shadow-xs">
            <div className="overflow-x-auto">
              <table className="w-full text-left font-mono text-xs min-w-[650px]">
                <thead className="border-b subtle-border surface-header text-subtle font-sans font-semibold text-[11px]">
                  <tr>
                    <th className="py-2.5 px-3.5">{t('dashboard.connected_node')}</th>
                    <th className="py-2.5 px-3.5">{t('dashboard.virtual_ip')}</th>
                    <th className="py-2.5 px-3.5">{t('dashboard.wan_endpoint')}</th>
                    <th className="py-2.5 px-3.5">{t('dashboard.duration')}</th>
                    <th className="py-2.5 px-3.5">{t('dashboard.cipher')}</th>
                    <th className="py-2.5 px-3.5 text-right font-sans">{t('common.actions')}</th>
                  </tr>
                </thead>
                <tbody className="divide-y subtle-border">
                  {isSessionsLoading ? (
                    <tr>
                      <td colSpan={6} className="py-8 text-center text-subtle font-sans text-xs">
                        {t('common.loading')}
                      </td>
                    </tr>
                  ) : liveSessions.length === 0 ? (
                    <tr>
                      <td colSpan={6} className="py-8">
                        <EmptyState
                          icon={Activity}
                          title="No active VPN sessions"
                          description="No client nodes are currently connected to the gateway. As soon as a client establishes an IKEv2 tunnel, it will appear here in real time."
                          className="border-none shadow-none bg-transparent"
                        />
                      </td>
                    </tr>
                  ) : (
                    liveSessions.map((sess) => (
                      <tr key={sess.ike_name} className="hover-row">
                        <td className="py-2.5 px-3.5 font-sans font-medium text-slate-900 dark:text-white flex items-center gap-2">
                          <Laptop className="w-3.5 h-3.5 text-subtle" />
                          <span>{sess.peer_identity || sess.ike_name}</span>
                        </td>
                        <td className="py-2.5 px-3.5 text-blue-400">
                          <div className="flex items-center gap-1.5">
                            <span>{sess.virtual_ip || 'N/A'}</span>
                            {sess.virtual_ip && (
                              <CopyButton value={sess.virtual_ip} label={`Virtual IP ${sess.virtual_ip}`} />
                            )}
                          </div>
                        </td>
                        <td className="py-2.5 px-3.5 font-sans text-subtle">{sess.remote_wan || 'N/A'}</td>
                        <td className="py-2.5 px-3.5 font-sans text-subtle">{sess.uptime || 'Just now'}</td>
                        <td className="py-2.5 px-3.5">
                          <span className="bg-blue-500/10 text-blue-400 border border-blue-500/20 px-2 py-0.5 rounded-md text-[10px] font-mono">
                            {sess.cipher_suite || 'AES-GCM'}
                          </span>
                        </td>
                        <td className="py-2.5 px-3.5 text-right font-sans">
                          <button
                            type="button"
                            onClick={() => handleDisconnect(sess.ike_name)}
                            disabled={disconnectMutation.isPending}
                            className="p-1.5 hover:bg-rose-500/10 rounded-md text-rose-500 transition-colors disabled:opacity-50"
                            title={t('dashboard.disconnect')}
                            aria-label={t('dashboard.disconnect')}
                          >
                            <Power className="w-3.5 h-3.5" />
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
      ) : (
        /* USER VIEW: REGISTERED DEVICES */
        <div className="space-y-2">
          <div className="flex items-center justify-between">
            <span className="font-semibold text-xs tracking-tight">{t('dashboard.my_devices')}</span>
          </div>

          <div className="border subtle-border rounded-xl overflow-hidden surface-card shadow-xs">
            <div className="overflow-x-auto">
              <table className="w-full text-left font-mono text-xs min-w-[650px]">
                <thead className="border-b subtle-border surface-header text-subtle font-sans font-semibold text-[11px]">
                  <tr>
                    <th className="py-2.5 px-3.5">{t('resources.device_hostname')}</th>
                    <th className="py-2.5 px-3.5">{t('resources.virtual_ip')}</th>
                    <th className="py-2.5 px-3.5">{t('resources.platform')}</th>
                    <th className="py-2.5 px-3.5">{t('common.status')}</th>
                    <th className="py-2.5 px-3.5 text-right font-sans">{t('common.actions')}</th>
                  </tr>
                </thead>
                <tbody className="divide-y subtle-border">
                  {userDevices.length === 0 ? (
                    <tr>
                      <td colSpan={5} className="py-8 text-center text-subtle font-sans text-xs">
                        {t('dashboard.no_user_devices')}
                      </td>
                    </tr>
                  ) : (
                    userDevices.map((dev: any) => (
                      <tr key={dev.id} className="hover-row">
                        <td className="py-2.5 px-3.5 font-sans font-medium text-slate-900 dark:text-white flex items-center gap-2">
                          <Laptop className="w-3.5 h-3.5 text-subtle" />
                          <span>{dev.hostname}</span>
                        </td>
                        <td className="py-2.5 px-3.5 text-blue-400">{dev.virtual_ip || 'Dynamic Pool'}</td>
                        <td className="py-2.5 px-3.5 font-sans text-subtle uppercase text-[10px]">
                          {dev.platform}
                        </td>
                        <td className="py-2.5 px-3.5">
                          {dev.is_revoked ? (
                            <span className="bg-rose-500/10 text-rose-400 border border-rose-500/20 px-2 py-0.5 rounded-md text-[10px] font-sans">
                              Revoked
                            </span>
                          ) : (
                            <span className="bg-emerald-500/10 text-emerald-400 border border-emerald-500/20 px-2 py-0.5 rounded-md text-[10px] font-sans">
                              Active
                            </span>
                          )}
                        </td>
                        <td className="py-2.5 px-3.5 text-right font-sans flex items-center justify-end gap-1.5">
                          {!dev.is_revoked && (
                            <>
                              <button
                                onClick={() => handleDownloadProfile(dev.id, dev.platform || 'macos')}
                                className="p-1.5 hover:bg-blue-500/10 rounded-md text-blue-400 transition-colors"
                                title={t('dashboard.download_profile')}
                                aria-label={t('dashboard.download_profile')}
                              >
                                <Download className="w-3.5 h-3.5" />
                              </button>
                              <button
                                onClick={() => {
                                  if (confirm(`Revoke device '${dev.hostname}'? This will permanently invalidate its certificate.`)) {
                                    revokeDeviceMutation.mutate(dev.id);
                                  }
                                }}
                                className="p-1.5 hover:bg-rose-500/10 rounded-md text-rose-400 transition-colors"
                                title={t('dashboard.self_revoke')}
                                aria-label={t('dashboard.self_revoke')}
                              >
                                <ShieldAlert className="w-3.5 h-3.5" />
                              </button>
                            </>
                          )}
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
    </div>
  );
};
