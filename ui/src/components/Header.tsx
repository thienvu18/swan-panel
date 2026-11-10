import React from 'react';
import { Menu, Globe } from 'lucide-react';
import { useTranslation } from 'react-i18next';

interface HeaderProps {
  title: string;
  onOpenMobileMenu: () => void;
  isRealtimeConnected: boolean;
  actionButton?: React.ReactNode;
}

export const Header: React.FC<HeaderProps> = ({
  title,
  onOpenMobileMenu,
  isRealtimeConnected,
  actionButton,
}) => {
  const { i18n } = useTranslation();

  const toggleLanguage = () => {
    const nextLang = i18n.language.startsWith('vi') ? 'en' : 'vi';
    i18n.changeLanguage(nextLang);
  };

  return (
    <header className="h-14 border-b subtle-border px-4 md:px-6 flex items-center justify-between shrink-0 sticky top-0 z-20 surface-nav backdrop-blur">
      <div className="flex items-center gap-3">
        {/* Mobile Hamburger Button */}
        <button
          onClick={onOpenMobileMenu}
          className="p-1.5 rounded-lg border subtle-border hover:bg-slate-500/10 text-subtle hover:text-current md:hidden"
          aria-label="Open navigation menu"
        >
          <Menu className="w-4 h-4" />
        </button>

        <h1 className="font-bold text-sm tracking-tight">{title}</h1>

        {/* Real-time Indicator Pill */}
        <div className="hidden sm:flex items-center gap-1.5 px-2 py-0.5 rounded-full border subtle-border surface-header text-[10px] text-subtle font-mono">
          <span
            className={`w-1.5 h-1.5 rounded-full ${
              isRealtimeConnected ? 'bg-emerald-500 animate-pulse' : 'bg-amber-500'
            }`}
          />
          <span>{isRealtimeConnected ? 'Live' : 'Connecting...'}</span>
        </div>
      </div>

      {/* Header Actions: Language Switcher & Contextual Action Button */}
      <div className="flex items-center gap-2">
        <button
          onClick={toggleLanguage}
          className="inline-flex items-center gap-1.5 px-2 py-1 rounded-md text-xs font-semibold border subtle-border surface-header hover:bg-slate-500/10 text-subtle hover:text-current transition-colors"
          title="Switch Language (English / Tiếng Việt)"
          aria-label={`Switch to ${i18n.language.startsWith('vi') ? 'English' : 'Tiếng Việt'}`}
        >
          <Globe className="w-3.5 h-3.5" />
          <span className="uppercase text-[11px] tracking-wide font-mono">
            {i18n.language.startsWith('vi') ? 'VI' : 'EN'}
          </span>
        </button>
        {actionButton}
      </div>
    </header>
  );
};
