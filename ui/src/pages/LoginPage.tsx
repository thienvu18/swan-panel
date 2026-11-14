import React, { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Mail, Lock, ArrowRight, AlertCircle, CheckCircle2, Eye, EyeOff, Globe, Shield } from 'lucide-react';
import { SwanLogo } from '../components/SwanLogo';
import { oidcStartPath } from '../lib/transportSecurity';

interface LoginPageProps {
  onLoginSuccess: (token: string, user: any) => void;
}

export const LoginPage: React.FC<LoginPageProps> = ({ onLoginSuccess }) => {
  const { t, i18n } = useTranslation();
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [showPassword, setShowPassword] = useState(false);
  const [showSetupToken, setShowSetupToken] = useState(false);
  const [isLoading, setIsLoading] = useState(false);
  const [setupRequired, setSetupRequired] = useState(false);
  const [setupStatusLoaded, setSetupStatusLoaded] = useState(false);
  const [setupToken, setSetupToken] = useState('');
  const [adminName, setAdminName] = useState('');
  const [providers, setProviders] = useState<Array<{ id: string; name: string; provider_type: string }>>([]);
  const [pendingApproval, setPendingApproval] = useState(false);
  const [errorMessage, setErrorMessage] = useState('');
  const [successMessage, setSuccessMessage] = useState('');
  const onLoginSuccessRef = useRef(onLoginSuccess);
  onLoginSuccessRef.current = onLoginSuccess;

  useEffect(() => {
    fetch('/api/auth/setup/status')
      .then(async r => { if (!r.ok) throw new Error('Unable to determine initial setup state'); return r.json(); })
      .then(data => setSetupRequired(Boolean(data.setup_required)))
      .catch(() => setErrorMessage('Unable to check administrator setup. Check SwanPanel server availability.'))
      .finally(() => setSetupStatusLoaded(true));
    fetch('/api/auth/oidc/providers').then(r => r.json()).then(data => setProviders(data.providers || [])).catch(() => {});
    if (window.location.hash.includes('oidc-pending')) {
      setPendingApproval(true);
      window.history.replaceState(null, '', window.location.pathname);
    }
    if (window.location.hash.includes('oidc-complete')) {
      fetch('/api/auth/oidc/exchange', { method: 'POST', credentials: 'same-origin' }).then(async res => {
        const data = await res.json();
        if (!res.ok || !data.token || !data.user) throw new Error(data.error || 'SSO sign-in failed');
        localStorage.setItem('token', data.token);
        localStorage.setItem('user', JSON.stringify(data.user));
        window.history.replaceState(null, '', window.location.pathname);
        onLoginSuccessRef.current(data.token, data.user);
      }).catch(err => setErrorMessage(err.message));
    }
  }, []);

  const toggleLanguage = () => {
    const nextLang = i18n.language.startsWith('vi') ? 'en' : 'vi';
    i18n.changeLanguage(nextLang);
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!setupStatusLoaded || !email.trim() || !password.trim() || (setupRequired && (!setupToken.trim() || !adminName.trim()))) return;

    setIsLoading(true);
    setErrorMessage('');
    setSuccessMessage('');

    try {
      const res = await fetch(setupRequired ? '/api/auth/setup' : '/api/auth/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(setupRequired
          ? { email: email.trim(), password, token: setupToken.trim(), name: adminName.trim() }
          : { email: email.trim(), password }),
      });

      const data = await res.json();
      if (!res.ok) {
        setErrorMessage(data.error || t('auth.invalid_credentials'));
        setIsLoading(false);
        return;
      }

      if (setupRequired && res.ok) {
        setSetupRequired(false);
        setSetupToken('');
        setPassword('');
        setShowPassword(false);
        setShowSetupToken(false);
        setSuccessMessage(t('auth.admin_created'));
      } else if (data.token && data.user) {
        localStorage.setItem('token', data.token);
        localStorage.setItem('user', JSON.stringify(data.user));
        onLoginSuccess(data.token, data.user);
      } else {
        setErrorMessage(t('auth.invalid_credentials'));
      }
    } catch {
      setErrorMessage(t('auth.network_error'));
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <div className="min-h-screen flex flex-col justify-between items-center bg-[#0b0f17] text-[#f8fafc] p-4 sm:p-6 relative">
      {/* Top bar with language switcher */}
      <header className="w-full max-w-5xl flex justify-between items-center py-2">
        <div className="flex items-center gap-2.5">
          <div className="w-8 h-8 rounded-lg surface-header border subtle-border flex items-center justify-center p-1 shadow-xs">
            <SwanLogo className="w-6 h-6" />
          </div>
          <span className="font-bold text-sm tracking-tight">SwanPanel</span>
        </div>

        <button
          onClick={toggleLanguage}
          className="inline-flex items-center gap-1.5 px-2.5 py-1.5 rounded-lg text-xs font-semibold border subtle-border surface-header hover:bg-slate-500/10 text-subtle hover:text-current transition-colors"
          title="Switch Language (EN / VI)"
          aria-label={`Switch to ${i18n.language.startsWith('vi') ? 'English' : 'Tiếng Việt'}`}
        >
          <Globe className="w-3.5 h-3.5" />
          <span className="uppercase text-[11px] tracking-wide font-mono">
            {i18n.language.startsWith('vi') ? 'VI' : 'EN'}
          </span>
        </button>
      </header>

      {/* Main Login Card */}
      <main className="w-full max-w-sm my-auto">
        <div className="surface-card border subtle-border rounded-2xl p-6 sm:p-8 shadow-2xl space-y-6">
          <div className="text-center space-y-1.5">
            <div className="w-12 h-12 mx-auto rounded-xl surface-header border subtle-border flex items-center justify-center p-2 mb-3 shadow-inner">
              <SwanLogo className="w-8 h-8" />
            </div>
            <h1 className="text-lg font-bold tracking-tight text-slate-900 dark:text-white">{setupRequired ? 'Initial Administrator Setup' : t('auth.welcome_title')}</h1>
            <p className="text-xs text-subtle">{setupRequired ? 'Enter the one-time token from your SwanPanel server and create the first admin.' : t('auth.welcome_subtitle')}</p>
          </div>

          {/* Authentication and setup feedback */}
          {successMessage && (
            <div role="status" aria-live="polite" className="p-3 rounded-lg border border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-400 text-xs flex items-center gap-2">
              <CheckCircle2 className="w-4 h-4 shrink-0" aria-hidden="true" />
              <span>{successMessage}</span>
            </div>
          )}
          {errorMessage && (
            <div role="alert" className="p-3 rounded-lg border border-red-500/30 bg-red-500/10 text-red-400 text-xs flex items-center gap-2">
              <AlertCircle className="w-4 h-4 shrink-0" />
              <span>{errorMessage}</span>
            </div>
          )}

          {pendingApproval && <div className="text-xs text-amber-400 border border-amber-500/30 rounded-lg p-3">SSO identity registered. An administrator must approve this account before sign-in.</div>}
          {/* Login or first-run setup form */}
          <form onSubmit={handleSubmit} className="space-y-4">
            {setupRequired && <>
              <label className="text-[11px] font-semibold text-subtle block" htmlFor="setup-token">One-time setup token</label>
              <div className="relative">
                <input
                  id="setup-token"
                  type={showSetupToken ? 'text' : 'password'}
                  autoComplete="off"
                  value={setupToken}
                  onChange={e => setSetupToken(e.target.value)}
                  required
                  placeholder="Read the .setup-token file on the server"
                  className="w-full surface-input border subtle-border rounded-lg pl-3 pr-10 py-2 text-xs"
                />
                <button
                  type="button"
                  onClick={() => setShowSetupToken(value => !value)}
                  aria-label={t(showSetupToken ? 'auth.hide_setup_token' : 'auth.show_setup_token')}
                  aria-pressed={showSetupToken}
                  aria-controls="setup-token"
                  className="absolute right-2 top-1/2 -translate-y-1/2 p-1 rounded text-dim hover:text-current focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-500"
                >
                  {showSetupToken ? <EyeOff className="w-4 h-4" aria-hidden="true" /> : <Eye className="w-4 h-4" aria-hidden="true" />}
                </button>
              </div>
              <label className="text-[11px] font-semibold text-subtle block" htmlFor="admin-name">Administrator name</label>
              <input id="admin-name" value={adminName} onChange={e => setAdminName(e.target.value)} required className="w-full surface-input border subtle-border rounded-lg p-2 text-xs" />
            </>}
            <div className="space-y-1.5">
              <label className="text-[11px] font-semibold text-subtle block" htmlFor="email-input">
                {t('auth.email_label')}
              </label>
              <div className="relative">
                <Mail className="w-4 h-4 text-dim absolute left-3 top-1/2 -translate-y-1/2" />
                <input
                  id="email-input"
                  type="email"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  placeholder={t('auth.email_placeholder')}
                  required
                  autoFocus
                  className="w-full surface-input border subtle-border rounded-lg pl-9 pr-3 py-2 text-xs text-slate-900 dark:text-white placeholder-slate-400 focus:outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500 transition-colors"
                />
              </div>
            </div>

            <div className="space-y-1.5">
              <label className="text-[11px] font-semibold text-subtle block" htmlFor="password-input">
                {t('auth.password_label')}
              </label>
              <div className="relative">
                <Lock className="w-4 h-4 text-dim absolute left-3 top-1/2 -translate-y-1/2" />
                <input
                  id="password-input"
                  type={showPassword ? 'text' : 'password'}
                  autoComplete={setupRequired ? 'new-password' : 'current-password'}
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  placeholder={setupRequired ? 'At least 12 characters' : t('auth.password_placeholder')}
                  minLength={setupRequired ? 12 : undefined}
                  required
                  className="w-full surface-input border subtle-border rounded-lg pl-9 pr-10 py-2 text-xs text-slate-900 dark:text-white placeholder-slate-400 focus:outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500 transition-colors"
                />
                <button
                  type="button"
                  onClick={() => setShowPassword(value => !value)}
                  aria-label={t(showPassword ? 'auth.hide_password' : 'auth.show_password')}
                  aria-pressed={showPassword}
                  aria-controls="password-input"
                  className="absolute right-2 top-1/2 -translate-y-1/2 p-1 rounded text-dim hover:text-current focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-500"
                >
                  {showPassword ? <EyeOff className="w-4 h-4" aria-hidden="true" /> : <Eye className="w-4 h-4" aria-hidden="true" />}
                </button>
              </div>
            </div>

            <button
              type="submit"
              disabled={isLoading || !setupStatusLoaded}
              className="w-full flex items-center justify-center gap-2 py-2.5 px-4 rounded-lg bg-blue-600 hover:bg-blue-500 text-white font-semibold text-xs transition-colors disabled:opacity-50 disabled:cursor-not-allowed shadow-md shadow-blue-900/20"
            >
              <span>{isLoading ? 'Please wait...' : setupRequired ? 'Create administrator' : t('auth.sign_in_button')}</span>
              {!isLoading && <ArrowRight className="w-3.5 h-3.5" />}
            </button>
          </form>

          {setupStatusLoaded && !setupRequired && providers.length > 0 && (
            <div className="pt-3 border-t subtle-border space-y-2">
              <p className="text-[11px] text-subtle text-center">Sign in with your organization</p>
              {providers.map(provider => <a key={provider.id}
                href={oidcStartPath(provider.id)}
                className="block w-full text-center p-2 rounded-lg border subtle-border surface-header hover:bg-slate-500/10 text-xs font-semibold">
                <Shield className="w-3.5 h-3.5 inline-block mr-2" /> {provider.name}
              </a>)}
            </div>
          )}
        </div>
      </main>

      {/* Footer */}
      <footer className="w-full max-w-5xl py-3 text-center text-[11px] text-dim">
        <span>SwanPanel &bull; Zero-Trust strongSwan Control Plane &bull; Single Binary Delivery</span>
      </footer>
    </div>
  );
};
