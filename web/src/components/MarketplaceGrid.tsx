import { useMemo } from 'react';
import { useI18n } from '../i18n';
import type { CatalogItem, InstalledServer } from '../types';
import { CatalogCard } from './CatalogCard';
import { SearchX } from 'lucide-react';

interface MarketplaceGridProps {
  items: CatalogItem[];
  installedServers: InstalledServer[];
  isLoading: boolean;
  onInstall: (item: CatalogItem) => void;
  onViewDetails: (item: CatalogItem) => void;
  onManage: (server: InstalledServer) => void;
}

export function MarketplaceGrid({
  items,
  installedServers,
  isLoading,
  onInstall,
  onViewDetails,
  onManage,
}: MarketplaceGridProps) {
  const { t } = useI18n();

  const installedMap = useMemo(() => {
    const map = new Map<string, InstalledServer>();
    for (const server of installedServers) {
      if (server.catalogId) {
        map.set(server.catalogId, server);
      }
      map.set(server.name.toLowerCase(), server);
    }
    return map;
  }, [installedServers]);

  if (isLoading) {
    return (
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {[...Array(6)].map((_, i) => (
          <div
            key={i}
            className="bg-slate-900/40 border border-slate-800/60 rounded-2xl p-5 h-56 animate-pulse flex flex-col justify-between"
          >
            <div className="flex items-center gap-3">
              <div className="w-11 h-11 rounded-xl bg-slate-800/80" />
              <div className="space-y-2 flex-1">
                <div className="h-4 bg-slate-800/80 rounded w-1/2" />
                <div className="h-3 bg-slate-800/60 rounded w-1/3" />
              </div>
            </div>
            <div className="space-y-2">
              <div className="h-3 bg-slate-800/60 rounded w-full" />
              <div className="h-3 bg-slate-800/60 rounded w-4/5" />
            </div>
            <div className="h-8 bg-slate-800/80 rounded-xl w-full" />
          </div>
        ))}
      </div>
    );
  }

  if (items.length === 0) {
    return (
      <div className="bg-slate-900/40 border border-slate-800/60 rounded-2xl p-12 text-center flex flex-col items-center justify-center">
        <div className="w-12 h-12 rounded-2xl bg-slate-800/80 border border-slate-700/60 flex items-center justify-center text-slate-400 mb-3">
          <SearchX className="w-6 h-6" />
        </div>
        <h3 className="text-sm font-bold text-slate-200">{t('common.noResults')}</h3>
        <p className="text-xs text-slate-400 mt-1 max-w-sm">
          No MCP servers matched your current filter criteria. Try clearing search or choosing another category.
        </p>
      </div>
    );
  }

  return (
    <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
      {items.map((item) => {
        const installed =
          installedMap.get(item.id) || installedMap.get(item.name.toLowerCase());
        return (
          <CatalogCard
            key={item.id}
            item={item}
            installedServer={installed}
            onInstall={onInstall}
            onViewDetails={onViewDetails}
            onManage={onManage}
          />
        );
      })}
    </div>
  );
}
