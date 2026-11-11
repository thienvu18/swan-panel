import React from 'react';
import { LucideIcon } from 'lucide-react';

interface EmptyStateProps {
  icon: LucideIcon;
  title: string;
  description: string;
  actionLabel?: string;
  onAction?: () => void;
  actionIcon?: LucideIcon;
  secondaryActionLabel?: string;
  onSecondaryAction?: () => void;
  className?: string;
}

export const EmptyState: React.FC<EmptyStateProps> = ({
  icon: Icon,
  title,
  description,
  actionLabel,
  onAction,
  actionIcon: ActionIcon,
  secondaryActionLabel,
  onSecondaryAction,
  className = '',
}) => {
  return (
    <div
      className={`flex flex-col items-center justify-center p-8 text-center surface-card border subtle-border rounded-xl ${className}`}
    >
      <div className="w-12 h-12 rounded-2xl bg-blue-500/10 text-blue-500 flex items-center justify-center mb-3 shadow-xs">
        <Icon className="w-6 h-6" />
      </div>
      <h3 className="text-sm font-semibold text-slate-900 dark:text-white mb-1 tracking-tight">
        {title}
      </h3>
      <p className="text-xs text-subtle max-w-md mb-4 leading-relaxed font-sans">
        {description}
      </p>

      <div className="flex flex-wrap items-center justify-center gap-2">
        {actionLabel && onAction && (
          <button
            type="button"
            onClick={onAction}
            className="bg-blue-600 hover:bg-blue-500 text-white font-medium px-3.5 py-1.5 rounded-lg transition-colors flex items-center gap-1.5 shadow-xs text-xs"
          >
            {ActionIcon && <ActionIcon className="w-3.5 h-3.5" />}
            <span>{actionLabel}</span>
          </button>
        )}

        {secondaryActionLabel && onSecondaryAction && (
          <button
            type="button"
            onClick={onSecondaryAction}
            className="border subtle-border hover:bg-slate-500/10 text-subtle hover:text-current px-3 py-1.5 rounded-lg transition-colors text-xs font-medium"
          >
            {secondaryActionLabel}
          </button>
        )}
      </div>
    </div>
  );
};
