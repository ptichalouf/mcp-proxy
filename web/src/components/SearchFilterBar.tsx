import type { ReactNode } from 'react';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/types';
import {
  Search,
  Layers,
  Code2,
  Database,
  Globe,
  FolderTree,
  Cloud,
  Sparkles,
  MoreHorizontal,
  X,
} from 'lucide-react';

interface SearchFilterBarProps {
  searchQuery: string;
  onSearchChange: (q: string) => void;
  selectedCategory: string;
  onCategoryChange: (cat: string) => void;
  selectedTransport: string;
  onTransportChange: (trans: string) => void;
}

const CATEGORIES: { id: string; labelKey: TranslationKey; icon: ReactNode }[] = [
  { id: 'all', labelKey: 'categories.all', icon: <Layers className="w-3.5 h-3.5" /> },
  { id: 'developer', labelKey: 'categories.developer', icon: <Code2 className="w-3.5 h-3.5" /> },
  { id: 'database', labelKey: 'categories.database', icon: <Database className="w-3.5 h-3.5" /> },
  { id: 'search', labelKey: 'categories.search', icon: <Globe className="w-3.5 h-3.5" /> },
  { id: 'filesystem', labelKey: 'categories.filesystem', icon: <FolderTree className="w-3.5 h-3.5" /> },
  { id: 'cloud', labelKey: 'categories.cloud', icon: <Cloud className="w-3.5 h-3.5" /> },
  { id: 'communication', labelKey: 'categories.communication', icon: <Sparkles className="w-3.5 h-3.5" /> },
  { id: 'productivity', labelKey: 'categories.productivity', icon: <Sparkles className="w-3.5 h-3.5" /> },
  { id: 'monitoring', labelKey: 'categories.monitoring', icon: <Sparkles className="w-3.5 h-3.5" /> },
  { id: 'other', labelKey: 'categories.other', icon: <MoreHorizontal className="w-3.5 h-3.5" /> },
];

export function SearchFilterBar({
  searchQuery,
  onSearchChange,
  selectedCategory,
  onCategoryChange,
  selectedTransport,
  onTransportChange,
}: SearchFilterBarProps) {
  const { t } = useI18n();

  return (
    <div className="space-y-3">
      {/* Top row: Search input + Transport selector */}
      <div className="flex flex-col sm:flex-row items-stretch sm:items-center gap-3">
        {/* Search Input */}
        <div className="relative flex-1">
          <Search className="w-4 h-4 text-slate-400 absolute left-3.5 top-1/2 -translate-y-1/2 pointer-events-none" />
          <input
            type="text"
            value={searchQuery}
            onChange={(e) => onSearchChange(e.target.value)}
            placeholder={t('common.searchPlaceholder')}
            className="w-full bg-slate-900/80 border border-slate-800 rounded-xl pl-10 pr-10 py-2 text-sm text-slate-100 placeholder-slate-500 focus:outline-none focus:ring-2 focus:ring-cyan-500/50 focus:border-cyan-500/50 transition-all"
          />
          {searchQuery && (
            <button
              onClick={() => onSearchChange('')}
              className="absolute right-3 top-1/2 -translate-y-1/2 p-1 text-slate-400 hover:text-slate-200 rounded-md"
            >
              <X className="w-3.5 h-3.5" />
            </button>
          )}
        </div>

        {/* Transport Type Filter */}
        <div className="flex items-center gap-2">
          <select
            value={selectedTransport}
            onChange={(e) => onTransportChange(e.target.value)}
            className="bg-slate-900/80 border border-slate-800 rounded-xl px-3.5 py-2 text-xs font-semibold text-slate-300 focus:outline-none focus:ring-2 focus:ring-cyan-500/50 focus:border-cyan-500/50 cursor-pointer"
          >
            <option value="all">{t('categories.all')} Transports</option>
            <option value="stdio">stdio</option>
            <option value="sse">SSE (Server-Sent Events)</option>
            <option value="websocket">WebSocket</option>
            <option value="http">HTTP Stream</option>
          </select>
        </div>
      </div>

      {/* Category Pills */}
      <div className="flex items-center gap-1.5 overflow-x-auto pb-1 scrollbar-none">
        {CATEGORIES.map((cat) => {
          const isSelected = selectedCategory === cat.id;
          return (
            <button
              key={cat.id}
              onClick={() => onCategoryChange(cat.id)}
              className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium whitespace-nowrap transition-all cursor-pointer ${
                isSelected
                  ? 'bg-cyan-500/20 text-cyan-300 border border-cyan-500/40 shadow-sm shadow-cyan-500/10'
                  : 'bg-slate-900/50 text-slate-400 hover:text-slate-200 hover:bg-slate-800/60 border border-slate-800/80'
              }`}
            >
              {cat.icon}
              <span>{t(cat.labelKey as any)}</span>
            </button>
          );
        })}
      </div>
    </div>
  );
}
