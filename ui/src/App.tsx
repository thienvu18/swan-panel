import React, { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { Sidebar } from './components/Sidebar';
import { Header } from './components/Header';
import { ErrorBoundary } from './components/ErrorBoundary';
import { LoginPage } from './pages/LoginPage';
import { DashboardPage } from './pages/DashboardPage';
import { ResourcesPage } from './pages/ResourcesPage';
import { AccessRulesPage } from './pages/AccessRulesPage';
import { UsersPage } from './pages/UsersPage';
import { DNSEnginePage } from './pages/DNSEnginePage';
import { OIDCPage } from './pages/OIDCPage';
import { ServerSettingsPage } from './pages/ServerSettingsPage';
import { SystemLogsPage } from './pages/SystemLogsPage';
import { useRealtime } from './hooks/useRealtime';
import { CurrentUser } from './types';

const VALID_TABS = ['dashboard', 'resources', 'access', 'users', 'dns', 'oidc', 'server', 'logs'] as const;

export const App: React.FC = () => {
  const { t } = useTranslation();
  const [token, setToken] = useState<string | null>(() => localStorage.getItem('token'));
  const [currentUser, setCurrentUser] = useState<CurrentUser | null>(() => {
    try {
      const stored = localStorage.getItem('user');
      return stored ? JSON.parse(stored) : null;
    } catch {
      return null;
    }
  });

  // Read initial active tab from URL hash (e.g. #/resources) or fallback to dashboard
  const [currentTab, setCurrentTab] = useState<string>(() => {
    const hash = window.location.hash.replace(/^#\/?/, '').trim();
    if (VALID_TABS.includes(hash as any)) {
      return hash;
    }
    return 'dashboard';
  });

  const [isMobileMenuOpen, setIsMobileMenuOpen] = useState(false);
  const [isDark, setIsDark] = useState(() => {
    return localStorage.getItem('theme') !== 'light';
  });
  const [contextualBtn, setContextualBtn] = useState<React.ReactNode>(null);

  // Real-time SSE listener
  const { isConnected } = useRealtime(token);

  // Sync tab with browser back/forward buttons and direct URL navigation
  useEffect(() => {
    const handleHashChange = () => {
      const hash = window.location.hash.replace(/^#\/?/, '').trim();
      if (VALID_TABS.includes(hash as any)) {
        setCurrentTab(hash);
      }
    };

    window.addEventListener('hashchange', handleHashChange);
    return () => window.removeEventListener('hashchange', handleHashChange);
  }, []);

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

  const handleLoginSuccess = (newToken: string, newUser: CurrentUser) => {
    setToken(newToken);
    setCurrentUser(newUser);
  };

  const handleLogout = () => {
    localStorage.removeItem('token');
    localStorage.removeItem('user');
    setToken(null);
    setCurrentUser(null);
    setContextualBtn(null);
    window.location.hash = '';
  };

  const handleSelectTab = (tab: string) => {
    setCurrentTab(tab);
    window.location.hash = `#/${tab}`;
    if (tab !== 'server') {
      setContextualBtn(null);
    }
  };

  // If user is not authenticated, show direct enterprise login page
  if (!token) {
    return <LoginPage onLoginSuccess={handleLoginSuccess} />;
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
      default:
        return <DashboardPage />;
    }
  };

  return (
    <ErrorBoundary>
      <div className="min-h-screen flex antialiased text-xs font-sans bg-transparent text-current">
        {/* Responsive Sidebar */}
        <Sidebar
          currentTab={currentTab}
          onSelectTab={handleSelectTab}
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
    </ErrorBoundary>
  );
};

export default App;
