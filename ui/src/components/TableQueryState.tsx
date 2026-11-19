import React from 'react';
import { AlertCircle, LoaderCircle } from 'lucide-react';
import { useTranslation } from 'react-i18next';

/** Consistent inline feedback instead of showing a misleading empty table during fetch failures. */
export const TableQueryState: React.FC<{
  columns: number;
  error?: boolean;
  onRetry?: () => void;
}> = ({ columns, error = false, onRetry }) => {
  const { t } = useTranslation();
  return (
    <tr>
      <td colSpan={columns} className="!py-9 px-4 text-center">
        <div role={error ? 'alert' : 'status'} className={`flex flex-col items-center gap-2 ${error ? 'text-status-danger' : 'text-subtle'}`}>
          {error ? <AlertCircle className="w-5 h-5" /> : <LoaderCircle className="w-5 h-5 animate-spin" />}
          <p className="text-sm">{t(error ? 'common.load_failed' : 'common.loading')}</p>
          {error && onRetry && <button type="button" onClick={onRetry}
            className="rounded-lg border subtle-border px-3 py-2 text-xs font-semibold text-accent hover:bg-blue-500/10">
            {t('common.retry')}
          </button>}
        </div>
      </td>
    </tr>
  );
};
