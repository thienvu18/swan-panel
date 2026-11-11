import React, { useState, useRef, useEffect } from 'react';
import { Copy, Check } from 'lucide-react';
import { useTranslation } from 'react-i18next';

interface CopyButtonProps {
  value: string;
  label?: string;
  className?: string;
  iconSize?: number;
}

export const CopyButton: React.FC<CopyButtonProps> = ({
  value,
  label,
  className = '',
  iconSize = 14,
}) => {
  const { t } = useTranslation();
  const [copied, setCopied] = useState(false);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    return () => {
      if (timerRef.current) clearTimeout(timerRef.current);
    };
  }, []);

  const handleCopy = async (e: React.MouseEvent) => {
    e.stopPropagation();
    if (!value) return;

    try {
      await navigator.clipboard.writeText(value);
      if (timerRef.current) clearTimeout(timerRef.current);
      setCopied(true);
      timerRef.current = setTimeout(() => {
        setCopied(false);
      }, 2000);
    } catch {
      // Fallback if clipboard API is denied
      const textArea = document.createElement('textarea');
      textArea.value = value;
      textArea.style.position = 'fixed';
      textArea.style.opacity = '0';
      document.body.appendChild(textArea);
      textArea.select();
      try {
        document.execCommand('copy');
        setCopied(true);
        if (timerRef.current) clearTimeout(timerRef.current);
        timerRef.current = setTimeout(() => setCopied(false), 2000);
      } catch {
        // Silently fail
      }
      document.body.removeChild(textArea);
    }
  };

  const accessibleName = label
    ? `${t('common.copy') || 'Copy'} ${label}`
    : `${t('common.copy') || 'Copy'} ${value}`;

  return (
    <span className="relative inline-flex items-center">
      <button
        type="button"
        onClick={handleCopy}
        aria-label={accessibleName}
        title={copied ? (t('common.copied') || 'Copied!') : accessibleName}
        className={`p-1 rounded-md transition-colors text-subtle hover:text-current hover:bg-slate-500/10 min-w-[24px] min-h-[24px] inline-flex items-center justify-center ${className}`}
      >
        {copied ? (
          <Check className="text-emerald-500 animate-in zoom-in-50 duration-150" style={{ width: iconSize, height: iconSize }} />
        ) : (
          <Copy className="transition-opacity duration-150" style={{ width: iconSize, height: iconSize }} />
        )}
      </button>

      {/* Screen reader live region */}
      <span className="sr-only" role="status">
        {copied ? (t('common.copied') || 'Copied!') : ''}
      </span>
    </span>
  );
};
