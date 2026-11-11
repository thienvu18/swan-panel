import React, { useEffect, useRef } from 'react';
import { X, AlertCircle } from 'lucide-react';

interface ModalProps {
  isOpen: boolean;
  onClose: () => void;
  title: string;
  error?: string | null;
  children: React.ReactNode;
  maxWidth?: string;
}

export const Modal: React.FC<ModalProps> = ({
  isOpen,
  onClose,
  title,
  error,
  children,
  maxWidth = 'max-w-sm',
}) => {
  const modalContainerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!isOpen) return;

    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.stopPropagation();
        onClose();
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [isOpen, onClose]);

  if (!isOpen) return null;

  return (
    <div
      className="fixed inset-0 bg-black/60 backdrop-blur-xs z-50 flex items-center justify-center p-4 overscroll-contain"
      onClick={(e) => {
        if (e.target === e.currentTarget) {
          onClose();
        }
      }}
      role="dialog"
      aria-modal="true"
      aria-labelledby="modal-title"
    >
      <div
        ref={modalContainerRef}
        className={`surface-card border subtle-border rounded-xl ${maxWidth} w-full p-5 space-y-4 shadow-2xl transition-all`}
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between border-b subtle-border pb-2.5">
          <h3 id="modal-title" className="font-bold text-sm text-slate-900 dark:text-white">
            {title}
          </h3>
          <button
            onClick={onClose}
            className="text-subtle hover:text-current p-1 rounded-md hover:bg-slate-500/10 transition-colors"
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
