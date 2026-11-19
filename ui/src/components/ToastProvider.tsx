import React, { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { AlertCircle, CheckCircle2, Info, X } from 'lucide-react';

type NoticeKind = 'success' | 'error' | 'info';
interface Notice { id: number; message: string; kind: NoticeKind }
interface ToastActions {
  show: (message: string, kind?: NoticeKind) => void;
  success: (message: string) => void;
  error: (message: string) => void;
  info: (message: string) => void;
}

const ToastContext = createContext<ToastActions | null>(null);

export const ToastProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const { t } = useTranslation();
  const [notices, setNotices] = useState<Notice[]>([]);
  const sequence = useRef(0);
  const timers = useRef(new Map<number, ReturnType<typeof setTimeout>>());

  const dismiss = useCallback((id: number) => {
    const timer = timers.current.get(id);
    if (timer) clearTimeout(timer);
    timers.current.delete(id);
    setNotices(previous => previous.filter(notice => notice.id !== id));
  }, []);

  const show = useCallback((message: string, kind: NoticeKind = 'success') => {
    const id = ++sequence.current;
    setNotices(previous => [...previous.slice(-2), { id, message, kind }]);
    const timer = setTimeout(() => dismiss(id), kind === 'error' ? 8000 : 4500);
    timers.current.set(id, timer);
  }, [dismiss]);

  useEffect(() => {
    const activeTimers = timers.current;
    return () => { activeTimers.forEach(clearTimeout); activeTimers.clear(); };
  }, []);

  const actions = useMemo<ToastActions>(() => ({
    show,
    success: message => show(message, 'success'),
    error: message => show(message, 'error'),
    info: message => show(message, 'info'),
  }), [show]);

  return (
    <ToastContext.Provider value={actions}>
      {children}
      <div className="fixed bottom-4 right-4 left-4 sm:left-auto sm:w-96 z-[70] space-y-2 pointer-events-none" aria-live="polite" aria-relevant="additions text">
        {notices.map(notice => {
          const Icon = notice.kind === 'success' ? CheckCircle2 : notice.kind === 'error' ? AlertCircle : Info;
          const color = notice.kind === 'success' ? 'text-status-success' : notice.kind === 'error' ? 'text-status-danger' : 'text-accent';
          return (
            <div key={notice.id} role={notice.kind === 'error' ? 'alert' : 'status'}
              className="pointer-events-auto surface-card border subtle-border rounded-xl shadow-lg px-3 py-3 flex items-start gap-3">
              <Icon className={`w-4 h-4 shrink-0 mt-0.5 ${color}`} aria-hidden="true" />
              <p className="text-xs leading-5 flex-1 break-words">{notice.message}</p>
              <button type="button" onClick={() => dismiss(notice.id)}
                aria-label={t('common.close')}
                className="rounded-md p-1 text-subtle hover:text-current focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-500">
                <X className="w-4 h-4" aria-hidden="true" />
              </button>
            </div>
          );
        })}
      </div>
    </ToastContext.Provider>
  );
};

export function useToast(): ToastActions {
  const context = useContext(ToastContext);
  if (!context) throw new Error('ToastProvider is missing');
  return context;
}
