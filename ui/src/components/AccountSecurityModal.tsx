import React, { useId, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { CheckCircle2, Download, Fingerprint, Lock, QrCode, ShieldOff } from 'lucide-react';
import { Modal } from './Modal';
import { CopyButton } from './CopyButton';
import { QRCodeSVG } from './QRCodeSVG';
import { useToast } from './ToastProvider';
import { apiJSON } from '../lib/api';
import type { CurrentUser } from '../types';

interface AccountSecurityModalProps {
  isOpen: boolean;
  onClose: () => void;
  currentUser: CurrentUser | null;
}

interface MFAStatusResponse {
  supported: boolean;
  mfa_enabled: boolean;
  backup_codes_remaining: number;
}

interface MFASetupResponse {
  secret: string;
  otpauth_uri: string;
  issuer: string;
  account: string;
}

export const AccountSecurityModal: React.FC<AccountSecurityModalProps> = ({
  isOpen,
  onClose,
  currentUser,
}) => {
  const { t } = useTranslation();
  const toast = useToast();
  const queryClient = useQueryClient();
  const mfaHeadingId = useId();
  const pwdHeadingId = useId();

  const [setupPassword, setSetupPassword] = useState('');
  const [setupPayload, setSetupPayload] = useState<MFASetupResponse | null>(null);
  const [enrollCode, setEnrollCode] = useState('');
  const [backupCodes, setBackupCodes] = useState<string[] | null>(null);

  const [manageMode, setManageMode] = useState<'none' | 'regenerate' | 'disable'>('none');
  const [managePassword, setManagePassword] = useState('');
  const [manageCode, setManageCode] = useState('');

  const [currentPassword, setCurrentPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [passwordError, setPasswordError] = useState('');

  const statusQuery = useQuery<MFAStatusResponse>({
    queryKey: ['mfa_status'],
    queryFn: () => apiJSON('/api/auth/mfa/status'),
    enabled: isOpen && Boolean(currentUser),
  });

  const setupMutation = useMutation({
    mutationFn: () =>
      apiJSON<MFASetupResponse>('/api/auth/mfa/setup', {
        method: 'POST',
        body: JSON.stringify({ current_password: setupPassword }),
      }),
    onSuccess: (res) => {
      setSetupPassword('');
      setSetupPayload(res);
      setEnrollCode('');
    },
  });

  const enableMutation = useMutation({
    mutationFn: () =>
      apiJSON<{ mfa_enabled: boolean; backup_codes: string[] }>('/api/auth/mfa/enable', {
        method: 'POST',
        body: JSON.stringify({ code: enrollCode.trim() }),
      }),
    onSuccess: (res) => {
      setSetupPayload(null);
      setEnrollCode('');
      setBackupCodes(res.backup_codes);
      void queryClient.invalidateQueries({ queryKey: ['mfa_status'] });
      toast.success(t('mfa.enabled_success'));
    },
  });

  const regenerateMutation = useMutation({
    mutationFn: () =>
      apiJSON<{ backup_codes: string[] }>('/api/auth/mfa/backup-codes', {
        method: 'POST',
        body: JSON.stringify({ current_password: managePassword, code: manageCode.trim() }),
      }),
    onSuccess: (res) => {
      setManageMode('none');
      setManagePassword('');
      setManageCode('');
      setBackupCodes(res.backup_codes);
      void queryClient.invalidateQueries({ queryKey: ['mfa_status'] });
      toast.success(t('mfa.backup_regenerated_success'));
    },
  });

  const disableMutation = useMutation({
    mutationFn: () =>
      apiJSON<{ mfa_enabled: boolean }>('/api/auth/mfa/disable', {
        method: 'POST',
        body: JSON.stringify({ current_password: managePassword, code: manageCode.trim() }),
      }),
    onSuccess: () => {
      setManageMode('none');
      setManagePassword('');
      setManageCode('');
      setBackupCodes(null);
      void queryClient.invalidateQueries({ queryKey: ['mfa_status'] });
      toast.success(t('mfa.disabled_success'));
    },
  });

  const passwordMutation = useMutation({
    mutationFn: () =>
      apiJSON<{ message: string }>('/api/auth/password', {
        method: 'PUT',
        body: JSON.stringify({ current_password: currentPassword, new_password: newPassword }),
      }),
    onSuccess: () => {
      setCurrentPassword('');
      setNewPassword('');
      setConfirmPassword('');
      setPasswordError('');
      toast.success(t('mfa.password_updated'));
    },
  });

  const isAnyPending =
    setupMutation.isPending ||
    enableMutation.isPending ||
    regenerateMutation.isPending ||
    disableMutation.isPending ||
    passwordMutation.isPending;

  const activeError =
    setupMutation.error?.message ||
    enableMutation.error?.message ||
    regenerateMutation.error?.message ||
    disableMutation.error?.message ||
    passwordMutation.error?.message ||
    passwordError ||
    null;

  const handleClose = () => {
    if (isAnyPending) return;
    setSetupPassword('');
    setSetupPayload(null);
    setEnrollCode('');
    setManageMode('none');
    setManagePassword('');
    setManageCode('');
    setPasswordError('');
    setupMutation.reset();
    enableMutation.reset();
    regenerateMutation.reset();
    disableMutation.reset();
    passwordMutation.reset();
    onClose();
  };

  const handleDownloadBackupCodes = () => {
    if (!backupCodes) return;
    const text = [
      `SwanPanel Recovery Codes (${currentUser?.email || ''})`,
      `Generated: ${new Date().toISOString()}`,
      '',
      ...backupCodes,
    ].join('\n');
    const blob = new Blob([text], { type: 'text/plain;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    try {
      const a = document.createElement('a');
      a.href = url;
      a.download = 'swanpanel-recovery-codes.txt';
      document.body.appendChild(a);
      a.click();
      a.remove();
    } finally {
      window.setTimeout(() => URL.revokeObjectURL(url), 1000);
    }
  };

  const handlePasswordSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    setPasswordError('');
    if (newPassword.length < 12 || newPassword.length > 72) {
      setPasswordError(t('mfa.password_length_error'));
      return;
    }
    if (newPassword !== confirmPassword) {
      setPasswordError(t('mfa.password_mismatch'));
      return;
    }
    passwordMutation.mutate();
  };

  const mfaStatus = statusQuery.data;
  const isLocal = mfaStatus?.supported ?? currentUser?.auth_type !== 'oidc';

  return (
    <Modal
      isOpen={isOpen}
      onClose={handleClose}
      title={t('mfa.account_security')}
      error={activeError}
      closeDisabled={isAnyPending}
      maxWidth="max-w-lg"
    >
      <div className="space-y-5 text-xs">
        {/* TWO-FACTOR AUTHENTICATION SECTION */}
        <section aria-labelledby={mfaHeadingId} className="space-y-3">
          <div className="flex items-center justify-between gap-2">
            <h3 id={mfaHeadingId} className="font-semibold text-sm flex items-center gap-2">
              <Fingerprint className="w-4 h-4 text-accent" />
              {t('mfa.section_title')}
            </h3>
            {isLocal && mfaStatus && (
              <span
                className={`px-2 py-0.5 rounded-md border text-[10px] font-semibold ${
                  mfaStatus.mfa_enabled
                    ? 'bg-emerald-500/10 text-status-success border-emerald-500/30'
                    : 'bg-slate-500/10 text-subtle subtle-border'
                }`}
              >
                {mfaStatus.mfa_enabled ? t('mfa.status_enabled') : t('mfa.status_disabled')}
              </span>
            )}
          </div>

          {!isLocal ? (
            <div role="note" className="rounded-lg border subtle-border surface-header p-3 text-subtle">
              {t('mfa.sso_managed_notice')}
            </div>
          ) : backupCodes ? (
            <div className="rounded-lg border border-amber-500/30 bg-amber-500/10 p-4 space-y-3">
              <div className="flex items-start gap-2 text-status-warning font-semibold">
                <CheckCircle2 className="w-4 h-4 shrink-0 mt-0.5" />
                <span>{t('mfa.backup_codes_warning')}</span>
              </div>
              <div className="grid grid-cols-2 gap-2 font-mono text-xs bg-slate-950/60 text-slate-100 p-3 rounded-lg border subtle-border">
                {backupCodes.map((code) => (
                  <div key={code} className="text-center tracking-wider py-0.5">
                    {code}
                  </div>
                ))}
              </div>
              <div className="flex flex-wrap items-center justify-between gap-2">
                <div className="flex items-center gap-2">
                  <CopyButton value={backupCodes.join('\n')} label={t('mfa.copy_codes')} />
                  <button
                    type="button"
                    onClick={handleDownloadBackupCodes}
                    className="inline-flex items-center gap-1.5 px-2.5 py-1.5 rounded-lg border subtle-border hover:bg-slate-500/10 text-xs"
                  >
                    <Download className="w-3.5 h-3.5" />
                    {t('mfa.download_codes')}
                  </button>
                </div>
                <button
                  type="button"
                  onClick={() => setBackupCodes(null)}
                  className="rounded-lg bg-blue-600 hover:bg-blue-500 text-white px-3 py-1.5 font-semibold"
                >
                  {t('mfa.saved_codes_done')}
                </button>
              </div>
            </div>
          ) : setupPayload ? (
            <div className="rounded-xl border subtle-border surface-header p-4 sm:p-5 space-y-4">
              <p className="text-xs text-subtle leading-relaxed">{t('mfa.setup_step1_hint')}</p>

              {/* Centered QR Code */}
              <div className="flex flex-col items-center justify-center">
                <div className="bg-white p-3 rounded-xl border border-slate-200 dark:border-slate-700 shadow-xs inline-flex items-center justify-center">
                  <QRCodeSVG
                    value={setupPayload.otpauth_uri}
                    size={160}
                    label={t('mfa.qr_code_alt')}
                  />
                </div>
              </div>

              {/* Secret Key & URI cleanly below QR Code */}
              <div className="space-y-2">
                <div className="flex items-center justify-between gap-3 rounded-lg border subtle-border surface-card px-3 py-2.5">
                  <div className="min-w-0 flex-1">
                    <div className="text-[10px] font-semibold text-dim uppercase tracking-wider mb-0.5">
                      {t('mfa.secret_key_label')}
                    </div>
                    <div className="font-mono text-xs font-semibold tracking-wider text-current break-all select-all">
                      {setupPayload.secret}
                    </div>
                  </div>
                  <CopyButton value={setupPayload.secret} />
                </div>
                <div className="flex items-center justify-between gap-3 rounded-lg border subtle-border surface-card px-3 py-2">
                  <div className="min-w-0 flex-1">
                    <div className="text-[10px] font-semibold text-dim uppercase tracking-wider mb-0.5">
                      {t('mfa.otpauth_uri_label')}
                    </div>
                    <div className="font-mono text-[11px] text-subtle truncate select-all">
                      {setupPayload.otpauth_uri}
                    </div>
                  </div>
                  <CopyButton value={setupPayload.otpauth_uri} />
                </div>
              </div>

              <form
                onSubmit={(e) => {
                  e.preventDefault();
                  if (!enableMutation.isPending && enrollCode.trim()) {
                    enableMutation.mutate();
                  }
                }}
                method="post"
                className="space-y-2.5 pt-3 border-t subtle-border"
              >
                <div className="space-y-0.5">
                  <label htmlFor="mfa-enroll-code" className="block font-semibold text-xs text-current">
                    {t('mfa.enter_code_label')}
                  </label>
                  <p id="mfa-enroll-hint" className="text-[11px] text-dim">
                    {t('mfa.six_digit_hint')}
                  </p>
                </div>
                <div className="flex flex-col sm:flex-row items-stretch sm:items-center gap-2.5">
                  <input
                    id="mfa-enroll-code"
                    name="code"
                    type="text"
                    inputMode="numeric"
                    autoComplete="one-time-code"
                    autoCorrect="off"
                    autoCapitalize="none"
                    spellCheck={false}
                    pattern="\d{6}"
                    maxLength={6}
                    required
                    aria-describedby="mfa-enroll-hint"
                    value={enrollCode}
                    onChange={(e) => setEnrollCode(e.target.value)}
                    placeholder="123456"
                    className="flex-1 h-9 surface-input border subtle-border rounded-lg px-3 font-mono text-sm text-center tracking-[0.25em] focus:outline-none focus:border-blue-500"
                  />
                  <button
                    type="submit"
                    disabled={enableMutation.isPending || enrollCode.trim().length < 6}
                    className="h-9 shrink-0 rounded-lg bg-blue-600 hover:bg-blue-500 text-white px-4 font-semibold text-xs transition-colors disabled:opacity-50 inline-flex items-center justify-center"
                  >
                    {enableMutation.isPending ? t('common.saving') : t('mfa.activate_button')}
                  </button>
                </div>
              </form>
            </div>
          ) : mfaStatus?.mfa_enabled ? (
            <div className="rounded-lg border subtle-border surface-header p-3.5 space-y-3">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <span className="text-subtle">
                  {t('mfa.backup_remaining', { count: mfaStatus.backup_codes_remaining })}
                </span>
                <div className="flex flex-wrap gap-2">
                  <button
                    type="button"
                    onClick={() => {
                      setManageMode((m) => (m === 'regenerate' ? 'none' : 'regenerate'));
                      setManagePassword('');
                      setManageCode('');
                      regenerateMutation.reset();
                      disableMutation.reset();
                    }}
                    className="rounded-lg border subtle-border px-2.5 py-1.5 hover:bg-slate-500/10 font-medium"
                  >
                    {t('mfa.regenerate_codes')}
                  </button>
                  <button
                    type="button"
                    onClick={() => {
                      setManageMode((m) => (m === 'disable' ? 'none' : 'disable'));
                      setManagePassword('');
                      setManageCode('');
                      regenerateMutation.reset();
                      disableMutation.reset();
                    }}
                    className="rounded-lg border border-rose-500/30 bg-rose-500/10 text-status-danger px-2.5 py-1.5 hover:bg-rose-500/20 font-medium inline-flex items-center gap-1"
                  >
                    <ShieldOff className="w-3.5 h-3.5" />
                    {t('mfa.disable_button')}
                  </button>
                </div>
              </div>

              {manageMode !== 'none' && (
                <form
                  onSubmit={(e) => {
                    e.preventDefault();
                    if (manageMode === 'regenerate') regenerateMutation.mutate();
                    else disableMutation.mutate();
                  }}
                  method="post"
                  className="space-y-2.5 pt-2.5 border-t subtle-border"
                >
                  <input
                    type="email"
                    name="username"
                    autoComplete="username"
                    value={currentUser?.email || ''}
                    readOnly
                    hidden
                    tabIndex={-1}
                    aria-hidden="true"
                  />
                  <div>
                    <label htmlFor="mfa-manage-password" className="block mb-1 text-subtle">
                      {t('mfa.current_password')}
                    </label>
                    <input
                      id="mfa-manage-password"
                      name="current_password"
                      type="password"
                      autoComplete="current-password"
                      required
                      value={managePassword}
                      onChange={(e) => setManagePassword(e.target.value)}
                      className="w-full surface-input border subtle-border rounded-lg px-3 py-2 text-xs"
                    />
                  </div>
                  <div>
                    <label htmlFor="mfa-manage-code" className="block mb-1 text-subtle">
                      {t('mfa.enter_code_label')}
                    </label>
                    <p id="mfa-manage-hint" className="text-[11px] text-dim mb-1">
                      {manageMode === 'regenerate' ? t('mfa.six_digit_hint') : t('mfa.code_or_backup_hint')}
                    </p>
                    <input
                      id="mfa-manage-code"
                      name="code"
                      type="text"
                      inputMode={manageMode === 'regenerate' ? 'numeric' : 'text'}
                      autoCapitalize={manageMode === 'regenerate' ? 'none' : 'characters'}
                      autoCorrect="off"
                      spellCheck={false}
                      autoComplete="one-time-code"
                      required
                      aria-describedby="mfa-manage-hint"
                      value={manageCode}
                      onChange={(e) => setManageCode(e.target.value)}
                      className="w-full surface-input border subtle-border rounded-lg px-3 py-2 font-mono text-xs"
                    />
                  </div>
                  <div className="flex justify-end gap-2">
                    <button
                      type="button"
                      onClick={() => setManageMode('none')}
                      className="rounded-lg border subtle-border px-3 py-1.5"
                    >
                      {t('common.cancel')}
                    </button>
                    <button
                      type="submit"
                      disabled={regenerateMutation.isPending || disableMutation.isPending}
                      className={`rounded-lg px-3 py-1.5 text-white font-semibold disabled:opacity-50 ${
                        manageMode === 'disable' ? 'bg-rose-600 hover:bg-rose-500' : 'bg-blue-600 hover:bg-blue-500'
                      }`}
                    >
                      {t('common.confirm')}
                    </button>
                  </div>
                </form>
              )}
            </div>
          ) : (
            <form
              onSubmit={(e) => {
                e.preventDefault();
                if (!setupMutation.isPending && setupPassword) setupMutation.mutate();
              }}
              method="post"
              className="rounded-lg border subtle-border surface-header p-3.5 space-y-2.5"
            >
              <input
                type="email"
                name="username"
                autoComplete="username"
                value={currentUser?.email || ''}
                readOnly
                hidden
                tabIndex={-1}
                aria-hidden="true"
              />
              <p className="text-subtle">{t('mfa.setup_intro')}</p>
              <div>
                <label htmlFor="mfa-setup-password" className="block mb-1 text-subtle">
                  {t('mfa.current_password')}
                </label>
                <div className="flex gap-2">
                  <input
                    id="mfa-setup-password"
                    name="current_password"
                    type="password"
                    autoComplete="current-password"
                    required
                    value={setupPassword}
                    onChange={(e) => setSetupPassword(e.target.value)}
                    placeholder={t('auth.password_placeholder')}
                    className="flex-1 surface-input border subtle-border rounded-lg px-3 py-2 text-xs"
                  />
                  <button
                    type="submit"
                    disabled={setupMutation.isPending || !setupPassword}
                    className="rounded-lg bg-blue-600 hover:bg-blue-500 text-white px-3 py-2 font-semibold inline-flex items-center gap-1.5 disabled:opacity-50"
                  >
                    <QrCode className="w-3.5 h-3.5" />
                    {t('mfa.begin_setup')}
                  </button>
                </div>
              </div>
            </form>
          )}
        </section>

        {/* CHANGE PASSWORD SECTION (LOCAL ACCOUNTS) */}
        {isLocal && (
          <section aria-labelledby={pwdHeadingId} className="pt-4 border-t subtle-border space-y-3">
            <h3 id={pwdHeadingId} className="font-semibold text-sm flex items-center gap-2">
              <Lock className="w-4 h-4 text-accent" />
              {t('mfa.change_password_title')}
            </h3>
            <form onSubmit={handlePasswordSubmit} method="post" className="space-y-2.5">
              <input
                type="email"
                name="username"
                autoComplete="username"
                value={currentUser?.email || ''}
                readOnly
                hidden
                tabIndex={-1}
                aria-hidden="true"
              />
              <div>
                <label htmlFor="sec-current-pw" className="block mb-1 text-subtle">
                  {t('mfa.current_password')}
                </label>
                <input
                  id="sec-current-pw"
                  name="current_password"
                  type="password"
                  autoComplete="current-password"
                  required
                  value={currentPassword}
                  onChange={(e) => setCurrentPassword(e.target.value)}
                  className="w-full surface-input border subtle-border rounded-lg px-3 py-2 text-xs"
                />
              </div>
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-2.5">
                <div>
                  <label htmlFor="sec-new-pw" className="block mb-1 text-subtle">
                    {t('mfa.new_password')}
                  </label>
                  <input
                    id="sec-new-pw"
                    name="new_password"
                    type="password"
                    autoComplete="new-password"
                    minLength={12}
                    maxLength={72}
                    required
                    value={newPassword}
                    onChange={(e) => setNewPassword(e.target.value)}
                    placeholder={t('auth.initial_password_hint')}
                    className="w-full surface-input border subtle-border rounded-lg px-3 py-2 text-xs"
                  />
                </div>
                <div>
                  <label htmlFor="sec-confirm-pw" className="block mb-1 text-subtle">
                    {t('mfa.confirm_new_password')}
                  </label>
                  <input
                    id="sec-confirm-pw"
                    name="confirm_password"
                    type="password"
                    autoComplete="new-password"
                    minLength={12}
                    maxLength={72}
                    required
                    value={confirmPassword}
                    onChange={(e) => setConfirmPassword(e.target.value)}
                    className="w-full surface-input border subtle-border rounded-lg px-3 py-2 text-xs"
                  />
                </div>
              </div>
              <div className="flex justify-end">
                <button
                  type="submit"
                  disabled={passwordMutation.isPending || !currentPassword || !newPassword || !confirmPassword}
                  className="rounded-lg border subtle-border hover:bg-slate-500/10 px-3 py-2 font-semibold disabled:opacity-50"
                >
                  {passwordMutation.isPending ? t('common.saving') : t('mfa.update_password_button')}
                </button>
              </div>
            </form>
          </section>
        )}
      </div>
    </Modal>
  );
};
