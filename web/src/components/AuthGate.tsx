import { useState } from 'react';
import { KeyRound, Loader2, ShieldAlert } from 'lucide-react';
import { useI18n } from '../i18n';
import { api } from '../lib/api';
import { TOKEN_STORAGE_KEY, setToken } from '../lib/auth';

// Shown when the API answers 401: the operator set mcpProxy.options.authTokens,
// so the dashboard has to be given one before it can read anything. The token is
// verified against a real endpoint rather than trusted, so a typo is reported
// here instead of as an empty dashboard.
export function AuthGate({ onAuthenticated }: { onAuthenticated: () => void }) {
  const { t } = useI18n();
  const [value, setValue] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [isChecking, setIsChecking] = useState(false);

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    const candidate = value.trim();
    if (!candidate || isChecking) return;

    setIsChecking(true);
    setError(null);
    setToken(candidate);
    try {
      await api.getProxyHealth();
      onAuthenticated();
    } catch {
      setToken(null);
      setError(t('auth.invalid'));
    } finally {
      setIsChecking(false);
    }
  };

  return (
    <div className="min-h-screen bg-slate-950 text-slate-100 flex items-center justify-center p-4">
      <form
        onSubmit={submit}
        className="w-full max-w-md rounded-2xl border border-slate-800 bg-slate-900/60 p-6 space-y-4"
      >
        <div className="flex items-center gap-3">
          <div className="w-10 h-10 rounded-xl bg-amber-500/10 border border-amber-500/30 flex items-center justify-center text-amber-400">
            <ShieldAlert className="w-5 h-5" />
          </div>
          <div>
            <h1 className="text-sm font-bold">{t('auth.title')}</h1>
            <p className="text-xs text-slate-400">{t('auth.message')}</p>
          </div>
        </div>

        <div className="space-y-1.5">
          <label htmlFor="token" className="text-xs font-semibold text-slate-300">
            {t('auth.tokenLabel')}
          </label>
          <div className="relative">
            <KeyRound className="w-4 h-4 text-slate-500 absolute left-3 top-1/2 -translate-y-1/2" />
            <input
              id="token"
              type="password"
              autoFocus
              value={value}
              onChange={(event) => setValue(event.target.value)}
              placeholder={t('auth.tokenPlaceholder')}
              className="w-full pl-9 pr-3 py-2 rounded-lg bg-slate-950 border border-slate-800 text-sm text-slate-100 placeholder:text-slate-500 focus:outline-none focus:border-cyan-500/50"
            />
          </div>
        </div>

        {error && <p className="text-xs text-red-400">{error}</p>}

        <button
          type="submit"
          disabled={isChecking || !value.trim()}
          className="w-full flex items-center justify-center gap-2 px-4 py-2 rounded-lg text-sm font-semibold bg-gradient-to-r from-cyan-500 to-blue-600 text-white disabled:opacity-50 cursor-pointer"
        >
          {isChecking ? <Loader2 className="w-4 h-4 animate-spin" /> : <KeyRound className="w-4 h-4" />}
          <span>{t('auth.submit')}</span>
        </button>

        <p className="text-[11px] text-slate-500">
          {t('auth.stored', { key: TOKEN_STORAGE_KEY })}
        </p>
      </form>
    </div>
  );
}
