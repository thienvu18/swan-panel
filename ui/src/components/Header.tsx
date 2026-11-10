import React from 'react';
import { Menu } from 'lucide-react';

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
  return (
    <header className="h-14 border-b subtle-border px-4 md:px-6 flex items-center justify-between shrink-0 sticky top-0 z-20 surface-nav backdrop-blur">
      <div className="flex items-center gap-3">
        {/* Mobile Hamburger Button */}
        <button
          onClick={onOpenMobileMenu}
          className="p-1.5 rounded-lg border subtle-border hover:bg-slate-500/10 text-subtle hover:text-current md:hidden"
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

      {/* Contextual Action Button */}
      <div className="flex items-center gap-2">{actionButton}</div>
    </header>
  );
};
