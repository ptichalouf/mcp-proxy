export const LOCALES = ['en', 'fr'] as const;
export type Locale = (typeof LOCALES)[number];

export const DEFAULT_LOCALE: Locale = 'en';

export const LOCALE_STORAGE_KEY = 'mcp-proxy-web.locale';

export const LOCALE_LABELS: Record<Locale, { name: string; nativeName: string; flag: string }> = {
  en: { name: 'English', nativeName: 'English', flag: '🇺🇸' },
  fr: { name: 'French', nativeName: 'Français', flag: '🇫🇷' },
};

export const LOCALE_TAGS: Record<Locale, string> = {
  en: 'en-US',
  fr: 'fr-FR',
};
