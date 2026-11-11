import React, { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Mail, Lock, ArrowRight, AlertCircle, Globe, Shield } from 'lucide-react';
import { SwanLogo } from '../components/SwanLogo';

interface LoginPageProps {
  onLoginSuccess: (token: string, user: any) => void;
}

export const LoginPage: React.FC<LoginPageProps> = ({ onLoginSuccess }) => {
  const { t, i18n } = useTranslation();
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [isLoading, setIsLoading] = useState(false);
  const [errorMessage, setErrorMessage] = useState('');

  const toggleLanguage = () => {
    const nextLang = i18n.language.startsWith('vi') ? 'en' : 'vi';
    i18n.changeLanguage(nextLang);
  };

  const handleFillDefaultAdmin = () => {
    setEmail('admin@swanpanel.internal');
    setPassword('admin123456');
    setErrorMessage('');
  };

  const handleSubmit = async (e: React.FormEvent) => {
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
            <h1 className="text-lg font-bold tracking-tight text-slate-900 dark:text-white">{t('auth.welcome_title')}</h1>
            <p className="text-xs text-subtle">{t('auth.welcome_subtitle')}</p>
          </div>

          {/* Error Alert */}
          {errorMessage && (
            <div className="p-3 rounded-lg border border-red-500/30 bg-red-500/10 text-red-400 text-xs flex items-center gap-2">
              <AlertCircle className="w-4 h-4 shrink-0" />
              <span>{errorMessage}</span>
            </div>
          )}

          {/* Login Form */}
          <form onSubmit={handleSubmit} className="space-y-4">
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
                  type="password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  placeholder={t('auth.password_placeholder')}
                  required
                  className="w-full surface-input border subtle-border rounded-lg pl-9 pr-3 py-2 text-xs text-slate-900 dark:text-white placeholder-slate-400 focus:outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500 transition-colors"
                />
              </div>
            </div>

            <button
              type="submit"
              disabled={isLoading}
              className="w-full flex items-center justify-center gap-2 py-2.5 px-4 rounded-lg bg-blue-600 hover:bg-blue-500 text-white font-semibold text-xs transition-colors disabled:opacity-50 disabled:cursor-not-allowed shadow-md shadow-blue-900/20"
            >
              <span>{isLoading ? t('auth.signing_in') : t('auth.sign_in_button')}</span>
              {!isLoading && <ArrowRight className="w-3.5 h-3.5" />}
            </button>
          </form>

          {/* Quick Auto-Fill Default Admin Box */}
          <div className="pt-2 border-t subtle-border">
            <button
              type="button"
              onClick={handleFillDefaultAdmin}
              className="w-full text-left p-2.5 rounded-lg border subtle-border surface-header hover:bg-slate-800/80 transition-colors group cursor-pointer"
            >
              <div className="flex items-center justify-between text-[11px] font-semibold text-slate-300 mb-1">
                <div className="flex items-center gap-1.5">
                  <Shield className="w-3.5 h-3.5 text-blue-400" />
                  <span>{t('auth.root_admin_badge')}</span>
                </div>
                <span className="text-[10px] text-blue-400 font-mono group-hover:underline">
                  {t('auth.click_to_fill')}
                </span>
              </div>
              <p className="text-[10px] text-subtle font-mono">{t('auth.default_hint')}</p>
            </button>
          </div>
        </div>
      </main>

      {/* Footer */}
      <footer className="w-full max-w-5xl py-3 text-center text-[11px] text-dim">
        <span>SwanPanel &bull; Zero-Trust strongSwan Control Plane &bull; Single Binary Delivery</span>
      </footer>
    </div>
  );
};
