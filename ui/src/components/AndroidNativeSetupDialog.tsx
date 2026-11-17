import React, { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Download, AlertTriangle } from 'lucide-react';
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

  useEffect(() => {
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
    <Modal isOpen={isOpen} onClose={onClose} title={`${t('resources.android_setup_title')}: ${hostname}`} maxWidth="max-w-lg" error={error}>
      {loading && <p role="status" className="text-sm text-subtle">{t('resources.android_setup_loading')}</p>}
      {setup && (
        <div className="space-y-4">
          <p className="text-xs text-subtle">{t('resources.android_setup_intro')}</p>
          <div className="space-y-2">
            <p className="text-xs font-semibold">{t('resources.android_step_certificate')}</p>
            <button type="button"
              className="inline-flex items-center gap-2 px-3 py-2 rounded-md bg-blue-600 text-white text-xs font-medium hover:bg-blue-700"
              onClick={() => downloadAndroidCredential(setup.client_p12, `${hostname}.p12`, 'application/x-pkcs12')}>
              <Download className="w-4 h-4" /> {t('resources.android_download_p12')}
            </button>
            {field(t('resources.android_import_password'), setup.import_password)}
          </div>
          <div className="space-y-2">
            <p className="text-xs font-semibold">{t('resources.android_step_connection')}</p>
            {field(t('resources.android_server_address'), setup.server_address)}
            {field(t('resources.android_server_identity'), setup.server_identity)}
            {field(t('resources.android_client_identity'), setup.client_identity)}
          </div>
          <div className="space-y-2">
            <p className="text-xs font-semibold">{t('resources.android_step_ca')}</p>
            <button type="button"
              className="inline-flex items-center gap-2 px-3 py-2 rounded-md surface-input text-xs"
              onClick={() => downloadAndroidCredential(setup.root_ca, 'swanpanel-ca.cer', 'application/pkix-cert')}>
              <Download className="w-4 h-4" /> {t('resources.android_download_ca')}
            </button>
            <p className="text-[11px] text-subtle">{t('resources.android_ca_note')}</p>
          </div>
          <div className="flex items-start gap-2 rounded-md border border-amber-500/30 bg-amber-500/10 p-3 text-xs">
            <AlertTriangle className="w-4 h-4 text-amber-500 shrink-0" />
            <span>{t('resources.android_security_note')}</span>
          </div>
          <p className="text-[11px] text-subtle">{t('resources.android_oem_note')}</p>
        </div>
      )}
    </Modal>
  );
};
