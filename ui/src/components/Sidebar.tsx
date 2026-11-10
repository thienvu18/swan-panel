import React from 'react';
import { useTranslation } from 'react-i18next';
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
}

export const Sidebar: React.FC<SidebarProps> = ({
  currentTab,
  onSelectTab,
  isOpen,
  onClose,
  isDark,
  onToggleTheme,
}) => {
  const { t, i18n } = useTranslation();

  const navItems = [
    { id: 'dashboard', label: t('nav.dashboard'), icon: LayoutDashboard },
    { id: 'resources', label: t('nav.resources'), icon: Server, badge: '3' },
    { id: 'access', label: t('nav.access_rules'), icon: ShieldCheck, badge: '3' },
    { id: 'users', label: t('nav.users'), icon: Users, badge: '1', badgeColor: 'bg-amber-500/15 text-amber-500' },
    { id: 'dns', label: t('nav.dns_engine'), icon: Globe, badgeText: '.strongnet' },
    { id: 'oidc', label: t('nav.oidc_providers'), icon: KeyRound, badge: '2' },
    { id: 'server', label: t('nav.server_settings'), icon: Settings },
    { id: 'logs', label: t('nav.system_logs'), icon: Terminal, dot: true },
  ];

  const toggleLanguage = () => {
    const nextLang = i18n.language.startsWith('vi') ? 'en' : 'vi';
    i18n.changeLanguage(nextLang);
  };

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
        className={`fixed md:static inset-y-0 left-0 z-50 w-56 surface-nav border-r subtle-border flex flex-col justify-between shrink-0 select-none transition-transform duration-200 ease-in-out ${
          isOpen ? 'translate-x-0' : '-translate-x-full md:translate-x-0'
        }`}
      >
        <div>
          {/* Header & Controls */}
          <div className="h-14 flex items-center justify-between px-4 border-b subtle-border">
            <div className="flex items-center gap-2.5">
              <div className="w-8 h-8 rounded-lg surface-header border subtle-border flex items-center justify-center p-1 shadow-xs shrink-0">
                <SwanLogo className="w-6 h-6" />
              </div>
              <div>
                <span className="font-bold text-sm tracking-tight">SwanPanel</span>
                <div className="text-[10px] text-emerald-500 font-medium flex items-center gap-1">
                  <span className="w-1.5 h-1.5 rounded-full bg-emerald-500 animate-pulse"></span> {t('nav.gateway_online')}
                </div>
              </div>
            </div>

            <div className="flex items-center gap-1">
              {/* Language Switcher */}
              <button
                onClick={toggleLanguage}
                className="px-1.5 py-1 rounded-md text-[10px] font-bold border subtle-border hover:bg-slate-500/10 text-subtle hover:text-current transition-colors"
                title="Switch Language (EN / VI)"
              >
                {i18n.language.startsWith('vi') ? 'VI' : 'EN'}
              </button>

              {/* Theme Toggle */}
              <button
                onClick={onToggleTheme}
                className="p-1.5 rounded-md hover:bg-slate-500/10 text-subtle hover:text-current transition-colors"
                title="Toggle Theme"
              >
                {isDark ? <Sun className="w-3.5 h-3.5" /> : <Moon className="w-3.5 h-3.5" />}
              </button>

              {/* Mobile Close Button */}
              <button
                onClick={onClose}
                className="p-1.5 rounded-md hover:bg-slate-500/10 text-subtle hover:text-current md:hidden"
              >
                <X className="w-4 h-4" />
              </button>
            </div>
          </div>

          {/* Navigation Links */}
          <nav className="p-2 space-y-1 text-xs">
            {navItems.map((item) => {
              const Icon = item.icon;
              const isActive = currentTab === item.id;

              return (
                <button
                  key={item.id}
                  onClick={() => {
                    onSelectTab(item.id);
                    onClose();
                  }}
                  className={`w-full flex items-center justify-between px-3 py-2 rounded-lg font-medium transition-colors ${
                    isActive
                      ? 'bg-blue-600/10 text-blue-500 font-semibold'
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
                  {item.badgeText && (
                    <span className="font-mono text-[10px] text-blue-500 font-medium">
                      {item.badgeText}
                    </span>
                  )}
                  {item.dot && <span className="w-1.5 h-1.5 rounded-full bg-emerald-500"></span>}
                </button>
              );
            })}
          </nav>
        </div>

        {/* Footer Status */}
        <div className="p-3 border-t subtle-border text-[11px] text-subtle flex items-center justify-between font-mono">
          <span className="flex items-center gap-1.5">
            <span className="w-1.5 h-1.5 rounded-full bg-emerald-500"></span> 10.0.0.1
          </span>
          <span className="text-dim text-[10px]">Zero-Trust Native</span>
        </div>
      </aside>
    </>
  );
};
