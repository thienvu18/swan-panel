import React, { useRef } from 'react';

interface AdvancedJSONEditorProps {
  value: string;
  onChange: (value: string) => void;
  label: string;
  error?: string;
}

const tokenPattern = /("(?:\\.|[^"\\])*"(?=\s*:))|("(?:\\.|[^"\\])*")|(-?\b\d+(?:\.\d+)?(?:[eE][+-]?\d+)?\b)|(\b(?:true|false|null)\b)/g;

/** Read-only highlighted layer under a native editable textarea, keeping native selection and accessibility. */
export const AdvancedJSONEditor: React.FC<AdvancedJSONEditorProps> = ({ value, onChange, label, error }) => {
  const highlightRef = useRef<HTMLPreElement>(null);
  const numbersRef = useRef<HTMLDivElement>(null);
  const lines = value.split('\n').length;
  const parts: React.ReactNode[] = [];
  let last = 0;
  for (const match of value.matchAll(tokenPattern)) {
    if (match.index > last) parts.push(value.slice(last, match.index));
    const color = match[1] ? 'text-accent' : match[2] ? 'text-status-success'
      : match[3] ? 'text-status-warning' : 'text-status-danger';
    parts.push(<span key={match.index} className={color}>{match[0]}</span>);
    last = match.index + match[0].length;
  }
  parts.push(value.slice(last));

  return (
    <div className={`flex min-w-0 overflow-hidden rounded-lg border surface-input ${error ? 'border-rose-500/70' : 'subtle-border'}`}>
      <div ref={numbersRef} aria-hidden="true"
        className="w-12 shrink-0 border-r subtle-border surface-header py-4 pr-3 text-right text-xs leading-6 text-dim font-mono overflow-hidden select-none">
        {Array.from({ length: lines }, (_, i) => <div key={i}>{i + 1}</div>)}
      </div>
      <div className="relative min-w-0 flex-1 h-[26rem]">
        <pre ref={highlightRef} aria-hidden="true"
          className="absolute inset-0 m-0 overflow-hidden p-4 font-mono text-[13px] leading-6 whitespace-pre text-heading pointer-events-none tab-[2]">
          {parts}{'\n'}
        </pre>
        <textarea spellCheck={false} aria-label={label} aria-invalid={Boolean(error)}
          aria-describedby={error ? 'advanced-json-error' : undefined}
          value={value} onChange={e => onChange(e.target.value)}
          onScroll={e => {
            if (highlightRef.current) {
              highlightRef.current.scrollTop = e.currentTarget.scrollTop;
              highlightRef.current.scrollLeft = e.currentTarget.scrollLeft;
            }
            if (numbersRef.current) numbersRef.current.scrollTop = e.currentTarget.scrollTop;
          }}
          className="absolute inset-0 block h-full w-full resize-none overflow-auto bg-transparent p-4 font-mono text-[13px] leading-6 text-transparent caret-slate-900 dark:caret-white selection:bg-blue-500/30 focus:outline-none whitespace-pre tab-[2]" />
      </div>
    </div>
  );
};
