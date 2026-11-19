import React from 'react';
import { useTranslation } from 'react-i18next';
import { AlertCircle, RefreshCw } from 'lucide-react';

interface QueryErrorBannerProps {
  onRetry: () => void;
  message?: string;
}

export const QueryErrorBanner: React.FC<QueryErrorBannerProps> = ({ onRetry, message }) => {
  const { t } = useTranslation();
  return (
    <div role="alert" className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-rose-500/30 bg-rose-500/5 p-3 text-xs text-status-danger">
      <span className="flex items-center gap-2">
        <AlertCircle className="h-4 w-4 shrink-0" aria-hidden="true" />
        {message || t('common.load_failed')}
      </span>
      <button type="button" onClick={onRetry} className="inline-flex items-center gap-1.5 rounded-md border border-current/30 px-2.5 py-1.5 font-medium hover:bg-rose-500/10">
        <RefreshCw className="h-3.5 w-3.5" aria-hidden="true" />
        {t('common.retry')}
      </button>
    </div>
  );
};