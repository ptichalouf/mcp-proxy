import { useI18n } from '../i18n';
import { LanguageSwitcher } from './LanguageSwitcher';
import { Server, Store, Plus, RefreshCw } from 'lucide-react';

interface HeaderProps {
  activeTab: 'marketplace' | 'installed';
  onTabChange: (tab: 'marketplace' | 'installed') => void;
  installedCount: number;
  onRefresh: () => void;
  isRefreshing: boolean;
  onAddCustom: () => void;
}

export function Header({
  activeTab,
  onTabChange,
  installedCount,
  onRefresh,
  isRefreshing,
  onAddCustom,
}: HeaderProps) {
  const { t } = useI18n();

  return (
    <header className="border-b border-slate-800 bg-slate-900/50 backdrop-blur-md sticky top-0 z-30">
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
        <div className="flex items-center justify-between h-16">
          {/* Logo & Branding */}
          <div className="flex items-center gap-3">
            <div className="w-9 h-9 rounded-xl bg-gradient-to-tr from-cyan-500 to-blue-600 flex items-center justify-center shadow-lg shadow-cyan-500/20 text-white font-bold">
              <Server className="w-5 h-5" />
            </div>
            <div>
              <div className="flex items-center gap-2">
                <h1 className="text-base font-bold text-slate-100 tracking-tight">
                  {t('common.appName')}
                </h1>
                <span className="text-[10px] font-semibold px-2 py-0.5 rounded-full bg-cyan-500/10 text-cyan-400 border border-cyan-500/20">
                  v2.0
                </span>
              </div>
              <p className="text-xs text-slate-400 hidden sm:block">
                {t('common.appSubtitle')}
              </p>
            </div>
          </div>

          {/* Navigation Tabs */}
          <nav className="flex items-center bg-slate-950/80 p-1 rounded-xl border border-slate-800">
            <button
              onClick={() => onTabChange('marketplace')}
              className={`flex items-center gap-2 px-3.5 py-1.5 rounded-lg text-xs font-semibold transition-all ${
                activeTab === 'marketplace'
                  ? 'bg-gradient-to-r from-cyan-500 to-blue-600 text-white shadow-md shadow-cyan-500/25'
                  : 'text-slate-400 hover:text-slate-200 hover:bg-slate-900'
              }`}
            >
              <Store className="w-4 h-4" />
              <span>{t('navigation.marketplace')}</span>
            </button>
            <button
              onClick={() => onTabChange('installed')}
              className={`flex items-center gap-2 px-3.5 py-1.5 rounded-lg text-xs font-semibold transition-all relative ${
                activeTab === 'installed'
                  ? 'bg-gradient-to-r from-cyan-500 to-blue-600 text-white shadow-md shadow-cyan-500/25'
                  : 'text-slate-400 hover:text-slate-200 hover:bg-slate-900'
              }`}
            >
              <Server className="w-4 h-4" />
              <span>{t('navigation.installedServers')}</span>
              {installedCount > 0 && (
                <span className="px-1.5 py-0.2 rounded-full text-[10px] bg-slate-800 text-slate-300 font-bold ml-1">
                  {installedCount}
                </span>
              )}
            </button>
          </nav>

          {/* Actions & Locale */}
          <div className="flex items-center gap-2.5">
            <button
              onClick={onAddCustom}
              className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-semibold bg-cyan-500/10 text-cyan-300 hover:bg-cyan-500/20 border border-cyan-500/30 transition-all cursor-pointer"
            >
              <Plus className="w-3.5 h-3.5" />
              <span className="hidden md:inline">{t('common.addCustomServer')}</span>
            </button>

            <button
              onClick={onRefresh}
              disabled={isRefreshing}
              title={t('common.refresh')}
              className="p-2 rounded-lg text-slate-400 hover:text-slate-200 hover:bg-slate-800/80 border border-slate-800 transition-all disabled:opacity-50 cursor-pointer"
            >
              <RefreshCw className={`w-4 h-4 ${isRefreshing ? 'animate-spin text-cyan-400' : ''}`} />
            </button>

            <LanguageSwitcher />

            <a
              href="https://github.com/tbxark/mcp-proxy"
              target="_blank"
              rel="noopener noreferrer"
              className="p-2 rounded-lg text-slate-400 hover:text-slate-200 hover:bg-slate-800/80 border border-slate-800 transition-all hidden sm:flex items-center"
              title="GitHub"
            >
              <svg className="w-4 h-4 fill-current" viewBox="0 0 24 24">
                <path fillRule="evenodd" clipRule="evenodd" d="M12 2C6.477 2 2 6.484 2 12.017c0 4.425 2.865 8.18 6.839 9.504.5.092.682-.217.682-.483 0-.237-.008-.868-.013-1.703-2.782.605-3.369-1.343-3.369-1.343-.454-1.158-1.11-1.466-1.11-1.466-.908-.62.069-.608.069-.608 1.003.07 1.53 1.032 1.53 1.032.892 1.53 2.341 1.088 2.91.832.092-.647.35-1.088.636-1.338-2.22-.253-4.555-1.113-4.555-4.951 0-1.093.39-1.988 1.029-2.688-.103-.253-.446-1.272.098-2.65 0 0 .84-.27 2.75 1.026A9.564 9.564 0 0112 6.844c.85.004 1.705.115 2.504.337 1.909-1.296 2.747-1.027 2.747-1.027.546 1.379.202 2.398.1 2.651.64.7 1.028 1.595 1.028 2.688 0 3.848-2.339 4.695-4.566 4.943.359.309.678.92.678 1.855 0 1.338-.012 2.419-.012 2.747 0 .268.18.58.688.482A10.019 10.019 0 0022 12.017C22 6.484 17.522 2 12 2z" />
              </svg>
            </a>
          </div>
        </div>
      </div>
    </header>
  );
}
