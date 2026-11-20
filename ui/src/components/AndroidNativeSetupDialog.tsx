import React, { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Download, AlertTriangle, Eye, EyeOff } from 'lucide-react';
import { Modal } from './Modal';
import { CopyButton } from './CopyButton';
import { AndroidNativeSetup, downloadAndroidCredential, fetchAndroidNativeSetup } from '../lib/androidSetup';

interface Props {
  isOpen: boolean;
  onClose: () => void;
  deviceId: string;
  hostname: string;
}

// No APK, ADB, or imported third-party VPN profile is involved.
export const AndroidNativeSetupDialog: React.FC<Props> = ({ isOpen, onClose, deviceId, hostname }) => {
  const { t } = useTranslation();
  const [setup, setSetup] = useState<AndroidNativeSetup | null>(null);
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);
  const [showImportPassword, setShowImportPassword] = useState(false);

  useEffect(() => {
    setShowImportPassword(false);
    if (!isOpen || !deviceId) {
      setSetup(null);
      setError('');
      return;
    }
    const controller = new AbortController();
    setLoading(true);
    setSetup(null);
    setError('');
    fetchAndroidNativeSetup(deviceId, controller.signal)
      .then((data) => { if (!controller.signal.aborted) setSetup(data); })
      .catch((cause: unknown) => {
        if (!controller.signal.aborted) setError(cause instanceof Error ? cause.message : t('resources.android_setup_error'));
      })
      .finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => controller.abort();
  }, [deviceId, isOpen, t]);

  const field = (label: string, value: string) => (
    <div className="flex items-center justify-between gap-3 py-1.5 border-b subtle-border last:border-0">
      <span className="text-xs text-subtle">{label}</span>
      <span className="flex items-center gap-1 min-w-0">
        <code className="text-xs font-mono break-all text-right">{value}</code>
        <CopyButton value={value} label={label} />
      </span>
    </div>
  );

  return (
    <Modal isOpen={isOpen} onClose={onClose} title={t('resources.android_setup_title')} maxWidth="max-w-xl" error={error}>
      <div className="flex flex-wrap items-center gap-2 text-xs">
        <span className="text-subtle">{t('resources.device_hostname')}:</span>
        <code className="min-w-0 break-all font-mono text-current">{hostname}</code>
      </div>
      {loading && <p role="status" className="text-sm text-subtle">{t('resources.android_setup_loading')}</p>}
      {setup && (
        <div className="space-y-4">
          <p className="text-xs text-subtle">{t('resources.android_setup_intro')}</p>
          <div role="note" className="flex items-start gap-2 rounded-md border border-amber-500/30 bg-amber-500/10 p-3 text-xs">
            <AlertTriangle className="w-4 h-4 text-status-warning shrink-0" aria-hidden="true" />
            <span>{t('resources.android_security_note')}</span>
          </div>
          <ol className="list-decimal list-outside pl-5 space-y-4 marker:font-bold marker:text-accent">
            <li className="pl-1 space-y-2">
              <h3 className="text-xs font-semibold">{t('resources.android_step_certificate')}</h3>
              <button type="button"
                className="inline-flex items-center gap-2 px-3 py-2 rounded-md bg-blue-600 text-white text-xs font-medium hover:bg-blue-700"
                onClick={() => downloadAndroidCredential(setup.client_p12, `${hostname}.p12`, 'application/x-pkcs12')}>
                <Download className="w-4 h-4" /> {t('resources.android_download_p12')}
              </button>
              <div className="flex flex-col items-stretch gap-1.5 py-1.5 border-b subtle-border sm:flex-row sm:items-center sm:justify-between sm:gap-3">
                <span className="text-xs text-subtle">{t('resources.android_import_password')}</span>
                <span className="flex items-center gap-1 min-w-0 w-full sm:w-auto">
                  <code className="block flex-1 min-w-0 overflow-x-auto whitespace-nowrap text-xs font-mono sm:text-right">
                    {showImportPassword ? setup.import_password : '••••••••••••'}
                  </code>
                  <button
                    type="button"
                    onClick={() => setShowImportPassword(visible => !visible)}
                    aria-label={t(showImportPassword ? 'resources.android_hide_password' : 'resources.android_show_password')}
                    aria-pressed={showImportPassword}
                    className="shrink-0 p-1.5 rounded-md text-subtle hover:text-current hover:bg-slate-500/10 focus-visible:outline focus-visible:outline-2 focus-visible:outline-blue-500"
                  >
                    {showImportPassword ? <EyeOff className="w-3.5 h-3.5" /> : <Eye className="w-3.5 h-3.5" />}
                  </button>
                  <CopyButton value={setup.import_password} label={t('resources.android_import_password')} />
                </span>
              </div>
            <p className="text-[11px] text-subtle">{t('resources.android_password_masked_hint')}</p>
            </li>
            <li className="pl-1 space-y-2">
              <h3 className="text-xs font-semibold">{t('resources.android_step_connection')}</h3>
              {field(t('resources.android_server_address'), setup.server_address)}
              {field(t('resources.android_server_identity'), setup.server_identity)}
              {field(t('resources.android_client_identity'), setup.client_identity)}
            </li>
            <li className="pl-1 space-y-2">
              <h3 className="text-xs font-semibold">{t('resources.android_step_ca')}</h3>
              <button type="button"
                className="inline-flex items-center gap-2 px-3 py-2 rounded-md surface-input text-xs"
                onClick={() => downloadAndroidCredential(setup.root_ca, 'swanpanel-ca.cer', 'application/pkix-cert')}>
                <Download className="w-4 h-4" /> {t('resources.android_download_ca')}
              </button>
              <p className="text-[11px] text-subtle">{t('resources.android_ca_note')}</p>
            </li>
            <li className="pl-1 space-y-2">
              <h3 className="text-xs font-semibold">{t('resources.android_step_finish')}</h3>
              <p className="text-xs text-subtle">{t('resources.android_cleanup_note')}</p>
            </li>
          </ol>
          <p className="text-[11px] text-subtle">{t('resources.android_oem_note')}</p>
        </div>
      )}
    </Modal>
  );
};
