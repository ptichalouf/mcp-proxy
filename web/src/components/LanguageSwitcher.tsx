import { useI18n } from '../i18n';
import { LOCALES, LOCALE_LABELS, type Locale } from '../i18n/locales';
import { Globe } from 'lucide-react';

export function LanguageSwitcher() {
  const { locale, setLocale } = useI18n();

  return (
    <div className="flex items-center gap-1 bg-slate-900/80 border border-slate-800 rounded-lg p-1 text-xs">
      <Globe className="w-3.5 h-3.5 text-slate-400 ml-1 mr-0.5" />
      {LOCALES.map((l: Locale) => {
        const isCurrent = locale === l;
        return (
          <button
            key={l}
            type="button"
            onClick={() => setLocale(l)}
            aria-pressed={isCurrent}
            className={`px-2 py-1 rounded transition-all font-medium flex items-center gap-1.5 ${
              isCurrent
                ? 'bg-cyan-500/20 text-cyan-300 border border-cyan-500/30 shadow-sm'
                : 'text-slate-400 hover:text-slate-200 hover:bg-slate-800/60'
            }`}
          >
            <span>{LOCALE_LABELS[l].flag}</span>
            <span>{LOCALE_LABELS[l].name}</span>
          </button>
        );
      })}
    </div>
  );
}
