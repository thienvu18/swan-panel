import { useToast } from '../components/ToastProvider';
import React, { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { Laptop, Power, ShieldAlert, Download, Activity } from 'lucide-react';
import { CopyButton } from '../components/CopyButton';
import { AndroidNativeSetupDialog } from '../components/AndroidNativeSetupDialog';
import { EmptyState } from '../components/EmptyState';
import { useConfirmAction } from '../components/ConfirmDialog';
import { downloadDeviceProfile } from '../lib/profileDownload';
import { formatDuration } from '../lib/telemetry';
import type { Device } from '../types';

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
  active_sessions: number | null;
  total_devices: number;
  total_users: number;
  pending_users: number;
  gateway_status: string;
  uptime_seconds?: number;
}

export const DashboardPage: React.FC<{ isAdmin: boolean; currentUserID: string }> = ({ isAdmin, currentUserID }) => {
  const { t } = useTranslation();
  const toast = useToast();
  const queryClient = useQueryClient();
  const { requestConfirmation, confirmationDialog } = useConfirmAction();
  const [role, setRole] = useState<'admin' | 'user'>(isAdmin ? 'admin' : 'user');
  const [androidDevice, setAndroidDevice] = useState<{ id: string; hostname: string } | null>(null);
  useEffect(() => { if (!isAdmin) setRole('user'); }, [isAdmin]);

  // 1. Fetch dashboard statistics
  const { data: statsData, isLoading: isStatsLoading, isError: isStatsError } = useQuery<{ stats: DashboardStats }>({
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
    active_sessions: null,
    total_devices: 0,
    total_users: 0,
    pending_users: 0,
    gateway_status: 'unknown',
  };

  // 2. Fetch live active VPN sessions
  const { data: sessionsData, isLoading: isSessionsLoading, isError: isSessionsError } = useQuery<{ sessions: SessionInfo[] }>({
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
  const { data: userDevicesData } = useQuery<{ devices: Device[] }>({
    queryKey: ['devices', 'mine', currentUserID],
    queryFn: async () => {
      // Admins may switch the presentation, but must not see every device as "My Personal Devices".
      // The backend already supports admin-only user_id filtering; regular users are owner-scoped there.
      const path = isAdmin
        ? `/api/devices?user_id=${encodeURIComponent(currentUserID)}`
        : '/api/devices';
      const res = await fetch(path, {
        headers: { Authorization: `Bearer ${localStorage.getItem('token') || ''}` },
      });
      if (!res.ok) throw new Error('Failed to fetch personal devices');
      return res.json();
    },
    enabled: role === 'user' && (!isAdmin || Boolean(currentUserID)),
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
      toast.success(t('common.operation_saved'));
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

  const handleDisconnect = (ikeName: string) => requestConfirmation({
    title: t('confirm.disconnect_title'),
    description: t('confirm.disconnect_description', { name: ikeName }),
    confirmLabel: t('dashboard.disconnect'),
    onConfirm: () => disconnectMutation.mutateAsync(ikeName),
  });

  const handleDownloadProfile = async (deviceId: string, platform: string, hostname: string) => {
    try {
      await downloadDeviceProfile(deviceId, platform, hostname);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t('resources.profile_download_failed'));
    }
  };

  return (
    <div className="space-y-5">
      {confirmationDialog}
      <AndroidNativeSetupDialog isOpen={androidDevice !== null} onClose={() => setAndroidDevice(null)}
        deviceId={androidDevice?.id || ''} hostname={androidDevice?.hostname || ''} />
      {isStatsError && <div role="alert" className="border border-rose-500/40 rounded-lg p-3 text-xs text-status-danger">
        {t('dashboard.metrics_unavailable')}
      </div>}
      {/* Role Toggle Switcher */}
      {isAdmin && <div className="flex items-center justify-between">
        <div className="flex items-center gap-2 border subtle-border rounded-lg px-2.5 py-1 surface-header text-xs">
          <span className="text-dim">{t('dashboard.view_mode')}:</span>
          <button
            onClick={() => setRole('admin')}
            className={`font-semibold transition-colors ${
              role === 'admin' ? 'text-accent underline' : 'text-subtle hover:text-current'
            }`}
          >
            {t('dashboard.admin_view')}
          </button>
          <span className="text-dim">/</span>
          <button
            onClick={() => setRole('user')}
            className={`font-semibold transition-colors ${
              role === 'user' ? 'text-accent underline' : 'text-subtle hover:text-current'
            }`}
          >
            {t('dashboard.user_view')}
          </button>
        </div>
      </div>}

      {/* STAT CARDS (Admin View) */}
      {role === 'admin' && (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-3.5">
          <div className="border subtle-border rounded-xl p-4 surface-card flex flex-col justify-between shadow-xs">
            <span className="text-subtle font-medium text-xs">{t('dashboard.active_sessions')}</span>
            <div className="mt-2 flex items-baseline gap-2">
              <span className={`text-2xl font-bold font-mono ${stats.active_sessions === null || isStatsError ? 'text-subtle' : 'text-status-success'}`}>
                {isStatsLoading ? '…' : isStatsError || stats.active_sessions === null ? '—' : stats.active_sessions}
              </span>
              {!isStatsLoading && <span className={`text-[10px] font-mono ${stats.active_sessions === null || isStatsError ? 'text-subtle' : 'text-status-success'}`}>
                {isStatsError || stats.active_sessions === null ? t('common.unavailable') : t('dashboard.live_sessions_caption')}
              </span>}
            </div>
          </div>

          <div className="border subtle-border rounded-xl p-4 surface-card flex flex-col justify-between shadow-xs">
            <span className="text-subtle font-medium text-xs">{t('dashboard.total_devices')}</span>
            <div className="mt-2 flex items-baseline gap-2">
              <span className="text-2xl font-bold font-mono text-slate-900 dark:text-white">
                {isStatsLoading ? '…' : isStatsError ? '—' : stats.total_devices}
              </span>
              <span className="text-[10px] text-dim">{t('dashboard.native_profiles')}</span>
            </div>
          </div>

          <div className="border subtle-border rounded-xl p-4 surface-card flex flex-col justify-between shadow-xs">
            <span className="text-subtle font-medium text-xs">{t('dashboard.total_users')}</span>
            <div className="mt-2 flex items-baseline gap-2">
              <span className="text-2xl font-bold font-mono text-slate-900 dark:text-white">
                {isStatsLoading ? '…' : isStatsError ? '—' : stats.total_users}
              </span>
              <span className="text-[10px] text-accent font-mono">{t('dashboard.sso_local_caption')}</span>
            </div>
          </div>

          <div className="border subtle-border rounded-xl p-4 surface-card flex flex-col justify-between shadow-xs">
            <span className="text-subtle font-medium text-xs">{t('dashboard.pending_approvals')}</span>
            <div className="mt-2 flex items-baseline gap-2">
              <span className="text-2xl font-bold font-mono text-status-warning">
                {isStatsLoading ? '…' : isStatsError ? '—' : stats.pending_users}
              </span>
              <span className="text-[10px] text-status-warning font-mono">{t('dashboard.requires_review')}</span>
            </div>
          </div>
        </div>
      )}

      {/* ADMIN VIEW: LIVE SESSIONS TABLE */}
      {role === 'admin' ? (
        <div className="space-y-2">
          <div className="flex flex-wrap items-center justify-between gap-1.5">
            <span className="font-semibold text-xs tracking-tight">
              {t('dashboard.live_sessions')}
            </span>
            <span className="text-[11px] text-dim font-mono">
              {t('dashboard.auto_refresh_hint')}
            </span>
          </div>
          <p id="dashboard-session-scroll-hint" className="lg:hidden text-[10px] text-subtle">
            {t('dashboard.scroll_table_hint')}
          </p>

          <div className="border subtle-border rounded-xl overflow-hidden surface-card shadow-xs">
            <div className="overflow-x-auto focus-visible:outline focus-visible:outline-2 focus-visible:outline-blue-500 focus-visible:outline-offset-[-2px]" role="region" aria-label={t('dashboard.live_sessions')} aria-describedby="dashboard-session-scroll-hint" tabIndex={0}>
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
                  ) : isSessionsError ? (
                    <tr><td colSpan={6} role="alert" className="py-8 px-4 text-center text-status-danger font-sans">
                      {t('dashboard.sessions_unavailable')}
                    </td></tr>
                  ) : liveSessions.length === 0 ? (
                    <tr>
                      <td colSpan={6} className="py-8">
                        <EmptyState
                          icon={Activity}
                          title={t('dashboard.no_active_sessions')}
                          description={t('dashboard.sessions_empty_description')}
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
                        <td className="py-2.5 px-3.5 text-accent">
                          <div className="flex items-center gap-1.5">
                            <span>{sess.virtual_ip || t('common.unavailable')}</span>
                            {sess.virtual_ip && (
                              <CopyButton value={sess.virtual_ip} label={`Virtual IP ${sess.virtual_ip}`} />
                            )}
                          </div>
                        </td>
                        <td className="py-2.5 px-3.5 font-sans text-subtle">{sess.remote_wan || t('common.unavailable')}</td>
                        <td className="py-2.5 px-3.5 font-sans text-subtle" title={sess.uptime || undefined}>{formatDuration(sess.established_seconds || sess.uptime, t('common.unavailable'))}</td>
                        <td className="py-2.5 px-3.5">
                          <span className="bg-blue-500/10 text-accent border border-blue-500/20 px-2 py-0.5 rounded-md text-[10px] font-mono">
                            {sess.cipher_suite || t('common.unavailable')}
                          </span>
                        </td>
                        <td className="py-2.5 px-3.5 text-right font-sans">
                          <button
                            type="button"
                            onClick={() => handleDisconnect(sess.ike_name)}
                            disabled={disconnectMutation.isPending}
                            className="p-1.5 hover:bg-rose-500/10 rounded-md text-status-danger transition-colors disabled:opacity-50 focus-visible:ring-2 focus-visible:ring-rose-500 focus-visible:outline-none"
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
                    userDevices.map((dev: Device) => (
                      <tr key={dev.id} className="hover-row">
                        <td className="py-2.5 px-3.5 font-sans font-medium text-slate-900 dark:text-white flex items-center gap-2">
                          <Laptop className="w-3.5 h-3.5 text-subtle" />
                          <span>{dev.hostname}</span>
                        </td>
                        <td className="py-2.5 px-3.5 text-accent">{dev.virtual_ip || t('resources.dynamic_pool')}</td>
                        <td className="py-2.5 px-3.5 font-sans text-subtle uppercase text-[10px]">
                          {dev.platform}
                        </td>
                        <td className="py-2.5 px-3.5">
                          {dev.is_revoked ? (
                            <span className="bg-rose-500/10 text-status-danger border border-rose-500/20 px-2 py-0.5 rounded-md text-[10px] font-sans">
                              {t('common.revoked')}
                            </span>
                          ) : (
                            <span className="bg-emerald-500/10 text-status-success border border-emerald-500/20 px-2 py-0.5 rounded-md text-[10px] font-sans">
                              {t('common.active')}
                            </span>
                          )}
                        </td>
                        <td className="py-2.5 px-3.5 text-right font-sans flex items-center justify-end gap-1.5">
                          {!dev.is_revoked && (
                            <>
                              {dev.platform === 'android' ? (
                                <>
                                  <button
                                    onClick={() => setAndroidDevice({ id: dev.id, hostname: dev.hostname })}
                                    className="p-1.5 hover:bg-emerald-500/10 rounded-md text-status-success transition-colors"
                                    title={t('resources.download_android_native')}
                                    aria-label={t('resources.download_android_native')}
                                  >
                                    <Download className="w-3.5 h-3.5" />
                                  </button>
                                </>
                              ) : (
                                <button
                                  onClick={() => handleDownloadProfile(dev.id, dev.platform || 'macos', dev.hostname)}
                                  className="p-1.5 hover:bg-blue-500/10 rounded-md text-accent transition-colors focus-visible:ring-2 focus-visible:ring-blue-500 focus-visible:outline-none"
                                  title={t('dashboard.download_profile')}
                                  aria-label={t('dashboard.download_profile')}
                                >
                                  <Download className="w-3.5 h-3.5" />
                                </button>
                              )}
                              <button
                                onClick={() => requestConfirmation({
                                  title: t('confirm.revoke_title'),
                                  description: t('confirm.revoke_description', { name: dev.hostname }),
                                  confirmLabel: t('common.revoke'),
                                  onConfirm: () => revokeDeviceMutation.mutateAsync(dev.id),
                                })}
                                className="p-1.5 hover:bg-rose-500/10 rounded-md text-status-danger transition-colors focus-visible:ring-2 focus-visible:ring-rose-500 focus-visible:outline-none"
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
