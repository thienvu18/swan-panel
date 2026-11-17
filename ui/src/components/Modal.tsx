import React, { useEffect, useId, useRef } from 'react';

// Only the topmost dialog handles Escape when confirmation overlays a form.
let nextModalId = 0;
const openModalStack: number[] = [];
import { X, AlertCircle } from 'lucide-react';

interface ModalProps {
  isOpen: boolean;
  onClose: () => void;
  title: string;
  error?: string | null;
  children: React.ReactNode;
  maxWidth?: string;
  dialogRole?: 'dialog' | 'alertdialog';
  descriptionId?: string;
  closeDisabled?: boolean;
}

export const Modal: React.FC<ModalProps> = ({
  isOpen,
  onClose,
  title,
  error,
  children,
  maxWidth = 'max-w-sm',
  dialogRole = 'dialog',
  descriptionId,
  closeDisabled = false,
}) => {
  const modalContainerRef = useRef<HTMLDivElement>(null);
  const titleId = useId();
  const modalId = useRef(0);
  const onCloseRef = useRef(onClose);
  const closeDisabledRef = useRef(closeDisabled);
  if (modalId.current === 0) modalId.current = ++nextModalId;

  useEffect(() => {
    onCloseRef.current = onClose;
    closeDisabledRef.current = closeDisabled;
  }, [onClose, closeDisabled]);

  useEffect(() => {
    if (!isOpen) return;
    const id = modalId.current;
    const opener = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    const modal = modalContainerRef.current;
    const focusableSelector = 'button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), a[href], [tabindex]:not([tabindex="-1"])';
    const focusables = () => Array.from(modal?.querySelectorAll<HTMLElement>(focusableSelector) ?? [])
      .filter((el) => !el.hasAttribute('hidden') && !el.closest('[aria-hidden="true"]'));
    const initial = modal?.querySelector<HTMLElement>('[data-initial-focus]') ?? focusables()[0] ?? modal;
    initial?.focus();

    openModalStack.push(id);
    const handleKeyDown = (e: KeyboardEvent) => {
      if (openModalStack[openModalStack.length - 1] !== id) return;
      if (e.key === 'Escape' && !closeDisabledRef.current) {
        e.preventDefault();
        e.stopImmediatePropagation();
        onCloseRef.current();
      } else if (e.key === 'Tab') {
        const items = focusables();
        if (items.length === 0) {
          e.preventDefault();
          modal?.focus();
          return;
        }
        const first = items[0];
        const last = items[items.length - 1];
        const active = document.activeElement;
        if (e.shiftKey && (active === first || !modal?.contains(active))) {
          e.preventDefault();
          last.focus();
        } else if (!e.shiftKey && (active === last || !modal?.contains(active))) {
          e.preventDefault();
          first.focus();
        }
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => {
      window.removeEventListener('keydown', handleKeyDown);
      const index = openModalStack.lastIndexOf(id);
      if (index >= 0) openModalStack.splice(index, 1);
      if (opener?.isConnected) opener.focus();
    };
  }, [isOpen]);

  if (!isOpen) return null;

  return (
    <div
      className="fixed inset-0 bg-black/60 backdrop-blur-xs z-50 flex items-center justify-center p-4 overscroll-contain"
      onClick={(e) => {
        if (e.target === e.currentTarget && !closeDisabled) {
          onClose();
        }
      }}
      role={dialogRole}
      aria-describedby={descriptionId}
      aria-modal="true"
      aria-labelledby={titleId}
    >
      <div
        ref={modalContainerRef}
        tabIndex={-1}
        className={`surface-card border subtle-border rounded-xl ${maxWidth} w-full p-5 space-y-4 shadow-2xl transition-all`}
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between border-b subtle-border pb-2.5">
          <h3 id={titleId} className="font-bold text-sm text-slate-900 dark:text-white">
            {title}
          </h3>
          <button
            type="button"
            disabled={closeDisabled}
            onClick={onClose}
            className="text-subtle hover:text-current p-1 rounded-md hover:bg-slate-500/10 transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
            aria-label="Close"
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        {error && (
          <div className="p-2.5 rounded-lg bg-red-500/10 border border-red-500/20 text-red-500 text-xs flex items-center gap-2">
            <AlertCircle className="w-4 h-4 shrink-0" />
            <span className="font-medium">{error}</span>
          </div>
        )}

        {children}
      </div>
    </div>
  );
};
