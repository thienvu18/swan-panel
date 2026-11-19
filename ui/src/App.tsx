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
import { ToastProvider } from './components/ToastProvider';
import { CurrentUser } from './types';

const VALID_TABS = ['dashboard', 'resources', 'access', 'users', 'dns', 'oidc', 'server', 'logs'] as const;

const AppShell: React.FC = () => {
  const { t } = useTranslation();
  const [token, setToken] = useState<string | null>(() => localStorage.getItem('token'));
  const [userVerified, setUserVerified] = useState(false);
  const [authError, setAuthError] = useState(false);
  const [authRetry, setAuthRetry] = useState(0);
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
      if (VALID_TABS.includes(hash as any) && (currentUser?.is_admin || hash === "dashboard" || hash === "resources")) {
        setCurrentTab(hash);
      }
    };

    window.addEventListener('hashchange', handleHashChange);
    return () => window.removeEventListener('hashchange', handleHashChange);
  }, [currentUser?.is_admin]);

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
    setUserVerified(true);
  };

  const handleLogout = () => {
    localStorage.removeItem('token');
    localStorage.removeItem('user');
    setToken(null);
    setCurrentUser(null);
    setUserVerified(false);
    setAuthError(false);
    setContextualBtn(null);
    window.location.hash = '';
  };

  // Local storage is not an authority for current admin privileges.
  useEffect(() => {
    if (!token) return;
    let canceled = false;
    setAuthError(false);
    setUserVerified(false);
    fetch("/api/auth/me", { headers: { Authorization: "Bearer " + token }, cache: "no-store" })
      .then(async response => {
        if (canceled) return;
        if (!response.ok) {
          if (response.status === 401 || response.status === 403) { handleLogout(); }
          else { setAuthError(true); }
          return;
        }
        const data = await response.json();
        if (!canceled && data.user) {
          setCurrentUser(data.user);
          localStorage.setItem("user", JSON.stringify(data.user));
          setUserVerified(true);
          setAuthError(false);
          if (!data.user.is_admin && !["dashboard", "resources"].includes(currentTab)) {
            setCurrentTab("dashboard"); window.location.hash = "#/dashboard";
          }
        } else if (!canceled) {
          setAuthError(true);
        }
      }).catch(() => { if (!canceled) setAuthError(true); });
    return () => { canceled = true; };
  // Revalidate whenever the token changes; the backend still authorizes every request.
  }, [token, authRetry]);

  const handleSelectTab = (tab: string) => {
    if (!currentUser?.is_admin && !["dashboard", "resources"].includes(tab)) return;
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

  if (!userVerified) {
    return (
      <div className="min-h-screen flex items-center justify-center p-6">
        {authError ? (
          <div role="alert" className="max-w-sm space-y-3 text-center">
            <p className="text-sm text-rose-500">{t('common.load_failed')}</p>
            <button type="button" onClick={() => setAuthRetry(retry => retry + 1)}
              className="rounded-lg bg-blue-600 hover:bg-blue-500 text-white px-4 py-2 text-xs font-medium">
              {t('common.retry')}
            </button>
          </div>
        ) : <div role="status" className="text-xs text-subtle">{t('auth.verifying_permissions')}</div>}
      </div>
    );
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
        return <DashboardPage isAdmin={Boolean(currentUser?.is_admin)} currentUserID={currentUser?.id ?? ''} />;
      case 'resources':
        return <ResourcesPage isAdmin={Boolean(currentUser?.is_admin)} />;
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
        return <SystemLogsPage isRealtimeConnected={isConnected} />;
      default:
        return <DashboardPage isAdmin={Boolean(currentUser?.is_admin)} currentUserID={currentUser?.id ?? ''} />;
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

export const App: React.FC = () => <ToastProvider><AppShell /></ToastProvider>;

export default App;