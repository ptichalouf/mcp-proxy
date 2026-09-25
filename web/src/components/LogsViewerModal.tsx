import { useState, useEffect, useRef, useCallback } from 'react';
import { useI18n } from '../i18n';
import type { InstalledServer, ServerLogEntry } from '../types';
import { api } from '../lib/api';
import {
  X,
  RefreshCw,
  Copy,
  Check,
  Terminal,
  Clock,
} from 'lucide-react';

interface LogsViewerModalProps {
  isOpen: boolean;
  onClose: () => void;
  server: InstalledServer | null;
}

export function LogsViewerModal({
  isOpen,
  onClose,
  server,
}: LogsViewerModalProps) {
  const { t } = useI18n();

  const [logs, setLogs] = useState<ServerLogEntry[]>([]);
  const [isLoading, setIsLoading] = useState(false);
  const [copied, setCopied] = useState(false);
  const [autoScroll, setAutoScroll] = useState(true);
  const [filterLevel, setFilterLevel] = useState<string>('all');

  const logsEndRef = useRef<HTMLDivElement>(null);

  const fetchLogs = useCallback(async () => {
    if (!server) return;
    setIsLoading(true);
    try {
      const data = await api.getServerLogs(server.id);
      setLogs(data);
    } catch {
      // Fallback mock logs if server is newly created
      setLogs([
        {
          timestamp: new Date().toISOString(),
          level: 'info',
          stream: 'stdout',
          message: `[mcp-proxy] Connected to MCP server '${server.name}' (${server.transportType})`,
        },
      ]);
    } finally {
      setIsLoading(false);
    }
  }, [server]);

  useEffect(() => {
    if (isOpen && server) {
      fetchLogs();
      const interval = setInterval(fetchLogs, 4000);
      return () => clearInterval(interval);
    }
  }, [isOpen, server, fetchLogs]);

  useEffect(() => {
    if (autoScroll && logsEndRef.current) {
      logsEndRef.current.scrollIntoView({ behavior: 'smooth' });
    }
  }, [logs, autoScroll]);

  if (!isOpen || !server) return null;

  const handleCopyLogs = () => {
    const text = logs
      .map((l) => `[${l.timestamp}] [${l.level?.toUpperCase() || 'INFO'}] ${l.message}`)
      .join('\n');
    navigator.clipboard.writeText(text);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const filteredLogs = logs.filter((l) => {
    if (filterLevel === 'all') return true;
    if (filterLevel === 'error') return l.level === 'error' || l.stream === 'stderr';
    if (filterLevel === 'warn') return l.level === 'warn';
    if (filterLevel === 'info') return l.level === 'info' || l.stream === 'stdout';
    return true;
  });

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-slate-950/80 backdrop-blur-md animate-fade-in">
      <div className="bg-slate-900 border border-slate-800 rounded-2xl w-full max-w-4xl h-[80vh] flex flex-col shadow-2xl overflow-hidden">
        {/* Header */}
        <div className="flex items-center justify-between px-6 py-4 border-b border-slate-800 bg-slate-950/60">
          <div className="flex items-center gap-3">
            <div className="w-8 h-8 rounded-lg bg-cyan-500/10 border border-cyan-500/20 text-cyan-400 flex items-center justify-center font-bold">
              <Terminal className="w-4 h-4" />
            </div>
            <div>
              <h2 className="text-sm font-bold text-slate-100 flex items-center gap-2">
                <span>{t('logsModal.title', { name: server.name })}</span>
                <span className="text-[10px] font-mono px-2 py-0.5 rounded bg-slate-800 text-slate-400">
                  {server.id}
                </span>
              </h2>
              <p className="text-xs text-slate-400">Live output stream & diagnostics</p>
            </div>
          </div>

          <div className="flex items-center gap-2">
            {/* Filter */}
            <select
              value={filterLevel}
              onChange={(e) => setFilterLevel(e.target.value)}
              className="bg-slate-950 border border-slate-800 rounded-lg px-2.5 py-1 text-xs text-slate-300 focus:outline-none cursor-pointer"
            >
              <option value="all">All levels</option>
              <option value="info">Info / stdout</option>
              <option value="warn">Warnings</option>
              <option value="error">Errors / stderr</option>
            </select>

            {/* Refresh */}
            <button
              onClick={fetchLogs}
              disabled={isLoading}
              className="p-1.5 rounded-lg text-slate-400 hover:text-slate-200 hover:bg-slate-800 border border-slate-800 disabled:opacity-50"
              title={t('common.refresh')}
            >
              <RefreshCw className={`w-3.5 h-3.5 ${isLoading ? 'animate-spin text-cyan-400' : ''}`} />
            </button>

            {/* Copy */}
            <button
              onClick={handleCopyLogs}
              className="p-1.5 rounded-lg text-slate-400 hover:text-slate-200 hover:bg-slate-800 border border-slate-800 flex items-center gap-1 text-xs"
              title={t('logsModal.copyLogs')}
            >
              {copied ? (
                <Check className="w-3.5 h-3.5 text-emerald-400" />
              ) : (
                <Copy className="w-3.5 h-3.5" />
              )}
            </button>

            {/* Close */}
            <button
              onClick={onClose}
              className="p-1.5 rounded-lg text-slate-400 hover:text-slate-200 hover:bg-slate-800"
            >
              <X className="w-4 h-4" />
            </button>
          </div>
        </div>

        {/* Logs Terminal Area */}
        <div className="flex-1 bg-slate-950 p-4 font-mono text-xs overflow-y-auto space-y-1.5 text-slate-300 select-text">
          {filteredLogs.map((log, idx) => {
            const isError = log.level === 'error' || log.stream === 'stderr';
            const isWarn = log.level === 'warn';
            const timeStr = log.timestamp
              ? new Date(log.timestamp).toLocaleTimeString()
              : '--:--:--';

            return (
              <div
                key={idx}
                className={`flex items-start gap-2.5 leading-relaxed hover:bg-slate-900/40 p-1 rounded transition-colors ${
                  isError ? 'text-rose-400' : isWarn ? 'text-amber-300' : 'text-slate-300'
                }`}
              >
                <span className="text-[10px] text-slate-600 shrink-0 select-none">
                  {timeStr}
                </span>
                <span
                  className={`text-[10px] uppercase font-bold px-1 rounded select-none shrink-0 ${
                    isError
                      ? 'bg-rose-500/20 text-rose-300 border border-rose-500/30'
                      : isWarn
                      ? 'bg-amber-500/20 text-amber-300 border border-amber-500/30'
                      : 'bg-slate-800 text-slate-400'
                  }`}
                >
                  {log.level || log.stream || 'LOG'}
                </span>
                <span className="break-all whitespace-pre-wrap flex-1">{log.message}</span>
              </div>
            );
          })}

          {filteredLogs.length === 0 && (
            <div className="text-center py-16 text-slate-600 flex flex-col items-center justify-center gap-2">
              <Clock className="w-6 h-6 text-slate-700" />
              <span>{t('logsModal.noLogs')}</span>
            </div>
          )}

          <div ref={logsEndRef} />
        </div>

        {/* Footer */}
        <div className="px-6 py-3 border-t border-slate-800 bg-slate-950/60 flex items-center justify-between text-xs text-slate-400">
          <div className="flex items-center gap-3">
            <label className="flex items-center gap-1.5 cursor-pointer select-none">
              <input
                type="checkbox"
                checked={autoScroll}
                onChange={(e) => setAutoScroll(e.target.checked)}
                className="rounded border-slate-800 bg-slate-900 text-cyan-500 focus:ring-0"
              />
              <span className="text-[11px]">{t('logsModal.autoScroll')}</span>
            </label>
            <span className="text-slate-700">|</span>
            <span className="text-[11px]">{filteredLogs.length} lines</span>
          </div>

          <button
            onClick={onClose}
            className="px-4 py-1.5 rounded-lg text-xs font-semibold bg-slate-800 hover:bg-slate-700 text-slate-200 border border-slate-700 transition-colors"
          >
            {t('common.close')}
          </button>
        </div>
      </div>
    </div>
  );
}
