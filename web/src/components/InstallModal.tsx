import { useState, useEffect, type FormEvent } from 'react';
import { useI18n } from '../i18n';
import type { CatalogItem, MCPTransportType } from '../types';
import { catalogEnvRequirements, missingRequiredEnv, type CatalogEnvField } from '../lib/catalogEnv';
import {
  X,
  Plus,
  Trash2,
  Terminal,
  Radio,
  Eye,
  EyeOff,
  AlertCircle,
} from 'lucide-react';

interface InstallModalProps {
  isOpen: boolean;
  onClose: () => void;
  catalogItem: CatalogItem | null;
  onInstall: (data: {
    catalogId?: string;
    name: string;
    transportType?: string;
    command?: string;
    args?: string[];
    url?: string;
    env?: Record<string, string>;
  }) => Promise<void>;
}

export function InstallModal({
  isOpen,
  onClose,
  catalogItem,
  onInstall,
}: InstallModalProps) {
  const { t, locale } = useI18n();

  const [name, setName] = useState('');
  const [transportType, setTransportType] = useState<MCPTransportType>('stdio');
  const [command, setCommand] = useState('');
  const [argsText, setArgsText] = useState('');
  const [url, setUrl] = useState('');
  const [envVars, setEnvVars] = useState<CatalogEnvField[]>([]);
  const [showSecrets, setShowSecrets] = useState<Record<number, boolean>>({});

  const [isSubmitting, setIsSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Initialize or reset when catalogItem changes
  useEffect(() => {
    if (catalogItem) {
      setName(catalogItem.name.toLowerCase().replace(/[^a-z0-9_-]/g, '-'));
      setTransportType(catalogItem.transportType);
      setCommand(catalogItem.defaultConfig?.command || '');
      setArgsText(catalogItem.defaultConfig?.args?.join(' ') || '');
      setUrl(catalogItem.defaultConfig?.url || '');

      // Use the Go API's `env` metadata (description, required and isSecret).
      setEnvVars(catalogEnvRequirements(catalogItem));
    } else {
      // Custom server default
      setName('');
      setTransportType('stdio');
      setCommand('');
      setArgsText('');
      setUrl('');
      setEnvVars([]);
    }
    setError(null);
    setShowSecrets({});
  }, [catalogItem, isOpen]);

  if (!isOpen) return null;

  const handleAddEnv = () => {
    setEnvVars((prev) => [...prev, { key: '', value: '', description: '', required: false, isSecret: false }]);
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

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    if (!name.trim()) {
      setError(t('errors.nameRequired'));
      return;
    }

    if (transportType === 'stdio' && !command.trim()) {
      setError(t('errors.commandRequired'));
      return;
    }

    if (transportType !== 'stdio' && !url.trim()) {
      setError('URL is required for remote transports.');
      return;
    }

    setIsSubmitting(true);
    setError(null);

    try {
      const envMap: Record<string, string> = {};
      for (const item of envVars) {
        if (item.key.trim()) {
          envMap[item.key.trim()] = item.value;
        }
      }

      const missing = missingRequiredEnv(catalogEnvRequirements(catalogItem), envMap);
      if (missing.length) {
        setError(locale === 'fr'
          ? `Variable requise : ${missing.join(', ')}`
          : `Required variable: ${missing.join(', ')}`);
        return;
      }
      const parsedArgs = argsText
        .trim()
        .split(/\s+/)
        .filter(Boolean);

      await onInstall({
        catalogId: catalogItem?.id,
        name: name.trim(),
        transportType,
        command: transportType === 'stdio' ? command.trim() : undefined,
        args: transportType === 'stdio' ? parsedArgs : undefined,
        url: transportType !== 'stdio' ? url.trim() : undefined,
        env: Object.keys(envMap).length > 0 ? envMap : undefined,
      });

      onClose();
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : 'Failed to install server');
    } finally {
      setIsSubmitting(false);
    }
  };

  const description =
    catalogItem && locale === 'fr' && catalogItem.descriptionFr
      ? catalogItem.descriptionFr
      : catalogItem?.description;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-slate-950/80 backdrop-blur-md animate-fade-in">
      <div className="bg-slate-900 border border-slate-800 rounded-2xl w-full max-w-xl max-h-[90vh] flex flex-col shadow-2xl overflow-hidden">
        {/* Header */}
        <div className="flex items-center justify-between px-6 py-4 border-b border-slate-800 bg-slate-950/50">
          <div className="flex items-center gap-3">
            <div className="w-8 h-8 rounded-lg bg-gradient-to-tr from-cyan-500 to-blue-600 flex items-center justify-center text-white font-bold text-sm">
              <Plus className="w-4 h-4" />
            </div>
            <div>
              <h2 className="text-sm font-bold text-slate-100">
                {catalogItem
                  ? t('installModal.title', { name: catalogItem.name })
                  : t('common.addCustomServer')}
              </h2>
              <p className="text-xs text-slate-400">
                {catalogItem ? `by ${catalogItem.vendor || catalogItem.author || 'Community'}` : 'Configure standard I/O or remote endpoint'}
              </p>
            </div>
          </div>
          <button
            onClick={onClose}
            className="p-1.5 rounded-lg text-slate-400 hover:text-slate-200 hover:bg-slate-800 transition-colors"
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        {/* Content Body */}
        <form onSubmit={handleSubmit} className="flex-1 overflow-y-auto p-6 space-y-4">
          {error && (
            <div className="p-3 rounded-xl bg-rose-500/10 border border-rose-500/30 text-rose-300 text-xs flex items-center gap-2">
              <AlertCircle className="w-4 h-4 shrink-0" />
              <span>{error}</span>
            </div>
          )}

          {description && (
            <div className="p-3 rounded-xl bg-slate-950/60 border border-slate-800/80 text-xs text-slate-300 leading-relaxed">
              {description}
            </div>
          )}

          {/* Server Name */}
          <div>
            <label className="block text-xs font-semibold text-slate-200 mb-1">
              {t('installModal.serverName')} <span className="text-rose-400">*</span>
            </label>
            <input
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="e.g. github-tools, postgres-mcp"
              required
              className="w-full bg-slate-950/80 border border-slate-800 rounded-xl px-3.5 py-2 text-xs text-slate-100 placeholder-slate-500 focus:outline-none focus:ring-2 focus:ring-cyan-500/50"
            />
          </div>

          {/* Transport Type */}
          <div>
            <label className="block text-xs font-semibold text-slate-200 mb-1">
              {t('installModal.transportType')}
            </label>
            <div className="grid grid-cols-4 gap-2">
              {(['stdio', 'sse', 'websocket', 'http'] as const).map((tType) => (
                <button
                  key={tType}
                  type="button"
                  onClick={() => setTransportType(tType)}
                  className={`py-2 px-2.5 rounded-xl text-xs font-semibold border flex items-center justify-center gap-1.5 transition-all cursor-pointer ${
                    transportType === tType
                      ? 'bg-cyan-500/20 text-cyan-300 border-cyan-500/50 shadow-sm shadow-cyan-500/10'
                      : 'bg-slate-950/60 text-slate-400 border-slate-800 hover:text-slate-200 hover:bg-slate-800'
                  }`}
                >
                  {tType === 'stdio' ? (
                    <Terminal className="w-3 h-3" />
                  ) : (
                    <Radio className="w-3 h-3" />
                  )}
                  <span>{tType}</span>
                </button>
              ))}
            </div>
          </div>

          {/* stdio details */}
          {transportType === 'stdio' ? (
            <>
              <div>
                <label className="block text-xs font-semibold text-slate-200 mb-1">
                  {t('installModal.command')} <span className="text-rose-400">*</span>
                </label>
                <input
                  type="text"
                  value={command}
                  onChange={(e) => setCommand(e.target.value)}
                  placeholder="e.g. npx, uvx, node, docker"
                  className="w-full bg-slate-950/80 border border-slate-800 rounded-xl px-3.5 py-2 text-xs font-mono text-slate-100 placeholder-slate-500 focus:outline-none focus:ring-2 focus:ring-cyan-500/50"
                />
              </div>

              <div>
                <label className="block text-xs font-semibold text-slate-200 mb-1">
                  {t('installModal.args')}
                </label>
                <input
                  type="text"
                  value={argsText}
                  onChange={(e) => setArgsText(e.target.value)}
                  placeholder="e.g. -y @modelcontextprotocol/server-github"
                  className="w-full bg-slate-950/80 border border-slate-800 rounded-xl px-3.5 py-2 text-xs font-mono text-slate-100 placeholder-slate-500 focus:outline-none focus:ring-2 focus:ring-cyan-500/50"
                />
              </div>
            </>
          ) : (
            <div>
              <label className="block text-xs font-semibold text-slate-200 mb-1">
                {t('installModal.url')} <span className="text-rose-400">*</span>
              </label>
              <input
                type="url"
                value={url}
                onChange={(e) => setUrl(e.target.value)}
                placeholder="http://localhost:8080/sse or ws://..."
                className="w-full bg-slate-950/80 border border-slate-800 rounded-xl px-3.5 py-2 text-xs font-mono text-slate-100 placeholder-slate-500 focus:outline-none focus:ring-2 focus:ring-cyan-500/50"
              />
            </div>
          )}

          {/* Environment Variables */}
          <div>
            <div className="flex items-center justify-between mb-2">
              <label className="text-xs font-semibold text-slate-200 flex items-center gap-1.5">
                <span>{t('installModal.envVars')}</span>
                <span className="text-[10px] text-slate-400 font-normal">
                  ({'${VAR}'} from proxy environment; literal secrets are saved in plaintext)
                </span>
              </label>
              <button
                type="button"
                onClick={handleAddEnv}
                className="text-xs text-cyan-400 hover:text-cyan-300 flex items-center gap-1 font-medium cursor-pointer"
              >
                <Plus className="w-3.5 h-3.5" />
                <span>Add Variable</span>
              </button>
            </div>

            <div className="space-y-2">
              {envVars.map((envItem, index) => {
                const isSecret = envItem.isSecret;
                const isVisible = showSecrets[index];

                return (
                  <div key={index} className="space-y-1">
                  <div className="flex items-center gap-2">
                    <input
                      type="text"
                      value={envItem.key}
                      onChange={(e) => handleEnvChange(index, 'key', e.target.value)}
                      placeholder="KEY (e.g. GITHUB_TOKEN)"
                      className="w-1/3 bg-slate-950/80 border border-slate-800 rounded-xl px-3 py-1.5 text-xs font-mono text-slate-100 placeholder-slate-500 focus:outline-none focus:ring-1 focus:ring-cyan-500/50"
                    />
                    <div className="relative flex-1">
                      <input
                        type={isSecret && !isVisible ? 'password' : 'text'}
                        value={envItem.value}
                        onChange={(e) => handleEnvChange(index, 'value', e.target.value)}
                        placeholder="VALUE (or ${GITHUB_PERSONAL_ACCESS_TOKEN})"
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
                  {(envItem.description || envItem.required) && (
                    <p className="text-[10px] text-slate-400 px-1">
                      {envItem.description}{envItem.required && <span className="text-rose-400"> *</span>}
                    </p>
                  )}
                  </div>
                );
              })}

              {envVars.length === 0 && (
                <div className="text-center py-4 border border-dashed border-slate-800 rounded-xl text-xs text-slate-500">
                  No environment variables configured.
                </div>
              )}
            </div>
          </div>
        </form>

        {/* Footer */}
        <div className="flex items-center justify-end gap-3 px-6 py-4 border-t border-slate-800 bg-slate-950/50">
          <button
            type="button"
            onClick={onClose}
            disabled={isSubmitting}
            className="px-4 py-2 rounded-xl text-xs font-semibold text-slate-400 hover:text-slate-200 hover:bg-slate-800 border border-slate-800 transition-colors cursor-pointer"
          >
            {t('common.cancel')}
          </button>
          <button
            onClick={handleSubmit}
            disabled={isSubmitting}
            className="px-5 py-2 rounded-xl text-xs font-semibold bg-gradient-to-r from-cyan-500 to-blue-600 hover:from-cyan-400 hover:to-blue-500 text-white shadow-lg shadow-cyan-500/25 border border-cyan-400/30 transition-all disabled:opacity-50 cursor-pointer flex items-center gap-2"
          >
            {isSubmitting ? (
              <>
                <span className="w-3.5 h-3.5 border-2 border-white/30 border-t-white rounded-full animate-spin" />
                <span>{t('installModal.installing')}</span>
              </>
            ) : (
              <span>{t('installModal.submit')}</span>
            )}
          </button>
        </div>
      </div>
    </div>
  );
}
