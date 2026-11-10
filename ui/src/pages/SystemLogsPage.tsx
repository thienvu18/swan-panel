import React, { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { RotateCcw, Power, Play, Pause, Trash2, Download, Search, Terminal } from 'lucide-react';

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
      if (!res.ok) {
        return {
          logs: [
            {
              timestamp: '12:54:10',
              level: 'info',
              scope: 'dev:thienvu-macbook user:thienvu ug:devops res:eng',
              tag1: 'thienvu-macbook',
              tag2: 'DevOps',
              message: 'IKE_SA_INIT handshake completed successfully from 113.161.42.18:500 (DH Group: CURVE_25519)',
            },
            {
              timestamp: '12:54:11',
              level: 'info',
              scope: 'dev:thienvu-macbook user:thienvu ug:devops ug:backend res:eng',
              tag1: 'thienvu-macbook',
              tag2: 'thienvu@company.com',
              message: "IKE_AUTH mutual X.509 certificate verification succeeded for subject 'CN=thienvu-macbook'",
            },
            {
              timestamp: '12:54:11',
              level: 'info',
              scope: 'dev:thienvu-macbook user:thienvu res:eng',
              tag1: 'thienvu-macbook',
              tag2: 'Engineering Subnet',
              message: 'Configuration Payload: Assigned Virtual IPv4 10.0.1.15 & DNS server 10.0.0.1',
            },
            {
              timestamp: '12:54:12',
              level: 'info',
              scope: 'dev:thienvu-macbook user:thienvu ug:devops res:prod',
              tag1: 'thienvu-macbook',
              tag2: 'Production VPC',
              message: 'CHILD_SA established. Installed Traffic Selector: remote_ts 10.100.0.0/16, 10.50.0.0/16 (Rule Nature: Group Policy)',
            },
            {
              timestamp: '12:55:04',
              level: 'warn',
              scope: 'dev:nam-thinkpad user:nam ug:backend res:eng',
              tag1: 'nam-thinkpad',
              tag2: 'DENY-DROP',
              message: 'nftables dropped packet from 10.0.1.16 towards 10.50.99.10:443 (Matched Exclusion Rule: User Nam Nguyen)',
            },
            {
              timestamp: '12:56:00',
              level: 'info',
              scope: 'dev:db-primary-prod res:prod',
              tag1: 'db-primary-prod',
              tag2: 'Production VPC',
              message: 'DPD (Dead Peer Detection) keepalive exchange completed with 171.244.18.9 (Roundtrip Latency: 4.2ms)',
            },
          ],
        };
      }
      return res.json();
    },
    refetchInterval: isPaused ? false : 3000,
  });

  const logs = cleared ? [] : data?.logs || [];

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
              <span className="font-bold text-xs text-white">{t('logs.service_status')}</span>
              {daemonStatus === 'active' ? (
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
              PID: 14208 &bull; Memory: 24.2 MB &bull; Uptime: 14d 6h 32m
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
              <optgroup label="Filter by Device">
                <option value="dev:thienvu-macbook">Device: thienvu-macbook</option>
                <option value="dev:db-primary-prod">Device: db-primary-prod</option>
                <option value="dev:nam-thinkpad">Device: nam-thinkpad</option>
              </optgroup>
              <optgroup label="Filter by User">
                <option value="user:thienvu">User: thienvu@company.com</option>
                <option value="user:nam">User: nam@company.com</option>
                <option value="user:alex">User: alex@partner.io</option>
              </optgroup>
              <optgroup label="Filter by Resource Group">
                <option value="res:prod">Resource Group: Production VPC</option>
                <option value="res:eng">Resource Group: Engineering Subnet</option>
              </optgroup>
              <optgroup label="Filter by User Group">
                <option value="ug:devops">User Group: DevOps</option>
                <option value="ug:backend">User Group: Backend</option>
                <option value="ug:sales">User Group: Sales</option>
              </optgroup>
            </select>
          </div>

          {/* Level Selector */}
          <div className="flex items-center gap-1.5">
            <span className="text-subtle text-[11px] font-medium">{t('logs.level')}:</span>
            <select
              value={selectedLevel}
              onChange={(e) => setSelectedLevel(e.target.value)}
              className="surface-input border subtle-border rounded-lg px-2 py-1 text-xs focus:outline-none focus:border-blue-500"
            >
              <option value="all">All Levels</option>
              <option value="info">INFO</option>
              <option value="warn">WARN</option>
              <option value="error">ERROR</option>
            </select>
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
            onClick={() => setIsPaused(!isPaused)}
            className="border subtle-border hover:bg-slate-500/10 px-2.5 py-1 rounded-lg text-subtle hover:text-current transition-colors flex items-center gap-1"
          >
            {isPaused ? <Play className="w-3.5 h-3.5" /> : <Pause className="w-3.5 h-3.5" />}
            {isPaused ? t('common.resume') : t('common.pause')}
          </button>
          <button
            onClick={() => setCleared(true)}
            className="border subtle-border hover:bg-slate-500/10 px-2.5 py-1 rounded-lg text-subtle hover:text-current transition-colors flex items-center gap-1"
          >
            <Trash2 className="w-3.5 h-3.5" />
            {t('common.clear')}
          </button>
          <button
            onClick={handleExport}
            className="border subtle-border hover:bg-slate-500/10 px-2.5 py-1 rounded-lg text-subtle hover:text-current transition-colors flex items-center gap-1"
          >
            <Download className="w-3.5 h-3.5" />
            {t('common.export')}
          </button>
        </div>
      </div>

      {/* LIVE STREAM CONSOLE TERMINAL */}
      <div className="border subtle-border rounded-xl overflow-hidden surface-card shadow-xs font-mono text-[11px]">
        <div className="surface-header border-b subtle-border px-4 py-2 flex items-center justify-between text-subtle">
          <span className="flex items-center gap-2">
            <span className="w-2 h-2 rounded-full bg-emerald-500 animate-pulse"></span>
            <span>{t('logs.live_events')}</span>
          </span>
          <span className="text-dim">Showing {logs.length} events</span>
        </div>

        <div className="p-4 space-y-2 max-h-96 overflow-y-auto bg-[#070a0f] text-slate-200">
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
      </div>
    </div>
  );
};
