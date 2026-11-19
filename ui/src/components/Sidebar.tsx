import React from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import {
  LayoutDashboard,
  Server,
  ShieldCheck,
  Users,
  Globe,
  KeyRound,
  Settings,
  Terminal,
  Sun,
  Moon,
  LogOut,
  X
} from 'lucide-react';
import { SwanLogo } from './SwanLogo';

interface SidebarProps {
  currentTab: string;
  onSelectTab: (tab: string) => void;
  isOpen: boolean;
  onClose: () => void;
  isDark: boolean;
  onToggleTheme: () => void;
  currentUser?: { name?: string; email?: string; is_admin?: boolean } | null;
  onLogout?: () => void;
}

export const Sidebar: React.FC<SidebarProps> = ({
  currentTab,
  onSelectTab,
  isOpen,
  onClose,
  isDark,
  onToggleTheme,
  currentUser,
  onLogout,
}) => {
  const { t, i18n } = useTranslation();

  const toggleLanguage = () => {
    const nextLang = i18n.language.startsWith('vi') ? 'en' : 'vi';
    i18n.changeLanguage(nextLang);
  };

  // Query real dashboard counts for dynamic sidebar badges
  const { data: statsData } = useQuery<{ stats: any }>({
    queryKey: ['dashboard_stats'],
    queryFn: async () => {
      const res = await fetch('/api/dashboard/stats', {
        headers: { Authorization: `Bearer ${localStorage.getItem('token') || ''}` },
      });
      if (!res.ok) return null;
      return res.json();
    },
    enabled: !!localStorage.getItem('token'),
  });

  const stats = statsData?.stats;
  const gatewayStatusText = stats?.gateway_status === 'online'
    ? t('nav.gateway_online')
    : stats?.gateway_status === 'offline'
      ? t('nav.gateway_offline')
      : t('nav.gateway_unknown');

  const navItems = [
    { id: 'dashboard', label: t('nav.dashboard'), icon: LayoutDashboard },
    {
      id: 'resources',
      label: t('nav.resources'),
      icon: Server,
      badge: stats?.total_devices > 0 ? String(stats.total_devices) : undefined,
    },
    { id: 'access', label: t('nav.access_rules'), icon: ShieldCheck },
    {
      id: 'users',
      label: t('nav.users'),
      icon: Users,
      badge: stats?.pending_users > 0 ? String(stats.pending_users) : undefined,
      badgeColor: 'bg-amber-500/15 text-status-warning',
    },
    { id: 'dns', label: t('nav.dns_engine'), icon: Globe },
    { id: 'oidc', label: t('nav.oidc_providers'), icon: KeyRound },
    { id: 'server', label: t('nav.server_settings'), icon: Settings },
    { id: 'logs', label: t('nav.system_logs'), icon: Terminal },
  ].filter(item => currentUser?.is_admin || item.id === "dashboard" || item.id === "resources");

  return (
    <>
      {/* Mobile Backdrop */}
      {isOpen && (
        <div
          className="fixed inset-0 bg-black/60 backdrop-blur-xs z-40 md:hidden"
          onClick={onClose}
        />
      )}

      {/* Sidebar Container */}
      <aside
        className={`fixed md:static inset-y-0 left-0 z-50 w-56 surface-nav border-r subtle-border flex flex-col shrink-0 select-none transition-transform duration-200 ease-in-out ${
          isOpen ? 'translate-x-0' : '-translate-x-full md:translate-x-0'
        }`}
      >
        {/* Header & Controls */}
        <div className="px-3 py-2 border-b subtle-border shrink-0">
          <div className="flex items-center justify-between gap-1.5">
            <div className="flex items-center min-w-0 gap-2">
              <div className="w-8 h-8 rounded-lg surface-header border subtle-border flex items-center justify-center p-1 shadow-xs shrink-0">
                <SwanLogo className="w-6 h-6" />
              </div>
              <span className="font-bold text-sm tracking-tight truncate">SwanPanel</span>
            </div>

            <div className="flex items-center gap-1 shrink-0">
              {/* Language Switcher */}
              <button
                onClick={toggleLanguage}
                className="px-2 py-1 text-[11px] font-mono font-medium rounded-md border subtle-border hover:bg-slate-500/10 text-subtle hover:text-current transition-colors flex items-center gap-1"
                title={t('nav.switch_language')}
                aria-label={t('nav.switch_language')}
              >
                <Globe className="w-3 h-3" />
                <span>{i18n.language.startsWith('vi') ? 'VI' : 'EN'}</span>
              </button>

              {/* Theme Toggle */}
              <button
                onClick={onToggleTheme}
                aria-label={t('nav.toggle_theme')}
                className="p-1.5 rounded-md hover:bg-slate-500/10 text-subtle hover:text-current transition-colors"
                title={t('nav.toggle_theme')}
              >
                {isDark ? <Sun className="w-3.5 h-3.5" /> : <Moon className="w-3.5 h-3.5" />}
              </button>
            </div>
          </div>

          {/* Keep translated gateway status on a separate row from the language/theme controls. */}
          <div className="mt-1.5 pl-10 flex items-center justify-between gap-1.5">
            <div className="min-w-0 flex items-center gap-1 text-[10px] font-medium text-subtle" title={gatewayStatusText}>
              <span className={`w-1.5 h-1.5 rounded-full shrink-0 ${
                stats?.gateway_status === 'online' ? 'bg-emerald-500' : stats?.gateway_status === 'offline' ? 'bg-rose-500' : 'bg-slate-500'
              }`}></span>
              <span className="truncate">{gatewayStatusText}</span>
            </div>
            <button
              onClick={onClose}
              aria-label={t('common.close')}
              className="p-1.5 rounded-md hover:bg-slate-500/10 text-subtle hover:text-current md:hidden shrink-0"
            >
              <X className="w-4 h-4" />
            </button>
          </div>
        </div>

        {/* Navigation Links */}
        <nav className="flex-1 overflow-y-auto p-2 space-y-1 text-xs">
          {navItems.map((item) => {
            const Icon = item.icon;
            const isActive = currentTab === item.id;

            return (
              <button
                key={item.id}
                data-tab={item.id}
                onClick={() => {
                  onSelectTab(item.id);
                  onClose();
                }}
                className={`w-full flex items-center justify-between px-3 py-2 rounded-lg font-medium transition-colors ${
                  isActive
                    ? 'bg-blue-600/10 text-accent font-semibold'
                    : 'text-subtle hover:text-current hover:bg-slate-500/10'
                }`}
              >
                <span className="flex items-center gap-2.5">
                  <Icon className="w-4 h-4" />
                  {item.label}
                </span>

                {item.badge && (
                  <span
                    className={`font-mono text-[10px] px-1.5 py-0.5 rounded ${
                      item.badgeColor || 'bg-slate-500/10 text-subtle'
                    }`}
                  >
                    {item.badge}
                  </span>
                )}
              </button>
            );
          })}
        </nav>

        {/* Bottom Section (User Profile & Status Footer) */}
        <div className="shrink-0 border-t subtle-border">
          {currentUser && (
            <div className="p-3 surface-header border-b subtle-border">
              <div className="flex items-center justify-between gap-2">
                <div className="min-w-0 flex-1">
                  <p className="text-xs font-semibold text-current truncate">{currentUser.name || 'Admin'}</p>
                  <p className="text-[10px] text-subtle truncate font-mono">{currentUser.email || ''}</p>
                </div>
                {onLogout && (
                  <button
                    onClick={onLogout}
                    className="p-1.5 rounded-lg border subtle-border hover:bg-red-500/10 text-subtle hover:text-red-700 dark:hover:text-red-400 transition-colors shrink-0"
                    title={t('auth.sign_out')}
                    aria-label={t('auth.sign_out')}
                  >
                    <LogOut className="w-3.5 h-3.5" />
                  </button>
                )}
              </div>
            </div>
          )}

          {/* Footer Status */}
          <div className="p-2.5 text-[10px] text-subtle flex items-center justify-between font-mono">
            <span className="flex items-center gap-1.5">
              <span className={`w-1.5 h-1.5 rounded-full ${stats?.gateway_status === 'online' ? 'bg-emerald-500' : stats?.gateway_status === 'offline' ? 'bg-rose-500' : 'bg-slate-500'}`}></span>
              {stats?.gateway_status === 'online' ? t('common.online') : stats?.gateway_status === 'offline' ? t('common.offline') : t('server.status_unknown')}
            </span>
            <span className="text-dim">IKEv2</span>
          </div>
        </div>
      </aside>
    </>
  );
};