import { useState } from 'react';
import { useI18n } from '../i18n';
import { AlertTriangle } from 'lucide-react';

interface ConfirmModalProps {
  isOpen: boolean;
  onClose: () => void;
  onConfirm: () => Promise<void>;
  title: string;
  message: string;
  confirmLabel?: string;
  isDestructive?: boolean;
}

export function ConfirmModal({
  isOpen,
  onClose,
  onConfirm,
  title,
  message,
  confirmLabel,
  isDestructive = true,
}: ConfirmModalProps) {
  const { t } = useI18n();
  const [isSubmitting, setIsSubmitting] = useState(false);

  if (!isOpen) return null;

  const handleConfirm = async () => {
    setIsSubmitting(true);
    try {
      await onConfirm();
      onClose();
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-slate-950/80 backdrop-blur-md animate-fade-in">
      <div className="bg-slate-900 border border-slate-800 rounded-2xl w-full max-w-md shadow-2xl overflow-hidden">
        <div className="p-6">
          <div className="flex items-center gap-3.5 mb-4">
            <div
              className={`w-10 h-10 rounded-xl flex items-center justify-center shrink-0 ${
                isDestructive
                  ? 'bg-rose-500/10 border border-rose-500/20 text-rose-400'
                  : 'bg-cyan-500/10 border border-cyan-500/20 text-cyan-400'
              }`}
            >
              <AlertTriangle className="w-5 h-5" />
            </div>
            <div>
              <h3 className="text-sm font-bold text-slate-100">{title}</h3>
              <p className="text-xs text-slate-400 mt-0.5">Please confirm your action</p>
            </div>
          </div>

          <p className="text-xs text-slate-300 leading-relaxed bg-slate-950/60 border border-slate-800/80 rounded-xl p-3.5 mb-6">
            {message}
          </p>

          <div className="flex items-center justify-end gap-3">
            <button
              type="button"
              onClick={onClose}
              disabled={isSubmitting}
              className="px-4 py-2 rounded-xl text-xs font-semibold text-slate-400 hover:text-slate-200 hover:bg-slate-800 border border-slate-800 transition-colors"
            >
              {t('common.cancel')}
            </button>
            <button
              type="button"
              onClick={handleConfirm}
              disabled={isSubmitting}
              className={`px-5 py-2 rounded-xl text-xs font-semibold text-white shadow-lg transition-all disabled:opacity-50 flex items-center gap-2 ${
                isDestructive
                  ? 'bg-gradient-to-r from-rose-500 to-red-600 hover:from-rose-400 hover:to-red-500 shadow-rose-500/20 border border-rose-400/30'
                  : 'bg-gradient-to-r from-cyan-500 to-blue-600 hover:from-cyan-400 hover:to-blue-500 shadow-cyan-500/20 border border-cyan-400/30'
              }`}
            >
              {isSubmitting ? (
                <>
                  <span className="w-3.5 h-3.5 border-2 border-white/30 border-t-white rounded-full animate-spin" />
                  <span>Processing...</span>
                </>
              ) : (
                <span>{confirmLabel || t('common.delete')}</span>
              )}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
