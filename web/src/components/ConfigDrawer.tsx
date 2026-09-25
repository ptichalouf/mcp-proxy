import { useState, useEffect } from 'react';
import { useI18n } from '../i18n';
import type { InstalledServer } from '../types';
import {
  X,
  Save,
  Plus,
  Trash2,
  Eye,
  EyeOff,
  Power,
  AlertCircle,
} from 'lucide-react';

interface ConfigDrawerProps {
  isOpen: boolean;
  onClose: () => void;
  server: InstalledServer | null;
  onSave: (
    id: string,
    data: {
      name?: string;
      enabled?: boolean;
      env?: Record<string, string>;
      args?: string[];
      command?: string;
      url?: string;
    }
  ) => Promise<void>;
}

export function ConfigDrawer({
  isOpen,
  onClose,
  server,
  onSave,
}: ConfigDrawerProps) {
  const { t } = useI18n();

  const [name, setName] = useState('');
  const [enabled, setEnabled] = useState(true);
  const [command, setCommand] = useState('');
  const [argsText, setArgsText] = useState('');
  const [url, setUrl] = useState('');
  const [envVars, setEnvVars] = useState<Array<{ key: string; value: string; isSecret?: boolean }>>([]);
  const [showSecrets, setShowSecrets] = useState<Record<number, boolean>>({});

  const [isSaving, setIsSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (server) {
      setName(server.name);
      setEnabled(server.enabled);
      setCommand(server.command || '');
      setArgsText(server.args?.join(' ') || '');
      setUrl(server.url || '');

      const envList: Array<{ key: string; value: string; isSecret?: boolean }> = [];
      if (server.env) {
        for (const [k, v] of Object.entries(server.env)) {
          envList.push({
            key: k,
            value: v,
            isSecret:
              k.toLowerCase().includes('key') ||
              k.toLowerCase().includes('token') ||
              k.toLowerCase().includes('secret'),
          });
        }
      }
      setEnvVars(envList);
      setError(null);
      setShowSecrets({});
    }
  }, [server, isOpen]);

  if (!isOpen || !server) return null;

  const handleAddEnv = () => {
    setEnvVars((prev) => [...prev, { key: '', value: '' }]);
  };

  const handleRemoveEnv = (index: number) => {
    setEnvVars((prev) => prev.filter((_, i) => i !== index));
  };

  const handleEnvChange = (index: number, field: 'key' | 'value', val: string) => {
    setEnvVars((prev) =>
      prev.map((item, i) => (i === index ? { ...item, [field]: val } : item))
    );
  };

  const toggleSecretVisibility = (index: number) => {
    setShowSecrets((prev) => ({ ...prev, [index]: !prev[index] }));
  };

  const handleSave = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!name.trim()) {
      setError(t('errors.nameRequired'));
      return;
    }

    setIsSaving(true);
    setError(null);

    try {
      const envMap: Record<string, string> = {};
      for (const item of envVars) {
        if (item.key.trim()) {
          envMap[item.key.trim()] = item.value;
        }
      }

      const parsedArgs = argsText
        .trim()
        .split(/\s+/)
        .filter(Boolean);

      await onSave(server.id, {
        name: name.trim(),
        enabled,
        command: server.transportType === 'stdio' ? command.trim() : undefined,
        args: server.transportType === 'stdio' ? parsedArgs : undefined,
        url: server.transportType !== 'stdio' ? url.trim() : undefined,
        env: envMap,
      });

      onClose();
    } catch (err: any) {
      setError(err.message || 'Failed to update configuration');
    } finally {
      setIsSaving(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 overflow-hidden bg-slate-950/70 backdrop-blur-sm animate-fade-in">
      <div className="absolute inset-0" onClick={onClose} />

      <div className="fixed inset-y-0 right-0 max-w-full flex pl-10">
        <div className="w-screen max-w-md bg-slate-900 border-l border-slate-800 shadow-2xl flex flex-col justify-between">
          {/* Header */}
          <div className="px-6 py-5 border-b border-slate-800 bg-slate-950/50 flex items-center justify-between">
            <div>
              <h2 className="text-sm font-bold text-slate-100 flex items-center gap-2">
                <span>{t('configDrawer.title')}</span>
                <span className="text-[10px] font-mono px-2 py-0.5 rounded bg-slate-800 text-slate-400">
                  {server.id}
                </span>
              </h2>
              <p className="text-xs text-slate-400 mt-0.5">
                {server.transportType.toUpperCase()} Server Configuration
              </p>
            </div>
            <button
              onClick={onClose}
              className="p-1.5 rounded-lg text-slate-400 hover:text-slate-200 hover:bg-slate-800"
            >
              <X className="w-4 h-4" />
            </button>
          </div>

          {/* Form Content */}
          <form onSubmit={handleSave} className="flex-1 overflow-y-auto p-6 space-y-5">
            {error && (
              <div className="p-3 rounded-xl bg-rose-500/10 border border-rose-500/30 text-rose-300 text-xs flex items-center gap-2">
                <AlertCircle className="w-4 h-4 shrink-0" />
                <span>{error}</span>
              </div>
            )}

            {/* Server Status Toggle */}
            <div className="p-3.5 bg-slate-950/60 border border-slate-800/80 rounded-xl flex items-center justify-between">
              <div className="flex items-center gap-2.5">
                <Power className={`w-4 h-4 ${enabled ? 'text-emerald-400' : 'text-slate-500'}`} />
                <div>
                  <div className="text-xs font-semibold text-slate-200">
                    {t('configDrawer.status')}
                  </div>
                  <div className="text-[11px] text-slate-400">
                    {enabled ? 'Server is active and receiving requests' : 'Server is disabled'}
                  </div>
                </div>
              </div>
              <button
                type="button"
                onClick={() => setEnabled(!enabled)}
                className={`w-11 h-6 flex items-center rounded-full p-1 transition-colors cursor-pointer ${
                  enabled ? 'bg-cyan-500 justify-end' : 'bg-slate-800 justify-start'
                }`}
              >
                <div className="bg-white w-4 h-4 rounded-full shadow-md" />
              </button>
            </div>

            {/* Server Name */}
            <div>
              <label className="block text-xs font-semibold text-slate-200 mb-1">
                {t('configDrawer.serverName')}
              </label>
              <input
                type="text"
                value={name}
                onChange={(e) => setName(e.target.value)}
                required
                className="w-full bg-slate-950/80 border border-slate-800 rounded-xl px-3.5 py-2 text-xs text-slate-100 placeholder-slate-500 focus:outline-none focus:ring-2 focus:ring-cyan-500/50"
              />
            </div>

            {/* stdio vs remote command/url */}
            {server.transportType === 'stdio' ? (
              <>
                <div>
                  <label className="block text-xs font-semibold text-slate-200 mb-1">
                    {t('configDrawer.command')}
                  </label>
                  <input
                    type="text"
                    value={command}
                    onChange={(e) => setCommand(e.target.value)}
                    className="w-full bg-slate-950/80 border border-slate-800 rounded-xl px-3.5 py-2 text-xs font-mono text-slate-100 placeholder-slate-500 focus:outline-none focus:ring-2 focus:ring-cyan-500/50"
                  />
                </div>

                <div>
                  <label className="block text-xs font-semibold text-slate-200 mb-1">
                    {t('configDrawer.args')}
                  </label>
                  <input
                    type="text"
                    value={argsText}
                    onChange={(e) => setArgsText(e.target.value)}
                    className="w-full bg-slate-950/80 border border-slate-800 rounded-xl px-3.5 py-2 text-xs font-mono text-slate-100 placeholder-slate-500 focus:outline-none focus:ring-2 focus:ring-cyan-500/50"
                  />
                </div>
              </>
            ) : (
              <div>
                <label className="block text-xs font-semibold text-slate-200 mb-1">
                  {t('configDrawer.url')}
                </label>
                <input
                  type="url"
                  value={url}
                  onChange={(e) => setUrl(e.target.value)}
                  className="w-full bg-slate-950/80 border border-slate-800 rounded-xl px-3.5 py-2 text-xs font-mono text-slate-100 placeholder-slate-500 focus:outline-none focus:ring-2 focus:ring-cyan-500/50"
                />
              </div>
            )}

            {/* Environment Variables */}
            <div>
              <div className="flex items-center justify-between mb-2">
                <label className="text-xs font-semibold text-slate-200">
                  {t('configDrawer.envVars')}
                </label>
                <button
                  type="button"
                  onClick={handleAddEnv}
                  className="text-xs text-cyan-400 hover:text-cyan-300 flex items-center gap-1 font-medium cursor-pointer"
                >
                  <Plus className="w-3.5 h-3.5" />
                  <span>{t('configDrawer.addEnv')}</span>
                </button>
              </div>

              <div className="space-y-2">
                {envVars.map((envItem, index) => {
                  const isSecret = envItem.isSecret;
                  const isVisible = showSecrets[index];

                  return (
                    <div key={index} className="flex items-center gap-2">
                      <input
                        type="text"
                        value={envItem.key}
                        onChange={(e) => handleEnvChange(index, 'key', e.target.value)}
                        placeholder="KEY"
                        className="w-2/5 bg-slate-950/80 border border-slate-800 rounded-xl px-3 py-1.5 text-xs font-mono text-slate-100 placeholder-slate-500 focus:outline-none focus:ring-1 focus:ring-cyan-500/50"
                      />
                      <div className="relative flex-1">
                        <input
                          type={isSecret && !isVisible ? 'password' : 'text'}
                          value={envItem.value}
                          onChange={(e) => handleEnvChange(index, 'value', e.target.value)}
                          placeholder="VALUE"
                          className="w-full bg-slate-950/80 border border-slate-800 rounded-xl px-3 py-1.5 pr-8 text-xs font-mono text-slate-100 placeholder-slate-500 focus:outline-none focus:ring-1 focus:ring-cyan-500/50"
                        />
                        {isSecret && (
                          <button
                            type="button"
                            onClick={() => toggleSecretVisibility(index)}
                            className="absolute right-2.5 top-1/2 -translate-y-1/2 text-slate-500 hover:text-slate-300"
                          >
                            {isVisible ? <EyeOff className="w-3.5 h-3.5" /> : <Eye className="w-3.5 h-3.5" />}
                          </button>
                        )}
                      </div>
                      <button
                        type="button"
                        onClick={() => handleRemoveEnv(index)}
                        className="p-1.5 text-slate-500 hover:text-rose-400 rounded-lg hover:bg-slate-800"
                      >
                        <Trash2 className="w-3.5 h-3.5" />
                      </button>
                    </div>
                  );
                })}

                {envVars.length === 0 && (
                  <div className="text-center py-4 border border-dashed border-slate-800 rounded-xl text-xs text-slate-500">
                    No custom environment variables.
                  </div>
                )}
              </div>
            </div>
          </form>

          {/* Footer */}
          <div className="px-6 py-4 border-t border-slate-800 bg-slate-950/50 flex items-center justify-end gap-3">
            <button
              type="button"
              onClick={onClose}
              disabled={isSaving}
              className="px-4 py-2 rounded-xl text-xs font-semibold text-slate-400 hover:text-slate-200 hover:bg-slate-800 border border-slate-800 transition-colors"
            >
              {t('common.cancel')}
            </button>
            <button
              onClick={handleSave}
              disabled={isSaving}
              className="px-5 py-2 rounded-xl text-xs font-semibold bg-gradient-to-r from-cyan-500 to-blue-600 hover:from-cyan-400 hover:to-blue-500 text-white shadow-lg shadow-cyan-500/20 border border-cyan-400/30 flex items-center gap-1.5 disabled:opacity-50"
            >
              {isSaving ? (
                <>
                  <span className="w-3.5 h-3.5 border-2 border-white/30 border-t-white rounded-full animate-spin" />
                  <span>{t('configDrawer.saving')}</span>
                </>
              ) : (
                <>
                  <Save className="w-3.5 h-3.5" />
                  <span>{t('configDrawer.save')}</span>
                </>
              )}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
