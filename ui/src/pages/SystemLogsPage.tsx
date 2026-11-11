import React, { useState, useRef, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { RotateCcw, Power, Play, Pause, Trash2, Download, Search, Terminal, ArrowDown } from 'lucide-react';

interface LogEntry {
  timestamp: string;
  level: string;
  scope: string;
  tag1: string;
  tag2: string;
  message: string;
}

export const SystemLogsPage: React.FC = () => {
  const { t } = useTranslation();
  const [selectedScope, setSelectedScope] = useState('all');
  const [selectedLevel, setSelectedLevel] = useState('all');
  const [searchKeyword, setSearchKeyword] = useState('');
  const [isPaused, setIsPaused] = useState(false);
  const [cleared, setCleared] = useState(false);
  const [daemonStatus, setDaemonStatus] = useState<'active' | 'inactive'>('active');
  const [isAutoScroll, setIsAutoScroll] = useState(true);
  const terminalRef = useRef<HTMLDivElement>(null);

  // Query logs
  const { data } = useQuery<{ logs: LogEntry[] }>({
    queryKey: ['system_logs', selectedScope, selectedLevel, searchKeyword],
    queryFn: async () => {
      const params = new URLSearchParams();
      if (selectedScope !== 'all') params.append('scope', selectedScope);
      if (selectedLevel !== 'all') params.append('level', selectedLevel);
      if (searchKeyword) params.append('search', searchKeyword);

      const res = await fetch(`/api/system/logs?${params.toString()}`, {
        headers: { Authorization: `Bearer ${localStorage.getItem('token') || ''}` },
      });
      if (!res.ok) throw new Error('Failed to fetch system logs');
      return res.json();
    },
    refetchInterval: isPaused ? false : 3000,
  });

  // Query real devices and users for scope filter options
  const { data: devicesData } = useQuery<{ devices: any[] }>({
    queryKey: ['devices'],
    queryFn: async () => {
      const res = await fetch('/api/devices', {
        headers: { Authorization: `Bearer ${localStorage.getItem('token') || ''}` },
      });
      if (!res.ok) return { devices: [] };
      return res.json();
    },
  });

  const { data: usersData } = useQuery<{ users: any[] }>({
    queryKey: ['users'],
    queryFn: async () => {
      const res = await fetch('/api/users', {
        headers: { Authorization: `Bearer ${localStorage.getItem('token') || ''}` },
      });
      if (!res.ok) return { users: [] };
      return res.json();
    },
  });

  const enrolledDevices = devicesData?.devices || [];
  const registeredUsers = usersData?.users || [];

  // Query strongSwan daemon status
  const { data: daemonData } = useQuery<{ service: string; status: string; pid: number; memory: string; uptime: string }>({
    queryKey: ['daemon_status'],
    queryFn: async () => {
      const res = await fetch('/api/system/daemon/status', {
        headers: { Authorization: `Bearer ${localStorage.getItem('token') || ''}` },
      });
      if (!res.ok) return { service: 'strongswan', status: 'inactive', pid: 0, memory: 'N/A', uptime: 'N/A' };
      return res.json();
    },
  });

  const logs = cleared ? [] : data?.logs || [];

  useEffect(() => {
    if (isAutoScroll && terminalRef.current) {
      terminalRef.current.scrollTop = terminalRef.current.scrollHeight;
    }
  }, [logs, isAutoScroll]);

  const handleTerminalScroll = () => {
    if (!terminalRef.current) return;
    const { scrollTop, scrollHeight, clientHeight } = terminalRef.current;
    const isAtBottom = scrollHeight - scrollTop - clientHeight < 40;
    if (isAtBottom && !isAutoScroll) {
      setIsAutoScroll(true);
    } else if (!isAtBottom && isAutoScroll) {
      setIsAutoScroll(false);
    }
  };

  const handleDaemonControl = async (action: 'restart' | 'stop') => {
    if (confirm(`Are you sure you want to ${action.toUpperCase()} the VPN Gateway? Active tunnels will be affected.`)) {
      await fetch('/api/system/daemon', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${localStorage.getItem('token') || ''}`,
        },
        body: JSON.stringify({ action }),
      });
      setDaemonStatus(action === 'stop' ? 'inactive' : 'active');
      alert(`Service action ${action.toUpperCase()} executed successfully.`);
    }
  };

  const handleExport = () => {
    const content = logs
      .map((l) => `[${l.timestamp}] [${l.level.toUpperCase()}] [${l.tag1}] [${l.tag2}] ${l.message}`)
      .join('\n');
    const blob = new Blob([content], { type: 'text/plain' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `strongswan-audit-${new Date().toISOString().slice(0, 10)}.log`;
    a.click();
    URL.revokeObjectURL(url);
  };

  return (
    <div className="space-y-4">
      {/* DAEMON CONTROL CARD */}
      <div className="border subtle-border rounded-xl p-4 surface-card flex flex-wrap items-center justify-between gap-3 shadow-xs">
        <div className="flex items-center gap-3">
          <div className="w-8 h-8 rounded-lg bg-emerald-500/10 text-emerald-500 border border-emerald-500/20 flex items-center justify-center font-mono">
            <Terminal className="w-4 h-4" />
          </div>
          <div>
            <div className="flex items-center gap-2">
              <span className="font-bold text-xs text-slate-900 dark:text-white">{t('logs.service_status')}</span>
              {(daemonData?.status || daemonStatus) === 'active' ? (
                <span className="bg-emerald-500/10 text-emerald-500 border border-emerald-500/20 px-2 py-0.5 rounded-md text-[10px] font-mono font-medium">
                  Active (Running)
                </span>
              ) : (
                <span className="bg-rose-500/10 text-rose-500 border border-rose-500/20 px-2 py-0.5 rounded-md text-[10px] font-mono font-medium">
                  Stopped
                </span>
              )}
            </div>
            <div className="text-[11px] text-dim font-mono">
              PID: {daemonData?.pid ?? '...'} &bull; Memory: {daemonData?.memory || '...'} &bull; Uptime: {daemonData?.uptime || '...'}
            </div>
          </div>
        </div>

        <div className="flex items-center gap-2">
          <button
            onClick={() => handleDaemonControl('restart')}
            className="bg-blue-600 hover:bg-blue-500 text-white font-medium px-3 py-1.5 rounded-lg transition-colors flex items-center gap-1.5 shadow-xs text-xs"
          >
            <RotateCcw className="w-3.5 h-3.5" />
            {t('common.restart')}
          </button>
          <button
            onClick={() => handleDaemonControl('stop')}
            className="border border-rose-500/30 text-rose-400 bg-rose-500/10 hover:bg-rose-500/20 font-medium px-3 py-1.5 rounded-lg transition-colors flex items-center gap-1.5 text-xs"
          >
            <Power className="w-3.5 h-3.5" />
            {t('common.stop')}
          </button>
        </div>
      </div>

      {/* CONTEXTUAL LOG FILTER BAR */}
      <div className="border subtle-border rounded-xl p-3 surface-card flex flex-wrap items-center justify-between gap-3 shadow-xs text-xs">
        <div className="flex flex-wrap items-center gap-2">
          {/* Scope Selector */}
          <div className="flex items-center gap-1.5">
            <span className="text-subtle text-[11px] font-medium">{t('logs.filter_scope')}:</span>
            <select
              value={selectedScope}
              onChange={(e) => setSelectedScope(e.target.value)}
              className="surface-input border subtle-border rounded-lg px-2.5 py-1 text-xs focus:outline-none focus:border-blue-500"
            >
              <option value="all">{t('logs.all_logs')}</option>
              {enrolledDevices.length > 0 && (
                <optgroup label="Filter by Device">
                  {enrolledDevices.map((d) => (
                    <option key={d.id} value={`dev:${d.hostname}`}>
                      Device: {d.hostname}
                    </option>
                  ))}
                </optgroup>
              )}
              {registeredUsers.length > 0 && (
                <optgroup label="Filter by User">
                  {registeredUsers.map((u) => (
                    <option key={u.id} value={`user:${u.email}`}>
                      User: {u.name || u.email} ({u.email})
                    </option>
                  ))}
                </optgroup>
              )}
            </select>
          </div>

          {/* Level Filter Chips */}
          <div className="flex items-center gap-1">
            {(['all', 'info', 'warn', 'error'] as const).map((lvl) => {
              const isSelected = selectedLevel === lvl;
              return (
                <button
                  key={lvl}
                  type="button"
                  onClick={() => setSelectedLevel(lvl)}
                  className={`px-2 py-0.5 rounded-md border subtle-border transition-colors uppercase font-mono text-[10px] ${
                    isSelected
                      ? lvl === 'error'
                        ? 'bg-rose-500/20 border-rose-500/40 text-rose-400 font-semibold'
                        : lvl === 'warn'
                        ? 'bg-amber-500/20 border-amber-500/40 text-amber-400 font-semibold'
                        : 'bg-blue-600/20 border-blue-500/40 text-blue-400 font-semibold'
                      : 'text-subtle hover:text-current hover:bg-slate-500/10'
                  }`}
                >
                  {lvl}
                </button>
              );
            })}
          </div>

          {/* Keyword Search */}
          <div className="relative">
            <input
              type="text"
              value={searchKeyword}
              onChange={(e) => setSearchKeyword(e.target.value)}
              placeholder={t('logs.search_placeholder')}
              className="surface-input border subtle-border rounded-lg pl-7 pr-2.5 py-1 text-xs w-48 focus:outline-none focus:border-blue-500"
            />
            <Search className="w-3.5 h-3.5 text-dim absolute left-2 top-1.5" />
          </div>
        </div>

        {/* Live Stream Actions */}
        <div className="flex items-center gap-1.5">
          <button
            type="button"
            onClick={() => setIsPaused(!isPaused)}
            className="border subtle-border hover:bg-slate-500/10 px-2.5 py-1 rounded-lg text-subtle hover:text-current transition-colors flex items-center gap-1"
          >
            {isPaused ? <Play className="w-3.5 h-3.5" /> : <Pause className="w-3.5 h-3.5" />}
            {isPaused ? t('common.resume') : t('common.pause')}
          </button>
          <button
            type="button"
            onClick={() => setCleared(true)}
            className="border subtle-border hover:bg-slate-500/10 px-2.5 py-1 rounded-lg text-subtle hover:text-current transition-colors flex items-center gap-1"
          >
            <Trash2 className="w-3.5 h-3.5" />
            {t('common.clear')}
          </button>
          <button
            type="button"
            onClick={handleExport}
            className="border subtle-border hover:bg-slate-500/10 px-2.5 py-1 rounded-lg text-subtle hover:text-current transition-colors flex items-center gap-1"
          >
            <Download className="w-3.5 h-3.5" />
            {t('common.export')}
          </button>
        </div>
      </div>

      {/* LIVE STREAM CONSOLE TERMINAL */}
      <div className="relative border subtle-border rounded-xl overflow-hidden surface-card shadow-xs font-mono text-[11px]">
        <div className="surface-header border-b subtle-border px-4 py-2 flex items-center justify-between text-subtle">
          <span className="flex items-center gap-2">
            <span className="w-2 h-2 rounded-full bg-emerald-500 animate-pulse"></span>
            <span>{t('logs.live_events')}</span>
          </span>
          <span className="text-dim">Showing {logs.length} events</span>
        </div>

        <div
          ref={terminalRef}
          onScroll={handleTerminalScroll}
          className="p-4 space-y-2 max-h-96 overflow-y-auto bg-[#070a0f] text-slate-200"
        >
          {logs.length === 0 ? (
            <div className="text-dim italic">No events matching the current filter.</div>
          ) : (
            logs.map((l, idx) => (
              <div key={idx} className="flex items-start gap-2.5">
                <span className="text-dim shrink-0">{l.timestamp}</span>
                <span
                  className={`font-semibold shrink-0 uppercase ${
                    l.level === 'warn' ? 'text-amber-400' : l.level === 'error' ? 'text-rose-400' : 'text-blue-400'
                  }`}
                >
                  [{l.level}]
                </span>
                {l.tag1 && (
                  <span className="bg-blue-950/60 text-blue-300 border border-blue-800/40 px-1 py-0.2 rounded text-[10px] shrink-0 font-sans">
                    {l.tag1}
                  </span>
                )}
                {l.tag2 && (
                  <span
                    className={`px-1 py-0.2 rounded text-[10px] shrink-0 font-sans ${
                      l.tag2.includes('DENY')
                        ? 'bg-rose-950/60 text-rose-300 border border-rose-800/40'
                        : 'bg-slate-800 text-slate-300'
                    }`}
                  >
                    {l.tag2}
                  </span>
                )}
                <span className="text-slate-300 break-all">{l.message}</span>
              </div>
            ))
          )}
        </div>

        {/* Floating Auto-Scroll Resume Button */}
        {!isAutoScroll && (
          <button
            type="button"
            onClick={() => {
              setIsAutoScroll(true);
              if (terminalRef.current) {
                terminalRef.current.scrollTop = terminalRef.current.scrollHeight;
              }
            }}
            className="absolute bottom-3 right-4 bg-blue-600 hover:bg-blue-500 text-white shadow-lg px-3 py-1 rounded-full text-xs font-sans flex items-center gap-1.5 transition-all animate-bounce z-10"
          >
            <ArrowDown className="w-3.5 h-3.5" />
            Auto-scroll paused (Click to resume)
          </button>
        )}
      </div>
    </div>
  );
};
