import React, { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { Sidebar } from './components/Sidebar';
import { Header } from './components/Header';
import { LandingPage } from './pages/LandingPage';
import { DashboardPage } from './pages/DashboardPage';
import { ResourcesPage } from './pages/ResourcesPage';
import { AccessRulesPage } from './pages/AccessRulesPage';
import { UsersPage } from './pages/UsersPage';
import { DNSEnginePage } from './pages/DNSEnginePage';
import { OIDCPage } from './pages/OIDCPage';
import { ServerSettingsPage } from './pages/ServerSettingsPage';
import { SystemLogsPage } from './pages/SystemLogsPage';
import { useRealtime } from './hooks/useRealtime';

export const App: React.FC = () => {
  const { t } = useTranslation();
  const [token, setToken] = useState<string | null>(() => localStorage.getItem('token'));
  const [currentUser, setCurrentUser] = useState<any>(() => {
    try {
      const stored = localStorage.getItem('user');
      return stored ? JSON.parse(stored) : null;
    } catch {
      return null;
    }
  });

  const [currentTab, setCurrentTab] = useState('dashboard');
  const [isMobileMenuOpen, setIsMobileMenuOpen] = useState(false);
  const [isDark, setIsDark] = useState(() => {
    return localStorage.getItem('theme') !== 'light';
  });
  const [contextualBtn, setContextualBtn] = useState<React.ReactNode>(null);

  // Real-time SSE listener
  const { isConnected } = useRealtime(token);

  useEffect(() => {
    const root = document.documentElement;
    if (isDark) {
      root.classList.add('dark');
      localStorage.setItem('theme', 'dark');
    } else {
      root.classList.remove('dark');
      localStorage.setItem('theme', 'light');
    }
  }, [isDark]);

  const handleLoginSuccess = (newToken: string, newUser: any) => {
    setToken(newToken);
    setCurrentUser(newUser);
  };

  const handleLogout = () => {
    localStorage.removeItem('token');
    localStorage.removeItem('user');
    setToken(null);
    setCurrentUser(null);
    setContextualBtn(null);
  };

  // If user is not authenticated, show modern landing page with guides & login
  if (!token) {
    return <LandingPage onLoginSuccess={handleLoginSuccess} />;
  }

  const titles: Record<string, string> = {
    dashboard: t('nav.dashboard'),
    resources: t('nav.resources'),
    access: t('nav.access_rules'),
    users: t('nav.users'),
    dns: t('nav.dns_engine'),
    oidc: t('nav.oidc_providers'),
    server: t('nav.server_settings'),
    logs: t('nav.system_logs'),
    guide: t('nav.guide'),
  };

  const renderContent = () => {
    switch (currentTab) {
      case 'dashboard':
        return <DashboardPage />;
      case 'resources':
        return <ResourcesPage />;
      case 'access':
        return <AccessRulesPage />;
      case 'users':
        return <UsersPage />;
      case 'dns':
        return <DNSEnginePage />;
      case 'oidc':
        return <OIDCPage />;
      case 'server':
        return <ServerSettingsPage onRegisterSaveButton={setContextualBtn} />;
      case 'logs':
        return <SystemLogsPage />;
      case 'guide':
        return (
          <div className="-m-4 md:-m-6">
            <LandingPage
              isEmbedded={true}
              defaultTab="install"
              onBackToAdmin={() => setCurrentTab('dashboard')}
            />
          </div>
        );
      default:
        return <DashboardPage />;
    }
  };

  return (
    <div className="min-h-screen flex antialiased text-xs font-sans bg-transparent text-current">
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
        currentUser={currentUser}
        onLogout={handleLogout}
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
