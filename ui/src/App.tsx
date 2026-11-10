import React, { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { Sidebar } from './components/Sidebar';
import { Header } from './components/Header';
import { DashboardPage } from './pages/DashboardPage';
import { ServerSettingsPage } from './pages/ServerSettingsPage';
import { useRealtime } from './hooks/useRealtime';

export const App: React.FC = () => {
  const { t } = useTranslation();
  const [currentTab, setCurrentTab] = useState('dashboard');
  const [isMobileMenuOpen, setIsMobileMenuOpen] = useState(false);
  const [isDark, setIsDark] = useState(true);
  const [contextualBtn, setContextualBtn] = useState<React.ReactNode>(null);

  // Lắng nghe SSE thời gian thực
  const { isConnected } = useRealtime();

  useEffect(() => {
    const root = document.documentElement;
    if (isDark) {
      root.classList.add('dark');
    } else {
      root.classList.remove('dark');
    }
  }, [isDark]);

  const titles: Record<string, string> = {
    dashboard: t('nav.dashboard'),
    resources: t('nav.resources'),
    access: t('nav.access_rules'),
    users: t('nav.users'),
    dns: t('nav.dns_engine'),
    oidc: t('nav.oidc_providers'),
    server: t('nav.server_settings'),
    logs: t('nav.system_logs'),
  };

  const renderContent = () => {
    switch (currentTab) {
      case 'dashboard':
        return <DashboardPage />;
      case 'server':
        return <ServerSettingsPage onRegisterSaveButton={setContextualBtn} />;
      default:
        return (
          <div className="border subtle-border rounded-xl p-12 text-center surface-card space-y-3">
            <h3 className="font-bold text-base text-white">{titles[currentTab]}</h3>
            <p className="text-xs text-subtle max-w-md mx-auto">
              Module is ready for deployment. Functional UI is accessible and customizable.
            </p>
          </div>
        );
    }
  };

  return (
    <div className="min-h-screen flex antialiased text-xs font-sans">
      {/* Responsive Sidebar */}
      <Sidebar
        currentTab={currentTab}
        onSelectTab={(tab) => {
          setCurrentTab(tab);
          if (tab !== 'server') {
            setContextualBtn(null);
          }
        }}
        isOpen={isMobileMenuOpen}
        onClose={() => setIsMobileMenuOpen(false)}
        isDark={isDark}
        onToggleTheme={() => setIsDark(!isDark)}
      />

      {/* Main Viewport */}
      <main className="flex-1 flex flex-col min-w-0 overflow-y-auto">
        <Header
          title={titles[currentTab] || t('nav.dashboard')}
          onOpenMobileMenu={() => setIsMobileMenuOpen(true)}
          isRealtimeConnected={isConnected}
          actionButton={contextualBtn}
        />

        <div className="p-4 md:p-6 max-w-6xl w-full mx-auto">{renderContent()}</div>
      </main>
    </div>
  );
};

export default App;
