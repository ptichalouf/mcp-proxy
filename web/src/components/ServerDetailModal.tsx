import { useI18n } from '../i18n';
import type { CatalogItem } from '../types';
import {
  X,
  Plus,
  ExternalLink,
  Shield,
  Star,
  Terminal,
  Radio,
  Key,
  BookOpen,
} from 'lucide-react';

interface ServerDetailModalProps {
  isOpen: boolean;
  onClose: () => void;
  item: CatalogItem | null;
  onInstall: (item: CatalogItem) => void;
}

export function ServerDetailModal({
  isOpen,
  onClose,
  item,
  onInstall,
}: ServerDetailModalProps) {
  const { t, locale } = useI18n();

  if (!isOpen || !item) return null;

  const description =
    locale === 'fr' && item.descriptionFr ? item.descriptionFr : item.description;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-slate-950/80 backdrop-blur-md animate-fade-in">
      <div className="bg-slate-900 border border-slate-800 rounded-2xl w-full max-w-2xl max-h-[90vh] flex flex-col shadow-2xl overflow-hidden">
        {/* Header */}
        <div className="flex items-start justify-between px-6 py-5 border-b border-slate-800 bg-slate-950/60">
          <div className="flex items-center gap-4">
            <div className="w-12 h-12 rounded-2xl bg-gradient-to-tr from-slate-800 to-slate-700 border border-slate-700/60 flex items-center justify-center text-cyan-400 font-bold text-2xl shadow-inner">
              {item.icon ? <span>{item.icon}</span> : item.name.slice(0, 2).toUpperCase()}
            </div>
            <div>
              <div className="flex items-center gap-2">
                <h2 className="text-base font-bold text-slate-100">{item.name}</h2>
                {item.verified && (
                  <span className="flex items-center gap-1 text-[10px] font-semibold px-2 py-0.5 rounded-full bg-cyan-500/10 text-cyan-300 border border-cyan-500/20">
                    <Shield className="w-3 h-3 text-cyan-400" />
                    Verified
                  </span>
                )}
              </div>
              <p className="text-xs text-slate-400 mt-0.5">by {item.author}</p>
            </div>
          </div>
          <button
            onClick={onClose}
            className="p-1.5 rounded-lg text-slate-400 hover:text-slate-200 hover:bg-slate-800 transition-colors"
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        {/* Content */}
        <div className="flex-1 overflow-y-auto p-6 space-y-5 text-xs text-slate-300">
          {/* Badges & Meta */}
          <div className="flex flex-wrap items-center gap-2">
            <span className="px-2.5 py-1 rounded-lg bg-slate-800 text-slate-200 font-semibold border border-slate-700">
              Category: {item.category}
            </span>
            <span className="px-2.5 py-1 rounded-lg bg-cyan-500/10 text-cyan-300 font-mono font-semibold border border-cyan-500/20 flex items-center gap-1">
              {item.transportType === 'stdio' ? <Terminal className="w-3 h-3" /> : <Radio className="w-3 h-3" />}
              {item.transportType}
            </span>
            {item.license && (
              <span className="px-2.5 py-1 rounded-lg bg-slate-800/80 text-slate-400 border border-slate-700">
                License: {item.license}
              </span>
            )}
            {item.stars !== undefined && (
              <span className="px-2.5 py-1 rounded-lg bg-amber-500/10 text-amber-300 border border-amber-500/20 flex items-center gap-1 font-semibold">
                <Star className="w-3 h-3 fill-amber-400/20" />
                {item.stars} stars
              </span>
            )}
          </div>

          {/* Description */}
          <div>
            <h4 className="text-xs font-bold text-slate-100 uppercase tracking-wider mb-1.5 flex items-center gap-1.5">
              <BookOpen className="w-3.5 h-3.5 text-cyan-400" />
              {t('serverDetailModal.description')}
            </h4>
            <p className="text-slate-300 leading-relaxed bg-slate-950/60 border border-slate-800/80 rounded-xl p-3.5">
              {description}
            </p>
          </div>

          {/* Default Command / Config Preview */}
          {item.defaultConfig && (
            <div>
              <h4 className="text-xs font-bold text-slate-100 uppercase tracking-wider mb-1.5 flex items-center gap-1.5">
                <Terminal className="w-3.5 h-3.5 text-cyan-400" />
                Default Execution
              </h4>
              <div className="bg-slate-950 font-mono p-3 rounded-xl border border-slate-800 text-[11px] text-cyan-300">
                {item.transportType === 'stdio' ? (
                  <span>
                    {item.defaultConfig.command} {item.defaultConfig.args?.join(' ')}
                  </span>
                ) : (
                  <span>{item.defaultConfig.url}</span>
                )}
              </div>
            </div>
          )}

          {/* Environment Variables Requirements */}
          {item.envRequirements && item.envRequirements.length > 0 && (
            <div>
              <h4 className="text-xs font-bold text-slate-100 uppercase tracking-wider mb-1.5 flex items-center gap-1.5">
                <Key className="w-3.5 h-3.5 text-cyan-400" />
                {t('serverDetailModal.envRequirements')}
              </h4>
              <div className="space-y-2">
                {item.envRequirements.map((env) => (
                  <div
                    key={env.key}
                    className="p-3 bg-slate-950/60 border border-slate-800/80 rounded-xl flex items-start justify-between gap-3"
                  >
                    <div>
                      <div className="flex items-center gap-2 font-mono font-bold text-cyan-300">
                        <span>{env.key}</span>
                        {env.required && (
                          <span className="text-[10px] font-sans px-1.5 py-0.2 rounded bg-rose-500/10 text-rose-400 border border-rose-500/20">
                            Required
                          </span>
                        )}
                        {env.secret && (
                          <span className="text-[10px] font-sans px-1.5 py-0.2 rounded bg-amber-500/10 text-amber-400 border border-amber-500/20">
                            Secret
                          </span>
                        )}
                      </div>
                      <p className="text-slate-400 mt-1">{env.description}</p>
                    </div>
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* Tags */}
          {item.tags && (
            <div className="flex flex-wrap gap-1.5 pt-2">
              {item.tags.map((tag) => (
                <span
                  key={tag}
                  className="px-2 py-0.5 rounded-md bg-slate-950 text-slate-400 border border-slate-800 text-[10px]"
                >
                  #{tag}
                </span>
              ))}
            </div>
          )}
        </div>

        {/* Footer */}
        <div className="flex items-center justify-between px-6 py-4 border-t border-slate-800 bg-slate-950/60">
          {item.repositoryUrl ? (
            <a
              href={item.repositoryUrl}
              target="_blank"
              rel="noopener noreferrer"
              className="text-xs text-slate-400 hover:text-slate-200 flex items-center gap-1.5 underline decoration-slate-700 hover:decoration-slate-400"
            >
              <span>{t('serverDetailModal.repository')}</span>
              <ExternalLink className="w-3 h-3" />
            </a>
          ) : (
            <div />
          )}

          <div className="flex items-center gap-2">
            <button
              onClick={onClose}
              className="px-4 py-2 rounded-xl text-xs font-semibold text-slate-400 hover:text-slate-200 hover:bg-slate-800 border border-slate-800 transition-colors"
            >
              {t('common.close')}
            </button>
            <button
              onClick={() => {
                onClose();
                onInstall(item);
              }}
              className="px-5 py-2 rounded-xl text-xs font-semibold bg-gradient-to-r from-cyan-500 to-blue-600 hover:from-cyan-400 hover:to-blue-500 text-white shadow-lg shadow-cyan-500/20 border border-cyan-400/30 flex items-center gap-1.5"
            >
              <Plus className="w-3.5 h-3.5" />
              <span>{t('serverDetailModal.installNow')}</span>
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
