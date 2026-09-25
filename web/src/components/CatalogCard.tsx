import { useI18n } from '../i18n';
import type { CatalogItem, InstalledServer } from '../types';
import {
  Star,
  Check,
  Plus,
  Terminal,
  Radio,
  Info,
  Shield,
} from 'lucide-react';

interface CatalogCardProps {
  item: CatalogItem;
  installedServer?: InstalledServer;
  onInstall: (item: CatalogItem) => void;
  onViewDetails: (item: CatalogItem) => void;
  onManage: (server: InstalledServer) => void;
}

export function CatalogCard({
  item,
  installedServer,
  onInstall,
  onViewDetails,
  onManage,
}: CatalogCardProps) {
  const { t, locale } = useI18n();

  const isInstalled = !!installedServer;
  const isEnabled = installedServer?.enabled ?? false;

  // Bilingual description fallback
  const description =
    locale === 'fr' && item.descriptionFr ? item.descriptionFr : item.description;

  return (
    <div className="group relative bg-slate-900/60 border border-slate-800/80 hover:border-cyan-500/40 rounded-2xl p-5 flex flex-col justify-between transition-all duration-300 hover:shadow-xl hover:shadow-cyan-500/5 hover:-translate-y-0.5">
      <div>
        {/* Top Header: Icon, Name, Category, Transport */}
        <div className="flex items-start justify-between gap-3">
          <div className="flex items-center gap-3">
            <div className="w-11 h-11 rounded-xl bg-gradient-to-tr from-slate-800 to-slate-700/80 border border-slate-700/60 flex items-center justify-center text-cyan-400 font-bold text-lg shadow-inner group-hover:text-cyan-300 group-hover:border-cyan-500/30 transition-colors">
              {item.icon ? (
                <span className="text-xl">{item.icon}</span>
              ) : (
                item.name.slice(0, 2).toUpperCase()
              )}
            </div>
            <div>
              <h3 className="text-sm font-bold text-slate-100 group-hover:text-cyan-300 transition-colors flex items-center gap-1.5">
                <span>{item.name}</span>
                {item.verified && (
                  <span title="Verified MCP Server">
                    <Shield className="w-3.5 h-3.5 text-cyan-400 fill-cyan-400/20" />
                  </span>
                )}
              </h3>
              <p className="text-xs text-slate-400">by {item.author || item.vendor || 'Community'}</p>
            </div>
          </div>

          {/* Badges */}
          <div className="flex flex-col items-end gap-1">
            <span className="text-[10px] font-semibold px-2 py-0.5 rounded-full bg-slate-800/80 text-slate-300 border border-slate-700/50">
              {item.category}
            </span>
            <span className="text-[10px] font-mono px-1.5 py-0.2 rounded bg-cyan-500/10 text-cyan-400 border border-cyan-500/20 flex items-center gap-1">
              {item.transportType === 'stdio' ? (
                <Terminal className="w-2.5 h-2.5" />
              ) : (
                <Radio className="w-2.5 h-2.5" />
              )}
              {item.transportType}
            </span>
          </div>
        </div>

        {/* Description */}
        <p className="text-xs text-slate-300 mt-3 line-clamp-2 leading-relaxed min-h-[32px]">
          {description}
        </p>

        {/* Tags & Stats */}
        <div className="flex flex-wrap items-center gap-1.5 mt-3.5">
          {item.tags?.slice(0, 3).map((tag) => (
            <span
              key={tag}
              className="text-[10px] px-2 py-0.5 rounded-md bg-slate-950/60 text-slate-400 border border-slate-800/60"
            >
              #{tag}
            </span>
          ))}

          {item.stars !== undefined && (
            <span className="text-[10px] text-amber-400 flex items-center gap-0.5 ml-auto font-medium">
              <Star className="w-3 h-3 fill-amber-400/20" />
              {item.stars}
            </span>
          )}
        </div>
      </div>

      {/* Footer / Actions */}
      <div className="mt-5 pt-3.5 border-t border-slate-800/60 flex items-center justify-between gap-2">
        <button
          onClick={() => onViewDetails(item)}
          className="text-xs text-slate-400 hover:text-slate-200 flex items-center gap-1 px-2.5 py-1.5 rounded-lg hover:bg-slate-800/60 transition-colors cursor-pointer"
        >
          <Info className="w-3.5 h-3.5" />
          <span>{t('installModal.learnMore')}</span>
        </button>

        {isInstalled ? (
          <div className="flex items-center gap-2">
            <span
              className={`text-[11px] font-semibold px-2 py-1 rounded-lg flex items-center gap-1 ${
                isEnabled
                  ? 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/20'
                  : 'bg-amber-500/10 text-amber-400 border border-amber-500/20'
              }`}
            >
              <Check className="w-3 h-3" />
              {t('common.installed')}
            </span>
            <button
              onClick={() => onManage(installedServer!)}
              className="text-xs font-semibold px-3 py-1.5 rounded-lg bg-slate-800 hover:bg-slate-700 text-slate-200 border border-slate-700 transition-colors cursor-pointer"
            >
              {t('table.edit')}
            </button>
          </div>
        ) : (
          <button
            onClick={() => onInstall(item)}
            className="flex items-center gap-1.5 text-xs font-semibold px-3.5 py-1.5 rounded-xl bg-gradient-to-r from-cyan-500 to-blue-600 hover:from-cyan-400 hover:to-blue-500 text-white shadow-md shadow-cyan-500/20 border border-cyan-400/30 transition-all cursor-pointer"
          >
            <Plus className="w-3.5 h-3.5" />
            <span>{t('common.install')}</span>
          </button>
        )}
      </div>
    </div>
  );
}
