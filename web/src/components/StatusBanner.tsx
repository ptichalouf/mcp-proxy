import { useI18n } from '../i18n';
import type { ProxyHealthSummary, ReloadResult } from '../types';
import { Activity, CheckCircle2, AlertCircle, RefreshCw, Cpu, Radio, ShieldCheck } from 'lucide-react';

interface StatusBannerProps {
  health: ProxyHealthSummary | null;
  onReload: () => void;
  isReloading: boolean;
  lastReloadResult: ReloadResult | null;
}

export function StatusBanner({
  health,
  onReload,
  isReloading,
  lastReloadResult,
}: StatusBannerProps) {
  const { t } = useI18n();

  const isHealthy = health?.status === 'ok' || health?.status === 'ready';

  return (
    <div className="bg-slate-900/60 border border-slate-800/80 rounded-2xl p-4 shadow-xl backdrop-blur-sm">
      <div className="flex flex-col md:flex-row items-start md:items-center justify-between gap-4">
        {/* Left: Health Indicator */}
        <div className="flex items-center gap-3">
          <div
            className={`w-10 h-10 rounded-xl flex items-center justify-center transition-all ${
              isHealthy
                ? 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/20 shadow-lg shadow-emerald-500/10'
                : 'bg-rose-500/10 text-rose-400 border border-rose-500/20 shadow-lg shadow-rose-500/10'
            }`}
          >
            {isHealthy ? (
              <CheckCircle2 className="w-5 h-5 animate-pulse" />
            ) : (
              <AlertCircle className="w-5 h-5" />
            )}
          </div>
          <div>
            <div className="flex items-center gap-2">
              <span className="text-sm font-semibold text-slate-200">
                {t('statusBanner.proxyStatus')}:
              </span>
              <span
                className={`text-xs font-bold px-2 py-0.5 rounded-full ${
                  isHealthy
                    ? 'bg-emerald-500/20 text-emerald-300 border border-emerald-500/30'
                    : 'bg-rose-500/20 text-rose-300 border border-rose-500/30'
                }`}
              >
                {health?.status ? health.status.toUpperCase() : 'UNKNOWN'}
              </span>
            </div>
            <p className="text-xs text-slate-400 mt-0.5 flex items-center gap-1.5">
              <ShieldCheck className="w-3.5 h-3.5 text-cyan-400" />
              <span>{t('statusBanner.configSync')}: {t('statusBanner.inSync')}</span>
            </p>
          </div>
        </div>

        {/* Center: Metrics Badges */}
        <div className="grid grid-cols-3 gap-3 w-full md:w-auto">
          <div className="bg-slate-950/60 border border-slate-800/60 rounded-xl px-3 py-2 text-center">
            <div className="text-[11px] font-medium text-slate-400 flex items-center justify-center gap-1">
              <Radio className="w-3 h-3 text-cyan-400" />
              <span>{t('statusBanner.activeServers')}</span>
            </div>
            <div className="text-base font-bold text-slate-100 mt-0.5">
              {health?.activeServers ?? 0} <span className="text-xs font-normal text-slate-400">/ {health?.totalServers ?? 0}</span>
            </div>
          </div>

          <div className="bg-slate-950/60 border border-slate-800/60 rounded-xl px-3 py-2 text-center">
            <div className="text-[11px] font-medium text-slate-400 flex items-center justify-center gap-1">
              <Activity className="w-3 h-3 text-blue-400" />
              <span>{t('statusBanner.activeConnections')}</span>
            </div>
            <div className="text-base font-bold text-slate-100 mt-0.5">
              {health?.activeConnections ?? 0}
            </div>
          </div>

          <div className="bg-slate-950/60 border border-slate-800/60 rounded-xl px-3 py-2 text-center">
            <div className="text-[11px] font-medium text-slate-400 flex items-center justify-center gap-1">
              <Cpu className="w-3 h-3 text-violet-400" />
              <span>{t('statusBanner.uptime')}</span>
            </div>
            <div className="text-xs font-semibold text-slate-200 mt-1 truncate max-w-[90px]">
              {health?.uptime ?? '--'}
            </div>
          </div>
        </div>

        {/* Right: Quick Reload Button */}
        <div className="flex items-center gap-2 w-full md:w-auto justify-end">
          <button
            onClick={onReload}
            disabled={isReloading}
            className="flex items-center justify-center gap-2 px-4 py-2 rounded-xl text-xs font-semibold bg-gradient-to-r from-cyan-500 to-blue-600 hover:from-cyan-400 hover:to-blue-500 text-white shadow-lg shadow-cyan-500/20 border border-cyan-400/30 transition-all disabled:opacity-50 cursor-pointer w-full md:w-auto"
          >
            <RefreshCw className={`w-3.5 h-3.5 ${isReloading ? 'animate-spin' : ''}`} />
            <span>{isReloading ? t('statusBanner.reloading') : t('statusBanner.quickReload')}</span>
          </button>
        </div>
      </div>

      {/* Reload Notification Banner */}
      {lastReloadResult && (
        <div
          className={`mt-3 p-2.5 rounded-xl text-xs flex items-center justify-between transition-all ${
            lastReloadResult.success
              ? 'bg-emerald-500/10 border border-emerald-500/20 text-emerald-300'
              : 'bg-rose-500/10 border border-rose-500/20 text-rose-300'
          }`}
        >
          <div className="flex items-center gap-2">
            {lastReloadResult.success ? (
              <CheckCircle2 className="w-4 h-4 text-emerald-400 shrink-0" />
            ) : (
              <AlertCircle className="w-4 h-4 text-rose-400 shrink-0" />
            )}
            <span>{lastReloadResult.message}</span>
          </div>
          {lastReloadResult.reloadedCount !== undefined && (
            <span className="text-[10px] font-mono opacity-80">
              {lastReloadResult.reloadedCount} servers updated
            </span>
          )}
        </div>
      )}
    </div>
  );
}
