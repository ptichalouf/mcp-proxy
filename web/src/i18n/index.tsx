import React, { createContext, useContext, useState, useEffect, useCallback } from 'react';
import { en } from './en';
import { fr } from './fr';
import { DEFAULT_LOCALE, LOCALE_STORAGE_KEY, type Locale } from './locales';
import type { TranslationKey, InterpolationParams, Dictionary } from './types';

const dictionaries: Record<Locale, Dictionary> = {
  en,
  fr,
};

interface I18nContextValue {
  locale: Locale;
  setLocale: (locale: Locale) => void;
  t: (key: TranslationKey, params?: InterpolationParams) => string;
}

const I18nContext = createContext<I18nContextValue | null>(null);

function getNestedValue(obj: Record<string, unknown>, path: string): string | undefined {
  const parts = path.split('.');
  let current: unknown = obj;
  for (const part of parts) {
    if (current && typeof current === 'object' && part in current) {
      current = (current as Record<string, unknown>)[part];
    } else {
      return undefined;
    }
  }
  return typeof current === 'string' ? current : undefined;
}

export function I18nProvider({ children }: { children: React.ReactNode }) {
  const [locale, setLocaleState] = useState<Locale>(() => {
    if (typeof window !== 'undefined') {
      const stored = localStorage.getItem(LOCALE_STORAGE_KEY) as Locale | null;
      if (stored && (stored === 'en' || stored === 'fr')) {
        return stored;
      }
      if (navigator.language?.toLowerCase().startsWith('fr')) {
        return 'fr';
      }
    }
    return DEFAULT_LOCALE;
  });

  const setLocale = useCallback((newLocale: Locale) => {
    setLocaleState(newLocale);
    if (typeof window !== 'undefined') {
      localStorage.setItem(LOCALE_STORAGE_KEY, newLocale);
      document.documentElement.lang = newLocale;
    }
  }, []);

  useEffect(() => {
    if (typeof window !== 'undefined') {
      document.documentElement.lang = locale;
    }
  }, [locale]);

  const t = useCallback(
    (key: TranslationKey, params?: InterpolationParams): string => {
      const dict = dictionaries[locale] || en;
      let text = getNestedValue(dict as unknown as Record<string, unknown>, key);

      // Fallback to English if translation missing
      if (!text && locale !== 'en') {
        text = getNestedValue(en as unknown as Record<string, unknown>, key);
      }

      if (!text) {
        return key;
      }

      if (params) {
        return Object.entries(params).reduce((acc, [paramKey, val]) => {
          return acc.replace(new RegExp(`\\{${paramKey}\\}`, 'g'), String(val));
        }, text);
      }

      return text;
    },
    [locale],
  );

  return (
    <I18nContext.Provider value={{ locale, setLocale, t }}>
      {children}
    </I18nContext.Provider>
  );
}

export function useI18n() {
  const context = useContext(I18nContext);
  if (!context) {
    throw new Error('useI18n must be used within an I18nProvider');
  }
  return context;
}
