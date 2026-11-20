import React from 'react';
import type { LucideIcon } from 'lucide-react';
import { useTranslation } from 'react-i18next';

/** On touch layouts, icon-only table controls need a visible key as hover hints are absent. */
export const TableActionLegend: React.FC<{ actions: Array<{ icon: LucideIcon; label: string }> }> = ({ actions }) => {
  const { t } = useTranslation();
  return (
    <div role="note" className="lg:hidden flex flex-wrap items-center gap-x-4 gap-y-2 text-xs text-dim py-2 px-1">
      <span className="font-semibold">{t('common.actions')}:</span>
      {actions.map(({ icon: Icon, label }) => (
        <span key={label} className="inline-flex items-center gap-1.5">
          <Icon className="w-3.5 h-3.5" aria-hidden="true" />
          {label}
        </span>
      ))}
    </div>
  );
};
