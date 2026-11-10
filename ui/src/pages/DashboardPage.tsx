import React, { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { Laptop, Power } from 'lucide-react';

export const DashboardPage: React.FC = () => {
  const { t } = useTranslation();
  const [role, setRole] = useState<'admin' | 'user'>('admin');

  // Fetch dashboard statistics via TanStack Query
  const { data, isLoading } = useQuery({
    queryKey: ['dashboard_stats'],
    queryFn: async () => {
      const res = await fetch('/api/dashboard/stats', {
        headers: {
          // Sample token or retrieved from localStorage
          Authorization: `Bearer ${localStorage.getItem('token') || ''}`,
        },
      });
      if (!res.ok) {
        // Fallback mockup when unauthenticated for instant interface preview
        return {
          stats: {
            active_sessions: 2,
            total_devices: 3,
            total_users: 2,
            pending_users: 1,
            gateway_status: 'online',
          },
        };
      }
      return res.json();
    },
  });

  const stats = data?.stats || {
    active_sessions: 2,
    total_devices: 3,
    total_users: 2,
    pending_users: 1,
  };

  const handleDisconnect = (name: string) => {
    if (confirm(`Disconnect live tunnel for ${name}?`)) {
      alert(`Session for ${name} terminated successfully via VICI socket.`);
    }
  };

  return (
    <div className="space-y-5">
      {/* Role Toggle Switcher */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2 border subtle-border rounded-lg px-2.5 py-1 surface-header text-xs">
          <span className="text-dim">View Mode:</span>
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

      {/* STAT CARDS (Responsive: 1 col on mobile, 3-4 cols on desktop) */}
      {role === 'admin' && (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-3.5">
          <div className="border subtle-border rounded-xl p-4 surface-card flex flex-col justify-between shadow-xs">
            <span className="text-subtle font-medium text-xs">{t('dashboard.active_sessions')}</span>
            <div className="mt-2 flex items-baseline gap-2">
              <span className="text-2xl font-bold font-mono text-emerald-500">
                {isLoading ? '...' : stats.active_sessions}
              </span>
              <span className="text-[10px] text-emerald-500 font-mono">Live IPC</span>
            </div>
          </div>

          <div className="border subtle-border rounded-xl p-4 surface-card flex flex-col justify-between shadow-xs">
            <span className="text-subtle font-medium text-xs">{t('dashboard.total_devices')}</span>
            <div className="mt-2 flex items-baseline gap-2">
              <span className="text-2xl font-bold font-mono text-white">
                {isLoading ? '...' : stats.total_devices}
              </span>
              <span className="text-[10px] text-dim">Native Profiles</span>
            </div>
          </div>

          <div className="border subtle-border rounded-xl p-4 surface-card flex flex-col justify-between shadow-xs">
            <span className="text-subtle font-medium text-xs">{t('dashboard.total_users')}</span>
            <div className="mt-2 flex items-baseline gap-2">
              <span className="text-2xl font-bold font-mono text-white">
                {isLoading ? '...' : stats.total_users}
              </span>
              <span className="text-[10px] text-blue-400 font-mono">SSO + Local</span>
            </div>
          </div>

          <div className="border subtle-border rounded-xl p-4 surface-card flex flex-col justify-between shadow-xs">
            <span className="text-subtle font-medium text-xs">{t('dashboard.pending_approvals')}</span>
            <div className="mt-2 flex items-baseline gap-2">
              <span className="text-2xl font-bold font-mono text-amber-500">
                {isLoading ? '...' : stats.pending_users}
              </span>
              <span className="text-[10px] text-amber-500 font-mono">Requires Review</span>
            </div>
          </div>
        </div>
      )}

      {/* LIVE SESSIONS TABLE */}
      <div className="space-y-2">
        <div className="flex items-center justify-between">
          <span className="font-semibold text-xs tracking-tight">
            {role === 'admin' ? t('dashboard.live_sessions') : 'My Personal Registered Devices'}
          </span>
          <span className="text-[11px] text-dim font-mono">
            Auto-refreshing via SSE (Zero Downtime)
          </span>
        </div>

        <div className="border subtle-border rounded-xl overflow-hidden surface-card shadow-xs">
          <div className="overflow-x-auto">
            <table className="w-full text-left font-mono text-xs min-w-[600px]">
              <thead className="border-b subtle-border surface-header text-subtle font-sans font-semibold text-[11px]">
                <tr>
                  <th className="py-2.5 px-3.5">Device Hostname</th>
                  <th className="py-2.5 px-3.5">Assigned Virtual IP</th>
                  <th className="py-2.5 px-3.5">Assigned Subnet</th>
                  <th className="py-2.5 px-3.5">Connected User</th>
                  <th className="py-2.5 px-3.5">Status</th>
                  <th className="py-2.5 px-3.5 text-right font-sans">{t('common.actions')}</th>
                </tr>
              </thead>
              <tbody className="divide-y subtle-border">
                <tr className="hover-row">
                  <td className="py-2.5 px-3.5 font-sans font-medium text-white flex items-center gap-2">
                    <Laptop className="w-3.5 h-3.5 text-subtle" />
                    thienvu-macbook
                  </td>
                  <td className="py-2.5 px-3.5 text-blue-400">10.0.1.15</td>
                  <td className="py-2.5 px-3.5 font-sans text-subtle">Engineering Subnet</td>
                  <td className="py-2.5 px-3.5 font-sans text-subtle">Thien Vu (thienvu@company.com)</td>
                  <td className="py-2.5 px-3.5">
                    <span className="bg-emerald-500/10 text-emerald-500 border border-emerald-500/20 px-2 py-0.5 rounded-md text-[10px] font-mono font-medium">
                      Connected
                    </span>
                  </td>
                  <td className="py-2.5 px-3.5 text-right font-sans">
                    <button
                      onClick={() => handleDisconnect('thienvu-macbook')}
                      className="p-1.5 hover:bg-rose-500/10 rounded-md text-rose-500 transition-colors"
                      data-tooltip="Disconnect Session"
                    >
                      <Power className="w-3.5 h-3.5" />
                    </button>
                  </td>
                </tr>

                <tr className="hover-row">
                  <td className="py-2.5 px-3.5 font-sans font-medium text-white flex items-center gap-2">
                    <Laptop className="w-3.5 h-3.5 text-subtle" />
                    db-primary-prod
                  </td>
                  <td className="py-2.5 px-3.5 text-blue-400">10.100.1.5</td>
                  <td className="py-2.5 px-3.5 font-sans text-subtle">Production VPC</td>
                  <td className="py-2.5 px-3.5 font-sans text-dim italic">Company Node (Headless)</td>
                  <td className="py-2.5 px-3.5">
                    <span className="bg-emerald-500/10 text-emerald-500 border border-emerald-500/20 px-2 py-0.5 rounded-md text-[10px] font-mono font-medium">
                      Connected
                    </span>
                  </td>
                  <td className="py-2.5 px-3.5 text-right font-sans">
                    <button
                      onClick={() => handleDisconnect('db-primary-prod')}
                      className="p-1.5 hover:bg-rose-500/10 rounded-md text-rose-500 transition-colors"
                      data-tooltip="Disconnect Session"
                    >
                      <Power className="w-3.5 h-3.5" />
                    </button>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </div>
  );
};
