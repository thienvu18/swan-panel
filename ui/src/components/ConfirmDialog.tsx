import React, { useId, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { AlertTriangle } from 'lucide-react';
import { Modal } from './Modal';

export interface ConfirmationAction {
  title: string;
  description: string;
  confirmLabel: string;
  onConfirm: () => Promise<unknown>;
  tone?: 'danger' | 'warning';
}

export const ConfirmDialog: React.FC<{
  action: ConfirmationAction | null;
  onClose: () => void;
}> = ({ action, onClose }) => {
  const { t } = useTranslation();
  const descriptionId = useId();
  const [isPending, setIsPending] = useState(false);
  const inFlight = useRef(false);
  const [error, setError] = useState<string | null>(null);

  const dismiss = () => {
    if (inFlight.current) return;
    setError(null);
    onClose();
  };

  const submit = async () => {
    if (!action || inFlight.current) return;
    inFlight.current = true;
    setError(null);
    setIsPending(true);
    try {
      await action.onConfirm();
      onClose();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : t('common.error_occurred'));
    } finally {
      inFlight.current = false;
      setIsPending(false);
    }
  };

  return (
    <Modal
      isOpen={Boolean(action)}
      onClose={dismiss}
      title={action?.title || ''}
      error={error}
      dialogRole="alertdialog"
      descriptionId={descriptionId}
      closeDisabled={isPending}
    >
      <div className="flex items-start gap-3">
        <div className={`rounded-lg p-2 shrink-0 ${action?.tone === 'warning' ? 'bg-amber-500/10 text-amber-500' : 'bg-rose-500/10 text-rose-500'}`}>
          <AlertTriangle className="w-5 h-5" aria-hidden="true" />
        </div>
        <p id={descriptionId} className="text-xs leading-5 text-subtle">{action?.description}</p>
      </div>
      <div className="flex justify-end gap-2 border-t subtle-border pt-3">
        <button type="button" data-initial-focus disabled={isPending} onClick={dismiss}
          className="rounded-lg px-3 py-2 border subtle-border text-xs font-medium hover:bg-slate-500/10 disabled:opacity-50">
          {t('common.cancel')}
        </button>
        <button type="button" disabled={isPending} onClick={() => void submit()}
          className={`rounded-lg px-3 py-2 text-xs font-semibold text-white disabled:opacity-50 ${action?.tone === 'warning' ? 'bg-amber-600 hover:bg-amber-500' : 'bg-rose-600 hover:bg-rose-500'}`}>
          {isPending ? t('common.loading') : action?.confirmLabel}
        </button>
      </div>
    </Modal>
  );
};

/** Confirmation is the only entry point for high-impact mutations. */
export const useConfirmAction = () => {
  const [action, setAction] = useState<ConfirmationAction | null>(null);
  return {
    requestConfirmation: setAction,
    confirmationDialog: <ConfirmDialog action={action} onClose={() => setAction(null)} />,
  };
};
