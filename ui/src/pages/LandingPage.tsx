import React, { useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Shield,
  ShieldCheck,
  ShieldAlert,
  Server,
  Globe,
  Terminal,
  Zap,
  Laptop,
  ArrowRight,
  CheckCircle2,
  AlertTriangle,
  Lock,
  Mail,
  Layers,
  Cpu,
  Network,
  Sun,
  Moon,
  ExternalLink,
  Smartphone
} from 'lucide-react';
import { SwanLogo } from '../components/SwanLogo';
import { CopyButton } from '../components/CopyButton';
import { Modal } from '../components/Modal';

interface LandingPageProps {
  onLoginSuccess?: (token: string, user: any) => void;
  defaultTab?: 'overview' | 'install' | 'userguide';
  isEmbedded?: boolean;
  onBackToAdmin?: () => void;
}

export const LandingPage: React.FC<LandingPageProps> = ({
  onLoginSuccess,
  defaultTab = 'overview',
  isEmbedded = false,
  onBackToAdmin,
}) => {
  const { t, i18n } = useTranslation();
  const [activeTab, setActiveTab] = useState<'overview' | 'install' | 'userguide'>(defaultTab);
  const [activeOsTab, setActiveOsTab] = useState<'macos' | 'ios' | 'windows' | 'linux' | 'android'>('macos');
  const [installMode, setInstallMode] = useState<'docker' | 'binary'>('docker');
  const [isLoginModalOpen, setIsLoginModalOpen] = useState(false);

  // Form states for login
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [isLoading, setIsLoading] = useState(false);
  const [errorMessage, setErrorMessage] = useState('');

  // Dark / Light Mode state
  const [isDark, setIsDark] = useState(() => {
    return localStorage.getItem('theme') !== 'light';
  });

  const toggleTheme = () => {
    const nextDark = !isDark;
    setIsDark(nextDark);
    if (nextDark) {
      document.documentElement.classList.add('dark');
      localStorage.setItem('theme', 'dark');
    } else {
      document.documentElement.classList.remove('dark');
      localStorage.setItem('theme', 'light');
    }
  };

  const toggleLanguage = () => {
    const nextLang = i18n.language.startsWith('vi') ? 'en' : 'vi';
    i18n.changeLanguage(nextLang);
  };

  const handleFillDefaultAdmin = () => {
    setEmail('admin@swanpanel.internal');
    setPassword('admin123456');
    setErrorMessage('');
  };

  const handleLoginSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!email.trim() || !password.trim()) return;

    setIsLoading(true);
    setErrorMessage('');

    try {
      const res = await fetch('/api/auth/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email: email.trim(), password: password.trim() }),
      });

      const data = await res.json();
      if (!res.ok) {
        setErrorMessage(data.error || t('auth.invalid_credentials'));
        setIsLoading(false);
        return;
      }

      if (data.token && data.user) {
        localStorage.setItem('token', data.token);
        localStorage.setItem('user', JSON.stringify(data.user));
        setIsLoginModalOpen(false);
        if (onLoginSuccess) {
          onLoginSuccess(data.token, data.user);
        }
      } else {
        setErrorMessage(t('auth.invalid_credentials'));
      }
    } catch {
      setErrorMessage(t('auth.network_error'));
    } finally {
      setIsLoading(false);
    }
  };

  const dockerComposeCode = `services:
  swan-panel:
    image: thienvu18/swan-panel:latest
    container_name: swan-panel
    restart: always
    cap_add:
      - NET_ADMIN
      - NET_RAW
    ports:
      - "8080:8080"      # SwanPanel Web Control Plane & API
      - "500:500/udp"    # IKEv2 Key Exchange
      - "4500:4500/udp"  # IPsec NAT-Traversal
      - "53:53/udp"      # MagicDNS Split-DNS Server
      - "53:53/tcp"
    environment:
      - ADMIN_EMAIL=admin@swanpanel.internal
      - ADMIN_PASSWORD=admin123456
      - GATEWAY_FQDN=vpn.company.com
      - LOG_LEVEL=INFO
    volumes:
      - ./data:/data
      - /var/run/charon.vici:/var/run/charon.vici:ro`;

  const binaryRunCode = `# 1. Download latest release
curl -L https://github.com/thienvu18/swan-panel/releases/latest/download/swan-panel-linux-amd64 -o swan-panel
chmod +x swan-panel

# 2. Run with root privileges (required for charon.vici socket and port 53/500/4500)
sudo ./swan-panel --config /etc/swanpanel/config.yaml`;

  return (
    <div className="min-h-screen flex flex-col surface-ground text-slate-900 dark:text-[#f8fafc] font-sans selection:bg-blue-600 selection:text-white">
      {/* =========================================================================
          TOP NAVIGATION BAR
          ========================================================================= */}
      <header className="sticky top-0 z-40 w-full surface-nav backdrop-blur-md border-b subtle-border">
        <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 h-16 flex items-center justify-between gap-4">
          {/* Logo & Brand */}
          <div className="flex items-center gap-3">
            <div className="w-9 h-9 rounded-xl surface-header border subtle-border flex items-center justify-center p-1.5 shadow-xs">
              <SwanLogo className="w-6 h-6" />
            </div>
            <div>
              <span className="font-extrabold text-base tracking-tight flex items-center gap-2">
                SwanPanel
                <span className="text-[10px] uppercase font-mono font-bold tracking-wider px-2 py-0.5 rounded-full bg-blue-600/15 text-blue-500 border border-blue-500/30">
                  Zero-Trust
                </span>
              </span>
            </div>
          </div>

          {/* 3 Main Function Tabs */}
          <nav className="hidden md:flex items-center gap-1 p-1 rounded-xl surface-card border subtle-border text-xs font-medium">
            <button
              onClick={() => setActiveTab('overview')}
              className={`px-3.5 py-1.5 rounded-lg transition-all flex items-center gap-2 ${
                activeTab === 'overview'
                  ? 'bg-blue-600 text-white shadow-xs font-semibold'
                  : 'text-subtle hover:text-current hover:bg-slate-500/10'
              }`}
            >
              <Shield className="w-3.5 h-3.5" />
              {t('landing.nav_overview')}
            </button>
            <button
              onClick={() => setActiveTab('install')}
              className={`px-3.5 py-1.5 rounded-lg transition-all flex items-center gap-2 ${
                activeTab === 'install'
                  ? 'bg-blue-600 text-white shadow-xs font-semibold'
                  : 'text-subtle hover:text-current hover:bg-slate-500/10'
              }`}
            >
              <Terminal className="w-3.5 h-3.5" />
              {t('landing.nav_install')}
            </button>
            <button
              onClick={() => setActiveTab('userguide')}
              className={`px-3.5 py-1.5 rounded-lg transition-all flex items-center gap-2 ${
                activeTab === 'userguide'
                  ? 'bg-blue-600 text-white shadow-xs font-semibold'
                  : 'text-subtle hover:text-current hover:bg-slate-500/10'
              }`}
            >
              <Laptop className="w-3.5 h-3.5" />
              {t('landing.nav_user_guide')}
            </button>
          </nav>

          {/* Controls & Sign-In CTA */}
          <div className="flex items-center gap-2">
            {/* Language Switcher */}
            <button
              onClick={toggleLanguage}
              className="px-2.5 py-1.5 rounded-lg text-xs font-semibold border subtle-border surface-header hover:bg-slate-500/10 text-subtle hover:text-current transition-colors flex items-center gap-1.5"
              title="Switch Language (EN / VI)"
              aria-label={`Switch to ${i18n.language.startsWith('vi') ? 'English' : 'Tiếng Việt'}`}
            >
              <Globe className="w-3.5 h-3.5" />
              <span className="uppercase text-[11px] font-mono tracking-wide">
                {i18n.language.startsWith('vi') ? 'VI' : 'EN'}
              </span>
            </button>

            {/* Theme Toggle */}
            <button
              onClick={toggleTheme}
              className="p-1.5 rounded-lg border subtle-border surface-header hover:bg-slate-500/10 text-subtle hover:text-current transition-colors"
              title="Toggle Light / Dark Mode"
              aria-label="Toggle Theme"
            >
              {isDark ? <Sun className="w-4 h-4 text-amber-400" /> : <Moon className="w-4 h-4 text-slate-600" />}
            </button>

            {/* If embedded inside logged in app, show Return button */}
            {isEmbedded && onBackToAdmin ? (
              <button
                onClick={onBackToAdmin}
                className="bg-blue-600 hover:bg-blue-500 text-white font-medium px-3.5 py-1.5 rounded-lg text-xs shadow-xs transition-colors flex items-center gap-1.5"
              >
                <ArrowRight className="w-3.5 h-3.5 rotate-180" />
                {t('landing.back_to_dashboard')}
              </button>
            ) : (
              <button
                onClick={() => setIsLoginModalOpen(true)}
                className="bg-blue-600 hover:bg-blue-500 text-white font-semibold px-4 py-1.5 rounded-lg text-xs shadow-sm transition-all flex items-center gap-1.5"
              >
                <Lock className="w-3.5 h-3.5" />
                {t('landing.sign_in')}
              </button>
            )}
          </div>
        </div>

        {/* Mobile Navigation Sub-bar */}
        <div className="md:hidden flex items-center justify-around border-t subtle-border p-1.5 text-xs surface-card">
          <button
            onClick={() => setActiveTab('overview')}
            className={`px-2 py-1 rounded-md ${activeTab === 'overview' ? 'bg-blue-600 text-white font-semibold' : 'text-subtle'}`}
          >
            {t('landing.nav_overview')}
          </button>
          <button
            onClick={() => setActiveTab('install')}
            className={`px-2 py-1 rounded-md ${activeTab === 'install' ? 'bg-blue-600 text-white font-semibold' : 'text-subtle'}`}
          >
            {t('landing.nav_install')}
          </button>
          <button
            onClick={() => setActiveTab('userguide')}
            className={`px-2 py-1 rounded-md ${activeTab === 'userguide' ? 'bg-blue-600 text-white font-semibold' : 'text-subtle'}`}
          >
            {t('landing.nav_user_guide')}
          </button>
        </div>
      </header>

      {/* =========================================================================
          TAB 1: LANDING PAGE (HERO, PAIN POINTS, SOLUTIONS, CTA)
          ========================================================================= */}
      {activeTab === 'overview' && (
        <main className="flex-1 space-y-16 pb-20">
          {/* Hero Section */}
          <section className="relative overflow-hidden pt-12 md:pt-20 pb-16 px-4 sm:px-6 lg:px-8 max-w-7xl mx-auto text-center">
            <div className="inline-flex items-center gap-2 px-3 py-1 rounded-full border subtle-border surface-card text-xs text-subtle mb-6 shadow-xs animate-fade-in">
              <span className="w-2 h-2 rounded-full bg-emerald-500 animate-pulse" />
              <span className="font-medium text-slate-800 dark:text-slate-200">{t('landing.badge')}</span>
            </div>

            <h1 className="text-3xl sm:text-5xl lg:text-6xl font-black tracking-tight max-w-4xl mx-auto leading-tight text-slate-900 dark:text-white">
              {t('landing.hero_title')}
            </h1>

            <p className="mt-5 text-sm sm:text-base lg:text-lg text-subtle max-w-2xl mx-auto leading-relaxed">
              {t('landing.hero_subtitle')}
            </p>

            {/* CTAs */}
            <div className="mt-8 flex flex-wrap items-center justify-center gap-3">
              <button
                onClick={() => setIsLoginModalOpen(true)}
                className="bg-blue-600 hover:bg-blue-500 text-white font-bold px-6 py-2.5 rounded-xl text-sm shadow-lg shadow-blue-600/25 transition-all flex items-center gap-2 hover:scale-[1.02]"
              >
                <Lock className="w-4 h-4" />
                {t('landing.cta_sign_in')}
              </button>
              <button
                onClick={() => setActiveTab('install')}
                className="surface-card hover:bg-slate-500/10 border subtle-border text-slate-800 dark:text-slate-200 font-semibold px-5 py-2.5 rounded-xl text-sm transition-all flex items-center gap-2"
              >
                <Terminal className="w-4 h-4 text-blue-500" />
                {t('landing.cta_install')}
              </button>
              <button
                onClick={() => setActiveTab('userguide')}
                className="surface-card hover:bg-slate-500/10 border subtle-border text-slate-800 dark:text-slate-200 font-semibold px-5 py-2.5 rounded-xl text-sm transition-all flex items-center gap-2"
              >
                <Laptop className="w-4 h-4 text-emerald-500" />
                {t('landing.cta_user_guide')}
              </button>
            </div>

            {/* 4 Architectural Pillars Grid */}
            <div className="mt-14 grid grid-cols-2 md:grid-cols-4 gap-3 sm:gap-4 max-w-4xl mx-auto text-left">
              <div className="p-4 rounded-xl surface-card border subtle-border shadow-xs">
                <div className="text-blue-500 font-mono font-bold text-lg mb-1">{t('landing.stat_binary')}</div>
                <div className="text-xs text-subtle">{t('landing.stat_binary_desc')}</div>
              </div>
              <div className="p-4 rounded-xl surface-card border subtle-border shadow-xs">
                <div className="text-emerald-500 font-mono font-bold text-lg mb-1">{t('landing.stat_native')}</div>
                <div className="text-xs text-subtle">{t('landing.stat_native_desc')}</div>
              </div>
              <div className="p-4 rounded-xl surface-card border subtle-border shadow-xs">
                <div className="text-amber-500 font-mono font-bold text-lg mb-1">{t('landing.stat_downtime')}</div>
                <div className="text-xs text-subtle">{t('landing.stat_downtime_desc')}</div>
              </div>
              <div className="p-4 rounded-xl surface-card border subtle-border shadow-xs">
                <div className="text-indigo-500 font-mono font-bold text-lg mb-1">{t('landing.stat_zt')}</div>
                <div className="text-xs text-subtle">{t('landing.stat_zt_desc')}</div>
              </div>
            </div>
          </section>

          {/* Section: PAIN POINTS (Nỗi đau của VPN truyền thống) */}
          <section className="max-w-6xl mx-auto px-4 sm:px-6 lg:px-8">
            <div className="text-center max-w-3xl mx-auto mb-10 space-y-2">
              <span className="text-rose-500 uppercase font-mono text-xs font-bold tracking-wider px-2.5 py-1 rounded-full bg-rose-500/10 border border-rose-500/20">
                {t('landing.pain_tag')}
              </span>
              <h2 className="text-2xl sm:text-3xl font-extrabold tracking-tight text-slate-900 dark:text-white">
                {t('landing.pain_title')}
              </h2>
              <p className="text-xs sm:text-sm text-subtle leading-relaxed">
                {t('landing.pain_subtitle')}
              </p>
            </div>

            <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
              <div className="p-5 rounded-2xl surface-card border border-rose-500/20 bg-rose-500/[0.02] shadow-xs space-y-2.5">
                <div className="w-8 h-8 rounded-lg bg-rose-500/15 border border-rose-500/30 flex items-center justify-center text-rose-500">
                  <Terminal className="w-4 h-4" />
                </div>
                <h3 className="font-bold text-sm text-slate-900 dark:text-white">{t('landing.pain_cli_title')}</h3>
                <p className="text-xs text-subtle leading-relaxed">{t('landing.pain_cli_desc')}</p>
              </div>

              <div className="p-5 rounded-2xl surface-card border border-rose-500/20 bg-rose-500/[0.02] shadow-xs space-y-2.5">
                <div className="w-8 h-8 rounded-lg bg-rose-500/15 border border-rose-500/30 flex items-center justify-center text-rose-500">
                  <Smartphone className="w-4 h-4" />
                </div>
                <h3 className="font-bold text-sm text-slate-900 dark:text-white">{t('landing.pain_client_title')}</h3>
                <p className="text-xs text-subtle leading-relaxed">{t('landing.pain_client_desc')}</p>
              </div>

              <div className="p-5 rounded-2xl surface-card border border-rose-500/20 bg-rose-500/[0.02] shadow-xs space-y-2.5">
                <div className="w-8 h-8 rounded-lg bg-rose-500/15 border border-rose-500/30 flex items-center justify-center text-rose-500">
                  <Lock className="w-4 h-4" />
                </div>
                <h3 className="font-bold text-sm text-slate-900 dark:text-white">{t('landing.pain_pki_title')}</h3>
                <p className="text-xs text-subtle leading-relaxed">{t('landing.pain_pki_desc')}</p>
              </div>

              <div className="p-5 rounded-2xl surface-card border border-rose-500/20 bg-rose-500/[0.02] shadow-xs space-y-2.5 md:col-span-1.5">
                <div className="w-8 h-8 rounded-lg bg-rose-500/15 border border-rose-500/30 flex items-center justify-center text-rose-500">
                  <AlertTriangle className="w-4 h-4" />
                </div>
                <h3 className="font-bold text-sm text-slate-900 dark:text-white">{t('landing.pain_reload_title')}</h3>
                <p className="text-xs text-subtle leading-relaxed">{t('landing.pain_reload_desc')}</p>
              </div>

              <div className="p-5 rounded-2xl surface-card border border-rose-500/20 bg-rose-500/[0.02] shadow-xs space-y-2.5 md:col-span-2">
                <div className="w-8 h-8 rounded-lg bg-rose-500/15 border border-rose-500/30 flex items-center justify-center text-rose-500">
                  <ShieldAlert className="w-4 h-4" />
                </div>
                <h3 className="font-bold text-sm text-slate-900 dark:text-white">{t('landing.pain_zt_title')}</h3>
                <p className="text-xs text-subtle leading-relaxed">{t('landing.pain_zt_desc')}</p>
              </div>
            </div>
          </section>

          {/* Section: THE SOLUTION (Giải pháp SwanPanel) */}
          <section className="max-w-6xl mx-auto px-4 sm:px-6 lg:px-8">
            <div className="text-center max-w-3xl mx-auto mb-10 space-y-2">
              <span className="text-blue-500 uppercase font-mono text-xs font-bold tracking-wider px-2.5 py-1 rounded-full bg-blue-600/10 border border-blue-500/20">
                {t('landing.sol_tag')}
              </span>
              <h2 className="text-2xl sm:text-3xl font-extrabold tracking-tight text-slate-900 dark:text-white">
                {t('landing.sol_title')}
              </h2>
              <p className="text-xs sm:text-sm text-subtle leading-relaxed">
                {t('landing.sol_subtitle')}
              </p>
            </div>

            <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
              <div className="p-5 rounded-2xl surface-card border border-blue-500/25 bg-blue-600/[0.03] shadow-xs space-y-2.5">
                <div className="w-8 h-8 rounded-lg bg-blue-600/15 border border-blue-500/30 flex items-center justify-center text-blue-500">
                  <CheckCircle2 className="w-4 h-4" />
                </div>
                <h3 className="font-bold text-sm text-slate-900 dark:text-white">{t('landing.sol_native_title')}</h3>
                <p className="text-xs text-subtle leading-relaxed">{t('landing.sol_native_desc')}</p>
              </div>

              <div className="p-5 rounded-2xl surface-card border border-blue-500/25 bg-blue-600/[0.03] shadow-xs space-y-2.5">
                <div className="w-8 h-8 rounded-lg bg-blue-600/15 border border-blue-500/30 flex items-center justify-center text-blue-500">
                  <Zap className="w-4 h-4" />
                </div>
                <h3 className="font-bold text-sm text-slate-900 dark:text-white">{t('landing.sol_vici_title')}</h3>
                <p className="text-xs text-subtle leading-relaxed">{t('landing.sol_vici_desc')}</p>
              </div>

              <div className="p-5 rounded-2xl surface-card border border-blue-500/25 bg-blue-600/[0.03] shadow-xs space-y-2.5">
                <div className="w-8 h-8 rounded-lg bg-blue-600/15 border border-blue-500/30 flex items-center justify-center text-blue-500">
                  <Cpu className="w-4 h-4" />
                </div>
                <h3 className="font-bold text-sm text-slate-900 dark:text-white">{t('landing.sol_binary_title')}</h3>
                <p className="text-xs text-subtle leading-relaxed">{t('landing.sol_binary_desc')}</p>
              </div>

              <div className="p-5 rounded-2xl surface-card border border-blue-500/25 bg-blue-600/[0.03] shadow-xs space-y-2.5">
                <div className="w-8 h-8 rounded-lg bg-blue-600/15 border border-blue-500/30 flex items-center justify-center text-blue-500">
                  <Globe className="w-4 h-4" />
                </div>
                <h3 className="font-bold text-sm text-slate-900 dark:text-white">{t('landing.sol_dns_title')}</h3>
                <p className="text-xs text-subtle leading-relaxed">{t('landing.sol_dns_desc')}</p>
              </div>

              <div className="p-5 rounded-2xl surface-card border border-blue-500/25 bg-blue-600/[0.03] shadow-xs space-y-2.5">
                <div className="w-8 h-8 rounded-lg bg-blue-600/15 border border-blue-500/30 flex items-center justify-center text-blue-500">
                  <ShieldCheck className="w-4 h-4" />
                </div>
                <h3 className="font-bold text-sm text-slate-900 dark:text-white">{t('landing.sol_zt_title')}</h3>
                <p className="text-xs text-subtle leading-relaxed">{t('landing.sol_zt_desc')}</p>
              </div>

              <div className="p-5 rounded-2xl surface-card border border-blue-500/25 bg-blue-600/[0.03] shadow-xs space-y-2.5">
                <div className="w-8 h-8 rounded-lg bg-blue-600/15 border border-blue-500/30 flex items-center justify-center text-blue-500">
                  <Server className="w-4 h-4" />
                </div>
                <h3 className="font-bold text-sm text-slate-900 dark:text-white">{t('landing.sol_obs_title')}</h3>
                <p className="text-xs text-subtle leading-relaxed">{t('landing.sol_obs_desc')}</p>
              </div>
            </div>
          </section>

          {/* Bottom Call to Action Banner */}
          <section className="max-w-5xl mx-auto px-4 sm:px-6 lg:px-8">
            <div className="rounded-3xl p-8 sm:p-10 border subtle-border surface-card text-center space-y-4 shadow-xl relative overflow-hidden">
              <div className="absolute inset-0 bg-blue-600/5 pointer-events-none" />
              <h2 className="text-2xl sm:text-3xl font-extrabold tracking-tight text-slate-900 dark:text-white">
                {t('landing.cta_banner_title')}
              </h2>
              <p className="text-xs sm:text-sm text-subtle max-w-xl mx-auto">
                {t('landing.cta_banner_desc')}
              </p>
              <div className="pt-2 flex flex-wrap items-center justify-center gap-3">
                <button
                  onClick={() => setActiveTab('install')}
                  className="bg-blue-600 hover:bg-blue-500 text-white font-bold px-6 py-2.5 rounded-xl text-xs shadow-md transition-all flex items-center gap-2"
                >
                  <Terminal className="w-4 h-4" />
                  {t('landing.cta_banner_btn')}
                </button>
                <button
                  onClick={() => setIsLoginModalOpen(true)}
                  className="surface-header border subtle-border hover:bg-slate-500/10 font-semibold px-5 py-2.5 rounded-xl text-xs transition-colors"
                >
                  {t('landing.sign_in')}
                </button>
              </div>
            </div>
          </section>
        </main>
      )}

      {/* =========================================================================
          TAB 2: HƯỚNG DẪN CÀI ĐẶT & CẤU HÌNH (ADMIN QUICKSTART)
          ========================================================================= */}
      {activeTab === 'install' && (
        <main className="flex-1 max-w-5xl mx-auto w-full px-4 sm:px-6 lg:px-8 py-10 space-y-10">
          <div className="space-y-2 border-b subtle-border pb-6">
            <span className="text-blue-500 uppercase font-mono text-xs font-bold tracking-wider px-2.5 py-1 rounded-full bg-blue-600/10 border border-blue-500/20">
              {t('landing.install_tag')}
            </span>
            <h1 className="text-2xl sm:text-3xl font-extrabold tracking-tight text-slate-900 dark:text-white">
              {t('landing.install_title')}
            </h1>
            <p className="text-xs sm:text-sm text-subtle">
              {t('landing.install_subtitle')}
            </p>
          </div>

          {/* Prerequisites & Ports Table */}
          <section className="space-y-4">
            <h2 className="text-base font-bold text-slate-900 dark:text-white flex items-center gap-2">
              <Network className="w-4 h-4 text-blue-500" />
              {t('landing.prereq_title')}
            </h2>
            <p className="text-xs text-subtle leading-relaxed">
              {t('landing.prereq_desc')}
            </p>

            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-3">
              <div className="p-3.5 rounded-xl surface-card border subtle-border">
                <div className="flex items-center justify-between mb-1">
                  <span className="font-mono font-bold text-xs text-blue-500">UDP 500</span>
                  <span className="text-[10px] uppercase font-mono px-1.5 py-0.5 rounded bg-blue-500/10 text-blue-400">IKEv2</span>
                </div>
                <p className="text-[11px] text-subtle">{t('landing.port_ike_desc')}</p>
              </div>

              <div className="p-3.5 rounded-xl surface-card border subtle-border">
                <div className="flex items-center justify-between mb-1">
                  <span className="font-mono font-bold text-xs text-indigo-500">UDP 4500</span>
                  <span className="text-[10px] uppercase font-mono px-1.5 py-0.5 rounded bg-indigo-500/10 text-indigo-400">NAT-T</span>
                </div>
                <p className="text-[11px] text-subtle">{t('landing.port_natt_desc')}</p>
              </div>

              <div className="p-3.5 rounded-xl surface-card border subtle-border">
                <div className="flex items-center justify-between mb-1">
                  <span className="font-mono font-bold text-xs text-emerald-500">TCP 8080</span>
                  <span className="text-[10px] uppercase font-mono px-1.5 py-0.5 rounded bg-emerald-500/10 text-emerald-400">Web & API</span>
                </div>
                <p className="text-[11px] text-subtle">{t('landing.port_api_desc')}</p>
              </div>

              <div className="p-3.5 rounded-xl surface-card border subtle-border">
                <div className="flex items-center justify-between mb-1">
                  <span className="font-mono font-bold text-xs text-amber-500">UDP/TCP 53</span>
                  <span className="text-[10px] uppercase font-mono px-1.5 py-0.5 rounded bg-amber-500/10 text-amber-400">MagicDNS</span>
                </div>
                <p className="text-[11px] text-subtle">{t('landing.port_dns_desc')}</p>
              </div>
            </div>
          </section>

          {/* Deployment Method Tabs (Docker vs Binary) */}
          <section className="space-y-4">
            <div className="flex items-center gap-2 border-b subtle-border pb-2">
              <button
                onClick={() => setInstallMode('docker')}
                className={`px-3 py-1.5 rounded-lg text-xs font-semibold transition-all flex items-center gap-2 ${
                  installMode === 'docker'
                    ? 'bg-blue-600 text-white shadow-xs'
                    : 'text-subtle hover:text-current hover:bg-slate-500/10'
                }`}
              >
                <Layers className="w-3.5 h-3.5" />
                {t('landing.docker_tab')}
              </button>
              <button
                onClick={() => setInstallMode('binary')}
                className={`px-3 py-1.5 rounded-lg text-xs font-semibold transition-all flex items-center gap-2 ${
                  installMode === 'binary'
                    ? 'bg-blue-600 text-white shadow-xs'
                    : 'text-subtle hover:text-current hover:bg-slate-500/10'
                }`}
              >
                <Cpu className="w-3.5 h-3.5" />
                {t('landing.binary_tab')}
              </button>
            </div>

            {installMode === 'docker' ? (
              <div className="space-y-3">
                <p className="text-xs text-subtle">{t('landing.docker_desc')}</p>
                <div className="relative rounded-xl border subtle-border bg-[#0b0f17] text-slate-100 p-4 font-mono text-xs overflow-x-auto shadow-sm">
                  <div className="flex items-center justify-between mb-2 pb-2 border-b border-slate-800 text-slate-400 text-[11px]">
                    <span>docker-compose.yml</span>
                    <CopyButton value={dockerComposeCode} label="docker-compose.yml" />
                  </div>
                  <pre className="text-[11px] leading-relaxed overflow-x-auto">{dockerComposeCode}</pre>
                </div>
                <div className="p-3 rounded-lg surface-card border subtle-border flex items-center justify-between text-xs font-mono">
                  <span>$ docker compose up -d</span>
                  <CopyButton value="docker compose up -d" label="docker compose command" />
                </div>
              </div>
            ) : (
              <div className="space-y-3">
                <p className="text-xs text-subtle">{t('landing.binary_desc')}</p>
                <div className="relative rounded-xl border subtle-border bg-[#0b0f17] text-slate-100 p-4 font-mono text-xs overflow-x-auto shadow-sm">
                  <div className="flex items-center justify-between mb-2 pb-2 border-b border-slate-800 text-slate-400 text-[11px]">
                    <span>Single Binary CLI Execution</span>
                    <CopyButton value={binaryRunCode} label="Single binary commands" />
                  </div>
                  <pre className="text-[11px] leading-relaxed overflow-x-auto">{binaryRunCode}</pre>
                </div>
              </div>
            )}
          </section>

          {/* 5-Step Configuration Walkthrough */}
          <section className="space-y-4">
            <h2 className="text-base font-bold text-slate-900 dark:text-white flex items-center gap-2">
              <CheckCircle2 className="w-4 h-4 text-emerald-500" />
              {t('landing.config_walkthrough_title')}
            </h2>

            <div className="space-y-3">
              <div className="p-4 rounded-xl surface-card border subtle-border flex gap-3.5">
                <div className="w-6 h-6 rounded-full bg-blue-600/15 border border-blue-500/30 text-blue-500 flex items-center justify-center font-mono text-xs font-bold shrink-0">
                  1
                </div>
                <div className="space-y-1">
                  <h3 className="font-bold text-xs text-slate-900 dark:text-white">{t('landing.step1_title')}</h3>
                  <p className="text-xs text-subtle leading-relaxed">{t('landing.step1_desc')}</p>
                </div>
              </div>

              <div className="p-4 rounded-xl surface-card border subtle-border flex gap-3.5">
                <div className="w-6 h-6 rounded-full bg-blue-600/15 border border-blue-500/30 text-blue-500 flex items-center justify-center font-mono text-xs font-bold shrink-0">
                  2
                </div>
                <div className="space-y-1">
                  <h3 className="font-bold text-xs text-slate-900 dark:text-white">{t('landing.step2_title')}</h3>
                  <p className="text-xs text-subtle leading-relaxed">{t('landing.step2_desc')}</p>
                </div>
              </div>

              <div className="p-4 rounded-xl surface-card border subtle-border flex gap-3.5">
                <div className="w-6 h-6 rounded-full bg-blue-600/15 border border-blue-500/30 text-blue-500 flex items-center justify-center font-mono text-xs font-bold shrink-0">
                  3
                </div>
                <div className="space-y-1">
                  <h3 className="font-bold text-xs text-slate-900 dark:text-white">{t('landing.step3_title')}</h3>
                  <p className="text-xs text-subtle leading-relaxed">{t('landing.step3_desc')}</p>
                </div>
              </div>

              <div className="p-4 rounded-xl surface-card border subtle-border flex gap-3.5">
                <div className="w-6 h-6 rounded-full bg-blue-600/15 border border-blue-500/30 text-blue-500 flex items-center justify-center font-mono text-xs font-bold shrink-0">
                  4
                </div>
                <div className="space-y-1">
                  <h3 className="font-bold text-xs text-slate-900 dark:text-white">{t('landing.step4_title')}</h3>
                  <p className="text-xs text-subtle leading-relaxed">{t('landing.step4_desc')}</p>
                </div>
              </div>

              <div className="p-4 rounded-xl surface-card border subtle-border flex gap-3.5">
                <div className="w-6 h-6 rounded-full bg-blue-600/15 border border-blue-500/30 text-blue-500 flex items-center justify-center font-mono text-xs font-bold shrink-0">
                  5
                </div>
                <div className="space-y-1">
                  <h3 className="font-bold text-xs text-slate-900 dark:text-white">{t('landing.step5_title')}</h3>
                  <p className="text-xs text-subtle leading-relaxed">{t('landing.step5_desc')}</p>
                </div>
              </div>
            </div>
          </section>
        </main>
      )}

      {/* =========================================================================
          TAB 3: HƯỚNG DẪN SỬ DỤNG CHO NGƯỜI DÙNG CUỐI (END-USER GUIDE)
          ========================================================================= */}
      {activeTab === 'userguide' && (
        <main className="flex-1 max-w-5xl mx-auto w-full px-4 sm:px-6 lg:px-8 py-10 space-y-10">
          <div className="space-y-2 border-b subtle-border pb-6">
            <span className="text-emerald-500 uppercase font-mono text-xs font-bold tracking-wider px-2.5 py-1 rounded-full bg-emerald-500/10 border border-emerald-500/20">
              {t('landing.user_guide_tag')}
            </span>
            <h1 className="text-2xl sm:text-3xl font-extrabold tracking-tight text-slate-900 dark:text-white">
              {t('landing.user_guide_title')}
            </h1>
            <p className="text-xs sm:text-sm text-subtle">
              {t('landing.user_guide_subtitle')}
            </p>
          </div>

          {/* Why Native VPN is Better Card */}
          <section className="p-5 rounded-2xl surface-card border subtle-border space-y-3 shadow-xs">
            <h2 className="font-bold text-sm text-slate-900 dark:text-white flex items-center gap-2">
              <Zap className="w-4 h-4 text-amber-500" />
              {t('landing.why_native_header')}
            </h2>
            <p className="text-xs text-subtle leading-relaxed">{t('landing.why_native_p1')}</p>
            <p className="text-xs text-subtle leading-relaxed">{t('landing.why_native_p2')}</p>
          </section>

          {/* OS Switcher & Step-by-Step Instructions */}
          <section className="space-y-4">
            <div className="flex flex-wrap items-center gap-1.5 border-b subtle-border pb-3">
              <button
                onClick={() => setActiveOsTab('macos')}
                className={`px-3.5 py-1.5 rounded-lg text-xs font-semibold transition-all flex items-center gap-1.5 ${
                  activeOsTab === 'macos' ? 'bg-blue-600 text-white shadow-xs' : 'text-subtle hover:text-current hover:bg-slate-500/10'
                }`}
              >
                <Laptop className="w-3.5 h-3.5" />
                {t('landing.tab_macos')}
              </button>
              <button
                onClick={() => setActiveOsTab('ios')}
                className={`px-3.5 py-1.5 rounded-lg text-xs font-semibold transition-all flex items-center gap-1.5 ${
                  activeOsTab === 'ios' ? 'bg-blue-600 text-white shadow-xs' : 'text-subtle hover:text-current hover:bg-slate-500/10'
                }`}
              >
                <Smartphone className="w-3.5 h-3.5" />
                {t('landing.tab_ios')}
              </button>
              <button
                onClick={() => setActiveOsTab('windows')}
                className={`px-3.5 py-1.5 rounded-lg text-xs font-semibold transition-all flex items-center gap-1.5 ${
                  activeOsTab === 'windows' ? 'bg-blue-600 text-white shadow-xs' : 'text-subtle hover:text-current hover:bg-slate-500/10'
                }`}
              >
                <Laptop className="w-3.5 h-3.5" />
                {t('landing.tab_windows')}
              </button>
              <button
                onClick={() => setActiveOsTab('linux')}
                className={`px-3.5 py-1.5 rounded-lg text-xs font-semibold transition-all flex items-center gap-1.5 ${
                  activeOsTab === 'linux' ? 'bg-blue-600 text-white shadow-xs' : 'text-subtle hover:text-current hover:bg-slate-500/10'
                }`}
              >
                <Terminal className="w-3.5 h-3.5" />
                {t('landing.tab_linux')}
              </button>
              <button
                onClick={() => setActiveOsTab('android')}
                className={`px-3.5 py-1.5 rounded-lg text-xs font-semibold transition-all flex items-center gap-1.5 ${
                  activeOsTab === 'android' ? 'bg-blue-600 text-white shadow-xs' : 'text-subtle hover:text-current hover:bg-slate-500/10'
                }`}
              >
                <Smartphone className="w-3.5 h-3.5" />
                {t('landing.tab_android')}
              </button>
            </div>

            {/* Platform Guides */}
            {activeOsTab === 'macos' && (
              <div className="space-y-3 animate-fade-in">
                <div className="p-4 rounded-xl surface-card border subtle-border flex gap-3">
                  <span className="font-mono text-xs font-bold text-blue-500 shrink-0">1.</span>
                  <p className="text-xs text-subtle leading-relaxed">{t('landing.macos_step1')}</p>
                </div>
                <div className="p-4 rounded-xl surface-card border subtle-border flex gap-3">
                  <span className="font-mono text-xs font-bold text-blue-500 shrink-0">2.</span>
                  <p className="text-xs text-subtle leading-relaxed">{t('landing.macos_step2')}</p>
                </div>
                <div className="p-4 rounded-xl surface-card border subtle-border flex gap-3">
                  <span className="font-mono text-xs font-bold text-blue-500 shrink-0">3.</span>
                  <p className="text-xs text-subtle leading-relaxed">{t('landing.macos_step3')}</p>
                </div>
                <div className="p-4 rounded-xl surface-card border subtle-border flex gap-3">
                  <span className="font-mono text-xs font-bold text-blue-500 shrink-0">4.</span>
                  <p className="text-xs text-subtle leading-relaxed">{t('landing.macos_step4')}</p>
                </div>
              </div>
            )}

            {activeOsTab === 'ios' && (
              <div className="space-y-3 animate-fade-in">
                <div className="p-4 rounded-xl surface-card border subtle-border flex gap-3">
                  <span className="font-mono text-xs font-bold text-blue-500 shrink-0">1.</span>
                  <p className="text-xs text-subtle leading-relaxed">{t('landing.ios_step1')}</p>
                </div>
                <div className="p-4 rounded-xl surface-card border subtle-border flex gap-3">
                  <span className="font-mono text-xs font-bold text-blue-500 shrink-0">2.</span>
                  <p className="text-xs text-subtle leading-relaxed">{t('landing.ios_step2')}</p>
                </div>
                <div className="p-4 rounded-xl surface-card border subtle-border flex gap-3">
                  <span className="font-mono text-xs font-bold text-blue-500 shrink-0">3.</span>
                  <p className="text-xs text-subtle leading-relaxed">{t('landing.ios_step3')}</p>
                </div>
                <div className="p-4 rounded-xl surface-card border subtle-border flex gap-3">
                  <span className="font-mono text-xs font-bold text-blue-500 shrink-0">4.</span>
                  <p className="text-xs text-subtle leading-relaxed">{t('landing.ios_step4')}</p>
                </div>
              </div>
            )}

            {activeOsTab === 'windows' && (
              <div className="space-y-3 animate-fade-in">
                <div className="p-4 rounded-xl surface-card border subtle-border flex gap-3">
                  <span className="font-mono text-xs font-bold text-blue-500 shrink-0">1.</span>
                  <p className="text-xs text-subtle leading-relaxed">{t('landing.win_step1')}</p>
                </div>
                <div className="p-4 rounded-xl surface-card border subtle-border flex gap-3">
                  <span className="font-mono text-xs font-bold text-blue-500 shrink-0">2.</span>
                  <p className="text-xs text-subtle leading-relaxed">{t('landing.win_step2')}</p>
                </div>
                <div className="p-4 rounded-xl surface-card border subtle-border flex gap-3">
                  <span className="font-mono text-xs font-bold text-blue-500 shrink-0">3.</span>
                  <p className="text-xs text-subtle leading-relaxed">{t('landing.win_step3')}</p>
                </div>
                <div className="p-4 rounded-xl surface-card border subtle-border flex gap-3">
                  <span className="font-mono text-xs font-bold text-blue-500 shrink-0">4.</span>
                  <p className="text-xs text-subtle leading-relaxed">{t('landing.win_step4')}</p>
                </div>
              </div>
            )}

            {activeOsTab === 'linux' && (
              <div className="space-y-3 animate-fade-in">
                <div className="p-4 rounded-xl surface-card border subtle-border flex gap-3">
                  <span className="font-mono text-xs font-bold text-blue-500 shrink-0">1.</span>
                  <p className="text-xs text-subtle leading-relaxed">{t('landing.linux_step1')}</p>
                </div>
                <div className="p-4 rounded-xl surface-card border subtle-border flex gap-3">
                  <span className="font-mono text-xs font-bold text-blue-500 shrink-0">2.</span>
                  <p className="text-xs text-subtle leading-relaxed">{t('landing.linux_step2')}</p>
                </div>
                <div className="p-4 rounded-xl surface-card border subtle-border flex gap-3">
                  <span className="font-mono text-xs font-bold text-blue-500 shrink-0">3.</span>
                  <p className="text-xs text-subtle leading-relaxed">{t('landing.linux_step3')}</p>
                </div>
              </div>
            )}

            {activeOsTab === 'android' && (
              <div className="space-y-3 animate-fade-in">
                <div className="p-4 rounded-xl surface-card border subtle-border flex gap-3">
                  <span className="font-mono text-xs font-bold text-blue-500 shrink-0">1.</span>
                  <p className="text-xs text-subtle leading-relaxed">{t('landing.android_step1')}</p>
                </div>
                <div className="p-4 rounded-xl surface-card border subtle-border flex gap-3">
                  <span className="font-mono text-xs font-bold text-blue-500 shrink-0">2.</span>
                  <p className="text-xs text-subtle leading-relaxed">{t('landing.android_step2')}</p>
                </div>
                <div className="p-4 rounded-xl surface-card border subtle-border flex gap-3">
                  <span className="font-mono text-xs font-bold text-blue-500 shrink-0">3.</span>
                  <p className="text-xs text-subtle leading-relaxed">{t('landing.android_step3')}</p>
                </div>
              </div>
            )}
          </section>
        </main>
      )}

      {/* =========================================================================
          SIGN IN MODAL (ACCESSIBLE FROM ALL TABS)
          ========================================================================= */}
      <Modal
        isOpen={isLoginModalOpen}
        onClose={() => setIsLoginModalOpen(false)}
        title={t('auth.welcome_title')}
        error={errorMessage}
        maxWidth="max-w-sm"
      >
        <form onSubmit={handleLoginSubmit} className="space-y-4 text-xs">
          <div>
            <label className="text-subtle block mb-1 font-medium">{t('auth.email_label')}</label>
            <div className="relative">
              <Mail className="w-3.5 h-3.5 text-subtle absolute left-2.5 top-1/2 -translate-y-1/2" />
              <input
                type="email"
                required
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                placeholder={t('auth.email_placeholder')}
                className="w-full surface-input border subtle-border rounded-lg pl-8 pr-2.5 py-1.5 font-mono text-xs focus:outline-none focus:border-blue-500"
              />
            </div>
          </div>

          <div>
            <label className="text-subtle block mb-1 font-medium">{t('auth.password_label')}</label>
            <div className="relative">
              <Lock className="w-3.5 h-3.5 text-subtle absolute left-2.5 top-1/2 -translate-y-1/2" />
              <input
                type="password"
                required
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="••••••••••••"
                className="w-full surface-input border subtle-border rounded-lg pl-8 pr-2.5 py-1.5 font-mono text-xs focus:outline-none focus:border-blue-500"
              />
            </div>
          </div>

          {/* Quick Auto-Fill for Testing */}
          <button
            type="button"
            onClick={handleFillDefaultAdmin}
            className="w-full py-1.5 px-2 rounded-lg bg-blue-600/10 hover:bg-blue-600/20 text-blue-500 font-mono text-[10px] text-left transition-colors flex items-center justify-between"
          >
            <span>{t('auth.root_admin_badge')}</span>
            <span className="font-semibold">{t('auth.click_to_fill')}</span>
          </button>

          <div className="pt-2 flex gap-2">
            <button
              type="submit"
              disabled={isLoading}
              className="flex-1 bg-blue-600 hover:bg-blue-500 disabled:opacity-50 text-white font-semibold py-2 rounded-lg shadow-sm transition-colors flex items-center justify-center gap-1.5 text-xs"
            >
              {isLoading ? t('auth.signing_in') : t('auth.sign_in_button')}
              <ArrowRight className="w-3.5 h-3.5" />
            </button>
            <button
              type="button"
              onClick={() => setIsLoginModalOpen(false)}
              className="px-3 py-2 border subtle-border rounded-lg hover:bg-slate-500/10 text-xs"
            >
              {t('common.cancel')}
            </button>
          </div>
        </form>
      </Modal>

      {/* =========================================================================
          FOOTER
          ========================================================================= */}
      <footer className="w-full surface-nav border-t subtle-border py-6 px-4 sm:px-6 lg:px-8 text-xs text-subtle">
        <div className="max-w-7xl mx-auto flex flex-col sm:flex-row items-center justify-between gap-4">
          <div className="flex items-center gap-2">
            <SwanLogo className="w-4 h-4" />
            <span className="font-bold text-slate-800 dark:text-slate-200">SwanPanel</span>
            <span>— Enterprise Zero-Trust strongSwan Control Plane</span>
          </div>
          <div className="flex items-center gap-4 text-[11px]">
            <a
              href="https://github.com/thienvu18/swan-panel"
              target="_blank"
              rel="noreferrer"
              className="hover:text-current transition-colors flex items-center gap-1"
            >
              GitHub <ExternalLink className="w-3 h-3" />
            </a>
            <button onClick={() => setActiveTab('install')} className="hover:text-current transition-colors">
              {t('landing.nav_install')}
            </button>
            <button onClick={() => setActiveTab('userguide')} className="hover:text-current transition-colors">
              {t('landing.nav_user_guide')}
            </button>
          </div>
        </div>
      </footer>
    </div>
  );
};
