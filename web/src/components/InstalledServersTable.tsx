import { useState } from 'react';
import { useI18n } from '../i18n';
import type { InstalledServer } from '../types';
import {
  Terminal,
  Radio,
  FileText,
  Sliders,
  Trash2,
  Power,
  Copy,
  Check,
} from 'lucide-react';

interface InstalledServersTableProps {
  servers: InstalledServer[];
  onToggleEnabled: (server: InstalledServer) => void;
  onEdit: (server: InstalledServer) => void;
  onViewLogs: (server: InstalledServer) => void;
  onDelete: (server: InstalledServer) => void;
  onBrowseMarketplace: () => void;
  isLoading: boolean;
}

export function InstalledServersTable({
  servers,
  onToggleEnabled,
  onEdit,
  onViewLogs,
  onDelete,
  onBrowseMarketplace,
  isLoading,
}: InstalledServersTableProps) {
  const { t } = useI18n();
  const [copiedId, setCopiedId] = useState<string | null>(null);

  const copyToClipboard = (text: string, id: string) => {
    navigator.clipboard.writeText(text);
    setCopiedId(id);
    setTimeout(() => setCopiedId(null), 2000);
  };

  if (isLoading) {
    return (
      <div className="bg-slate-900/60 border border-slate-800/80 rounded-2xl p-6 space-y-4">
        {[...Array(3)].map((_, i) => (
          <div key={i} className="h-16 bg-slate-800/40 rounded-xl animate-pulse" />
        ))}
      </div>
    );
  }

  if (servers.length === 0) {
    return (
      <div className="bg-slate-900/40 border border-slate-800/60 rounded-2xl p-12 text-center flex flex-col items-center justify-center">
        <div className="w-12 h-12 rounded-2xl bg-slate-800/80 border border-slate-700/60 flex items-center justify-center text-cyan-400 mb-3">
          <Terminal className="w-6 h-6" />
        </div>
        <h3 className="text-sm font-bold text-slate-200">{t('table.noServers')}</h3>
        <p className="text-xs text-slate-400 mt-1 max-w-sm mb-4">
          {t('table.noServersSub')}
        </p>
        <button
          onClick={onBrowseMarketplace}
          className="px-4 py-2 rounded-xl text-xs font-semibold bg-gradient-to-r from-cyan-500 to-blue-600 text-white shadow-md shadow-cyan-500/20 hover:from-cyan-400 hover:to-blue-500 transition-all cursor-pointer"
        >
          {t('table.browseMarketplace')}
        </button>
      </div>
    );
  }

  return (
    <div className="bg-slate-900/60 border border-slate-800/80 rounded-2xl overflow-hidden shadow-xl">
      <div className="overflow-x-auto">
        <table className="w-full text-left border-collapse">
          <thead>
            <tr className="border-b border-slate-800 bg-slate-950/60 text-[11px] font-bold text-slate-400 uppercase tracking-wider">
              <th className="py-3 px-4">{t('table.status')}</th>
              <th className="py-3 px-4">{t('table.name')}</th>
              <th className="py-3 px-4">{t('table.transport')}</th>
              <th className="py-3 px-4">{t('table.commandOrUrl')}</th>
              <th className="py-3 px-4">{t('table.envVars')}</th>
              <th className="py-3 px-4 text-right">{t('table.actions')}</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-800/60 text-xs">
            {servers.map((server) => {
              const isEnabled = server.enabled;
              const hasEnv = server.env && Object.keys(server.env).length > 0;
              const envCount = hasEnv ? Object.keys(server.env!).length : 0;

              return (
                <tr
                  key={server.id}
                  className="hover:bg-slate-800/30 transition-colors group"
                >
                  {/* Status Indicator */}
                  <td className="py-3.5 px-4 whitespace-nowrap">
                    <div className="flex items-center gap-2">
                      <span
                        className={`w-2.5 h-2.5 rounded-full ${
                          isEnabled
                            ? server.status === 'healthy' || !server.status
                              ? 'bg-emerald-400 shadow-sm shadow-emerald-400/50 ring-4 ring-emerald-500/10'
                              : 'bg-amber-400 shadow-sm shadow-amber-400/50'
                            : 'bg-slate-600'
                        }`}
                      />
                      <span
                        className={`text-[11px] font-semibold ${
                          isEnabled ? 'text-emerald-300' : 'text-slate-500'
                        }`}
                      >
                        {isEnabled ? t('table.enabled') : t('table.disabled')}
                      </span>
                    </div>
                  </td>

                  {/* Name & ID */}
                  <td className="py-3.5 px-4">
                    <div className="flex flex-col">
                      <div className="flex items-center gap-1.5 font-bold text-slate-100">
                        <span>{server.name}</span>
                        {server.catalogId && (
                          <span className="text-[10px] font-normal px-1.5 py-0.2 rounded bg-slate-800 text-slate-400 border border-slate-700/50">
                            {server.catalogId}
                          </span>
                        )}
                      </div>
                      <div className="flex items-center gap-1 text-[11px] text-slate-400 font-mono mt-0.5">
                        <span>ID: {server.id}</span>
                        <button
                          onClick={() => copyToClipboard(server.id, server.id)}
                          className="p-0.5 hover:text-slate-200 transition-colors"
                          title="Copy ID"
                        >
                          {copiedId === server.id ? (
                            <Check className="w-3 h-3 text-emerald-400" />
                          ) : (
                            <Copy className="w-3 h-3" />
                          )}
                        </button>
                      </div>
                    </div>
                  </td>

                  {/* Transport */}
                  <td className="py-3.5 px-4 whitespace-nowrap">
                    <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-md text-[10px] font-mono font-semibold bg-slate-800 text-cyan-400 border border-slate-700/60">
                      {server.transportType === 'stdio' ? (
                        <Terminal className="w-2.5 h-2.5" />
                      ) : (
                        <Radio className="w-2.5 h-2.5" />
                      )}
                      {server.transportType}
                    </span>
                  </td>

                  {/* Command / URL */}
                  <td className="py-3.5 px-4 font-mono text-[11px] text-slate-300 max-w-xs truncate">
                    {server.transportType === 'stdio' ? (
                      <span title={`${server.command || ''} ${server.args?.join(' ') || ''}`}>
                        {server.command} {server.args?.join(' ')}
                      </span>
                    ) : (
                      <span title={server.url}>{server.url || '--'}</span>
                    )}
                  </td>

                  {/* Env Vars count */}
                  <td className="py-3.5 px-4 whitespace-nowrap">
                    {hasEnv ? (
                      <span className="text-[10px] px-2 py-0.5 rounded-md bg-slate-800/80 text-slate-300 border border-slate-700/60 font-mono">
                        {envCount} var{envCount > 1 ? 's' : ''}
                      </span>
                    ) : (
                      <span className="text-[11px] text-slate-500">None</span>
                    )}
                  </td>

                  {/* Actions */}
                  <td className="py-3.5 px-4 whitespace-nowrap text-right">
                    <div className="flex items-center justify-end gap-1.5">
                      {/* Toggle Enable/Disable */}
                      <button
                        onClick={() => onToggleEnabled(server)}
                        className={`p-1.5 rounded-lg border transition-colors cursor-pointer ${
                          isEnabled
                            ? 'text-emerald-400 border-emerald-500/30 hover:bg-emerald-500/10'
                            : 'text-slate-500 border-slate-800 hover:bg-slate-800/80 hover:text-slate-300'
                        }`}
                        title={isEnabled ? t('table.disabled') : t('table.enabled')}
                      >
                        <Power className="w-3.5 h-3.5" />
                      </button>

                      {/* View Logs */}
                      <button
                        onClick={() => onViewLogs(server)}
                        className="p-1.5 rounded-lg text-slate-400 hover:text-slate-200 hover:bg-slate-800/80 border border-slate-800 transition-colors cursor-pointer"
                        title={t('table.logs')}
                      >
                        <FileText className="w-3.5 h-3.5" />
                      </button>

                      {/* Edit Config */}
                      <button
                        onClick={() => onEdit(server)}
                        className="p-1.5 rounded-lg text-cyan-400 hover:text-cyan-300 hover:bg-cyan-500/10 border border-cyan-500/30 transition-colors cursor-pointer"
                        title={t('table.edit')}
                      >
                        <Sliders className="w-3.5 h-3.5" />
                      </button>

                      {/* Delete */}
                      <button
                        onClick={() => onDelete(server)}
                        className="p-1.5 rounded-lg text-rose-400 hover:text-rose-300 hover:bg-rose-500/10 border border-rose-500/30 transition-colors cursor-pointer"
                        title={t('table.delete')}
                      >
                        <Trash2 className="w-3.5 h-3.5" />
                      </button>
                    </div>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </div>
  );
}
