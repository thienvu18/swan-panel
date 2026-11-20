import React from 'react';
import { Menu } from 'lucide-react';
import { useTranslation } from 'react-i18next';

interface HeaderProps {
  title: string;
  onOpenMobileMenu: () => void;
  actionButton?: React.ReactNode;
}

export const Header: React.FC<HeaderProps> = ({
  title,
  onOpenMobileMenu,
  actionButton,
}) => {
  const { t } = useTranslation();
  return (
    <header className="min-h-[60px] py-2 border-b subtle-border px-4 md:px-6 lg:px-8 flex flex-wrap sm:flex-nowrap items-center justify-between gap-2 shrink-0 sticky top-0 z-20 surface-nav backdrop-blur">
      <div className="flex items-center gap-3 min-w-0 flex-1">
        {/* Mobile Hamburger Button */}
        <button
          onClick={onOpenMobileMenu}
          className="p-1.5 rounded-lg border subtle-border hover:bg-slate-500/10 text-subtle hover:text-current md:hidden"
          aria-label={t('nav.open_navigation')}
        >
          <Menu className="w-4 h-4" />
        </button>

        <h1 className="font-bold text-base tracking-tight truncate">{title}</h1>
      </div>

      {/* Header Contextual Action Button */}
      {actionButton && <div className="flex items-center gap-2 ml-auto shrink-0">{actionButton}</div>}
    </header>
  );
};
