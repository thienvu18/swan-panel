import React, { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery, useMutation } from '@tanstack/react-query';
import { Check } from 'lucide-react';

interface ServerSettingsProps {
  onRegisterSaveButton?: (button: React.ReactNode) => void;
}

export const ServerSettingsPage: React.FC<ServerSettingsProps> = ({ onRegisterSaveButton }) => {
  const { t } = useTranslation();

  // State form
  const [reauthVal, setReauthVal] = useState('12');
  const [reauthUnit, setReauthUnit] = useState('h');
  const [rekeyIKE, setRekeyIKE] = useState('4h');
  const [rekeyChild, setRekeyChild] = useState('1h');
  const [inactivity, setInactivity] = useState('30m');

  const [mobike, setMobike] = useState(true);
  const [encap, setEncap] = useState(false);
  const [fragmentation, setFragmentation] = useState(true);

  const [dpdDelay, setDpdDelay] = useState('30s');
  const [dpdTimeout, setDpdTimeout] = useState('120s');
  const [dpdAction, setDpdAction] = useState('clear');

  const [serverFqdn, setServerFqdn] = useState('vpn.company.com');
  const [mssClamping, setMssClamping] = useState(1360);

  const [cipherPreset, setCipherPreset] = useState<'modern' | 'balanced' | 'custom'>('balanced');
  const [ikeProposals, setIkeProposals] = useState('aes256gcm16-prfsha384-ecp384,aes256-sha256-modp2048');
  const [espProposals, setEspProposals] = useState('aes256gcm16,aes128gcm16,chacha20poly1305');

  const [saveSuccess, setSaveSuccess] = useState(false);

  // Fetch configuration from API
  const { data } = useQuery({
    queryKey: ['strongswan_settings'],
    queryFn: async () => {
      const res = await fetch('/api/settings/strongswan', {
        headers: { Authorization: `Bearer ${localStorage.getItem('token') || ''}` },
      });
      if (!res.ok) return null;
      return res.json();
    },
  });

  useEffect(() => {
    if (data) {
      if (data.reauth_time) {
        setReauthVal(data.reauth_time.replace(/[^0-9]/g, '') || '12');
        setReauthUnit(data.reauth_time.replace(/[0-9]/g, '') || 'h');
      }
      if (data.rekey_time_ike) setRekeyIKE(data.rekey_time_ike);
      if (data.rekey_time_child) setRekeyChild(data.rekey_time_child);
      if (data.inactivity_timeout) setInactivity(data.inactivity_timeout);
      setMobike(data.mobike_enabled);
      setEncap(data.force_encap_natt);
      setFragmentation(data.fragmentation === 'yes');
      if (data.dpd_delay) setDpdDelay(data.dpd_delay);
      if (data.dpd_timeout) setDpdTimeout(data.dpd_timeout);
      if (data.dpd_action) setDpdAction(data.dpd_action);
      if (data.server_endpoint) setServerFqdn(data.server_endpoint);
      if (data.mss_clamping) setMssClamping(data.mss_clamping);
      if (data.cipher_preset) setCipherPreset(data.cipher_preset as any);
      if (data.ike_proposals) setIkeProposals(data.ike_proposals);
      if (data.esp_proposals) setEspProposals(data.esp_proposals);
    }
  }, [data]);

  const handlePresetChange = (preset: 'modern' | 'balanced' | 'custom') => {
    setCipherPreset(preset);
    if (preset === 'modern') {
      setIkeProposals('aes256gcm16-prfsha384-ecp384,chacha20poly1305-prfsha256-curve25519');
      setEspProposals('aes256gcm16,chacha20poly1305');
    } else if (preset === 'balanced') {
      setIkeProposals('aes256gcm16-prfsha384-ecp384,aes256-sha256-modp2048');
      setEspProposals('aes256gcm16,aes128gcm16,chacha20poly1305');
    }
  };

  const saveMutation = useMutation({
    mutationFn: async () => {
      const payload = {
        reauth_time: `${reauthVal}${reauthUnit}`,
        rekey_time_ike: rekeyIKE,
        rekey_time_child: rekeyChild,
        inactivity_timeout: inactivity,
        mobike_enabled: mobike,
        force_encap_natt: encap,
        fragmentation: fragmentation ? 'yes' : 'no',
        dpd_delay: dpdDelay,
        dpd_timeout: dpdTimeout,
        dpd_action: dpdAction,
        server_endpoint: serverFqdn,
        mss_clamping: Number(mssClamping),
        cipher_preset: cipherPreset,
        ike_proposals: ikeProposals,
        esp_proposals: espProposals,
      };

      const res = await fetch('/api/settings/strongswan', {
        method: 'PUT',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${localStorage.getItem('token') || ''}`,
        },
        body: JSON.stringify(payload),
      });

      if (!res.ok) throw new Error('Save failed');
      return res.json();
    },
    onSuccess: () => {
      setSaveSuccess(true);
      setTimeout(() => setSaveSuccess(false), 3000);
    },
  });

  // Register contextual Save Changes action button on Header
  useEffect(() => {
    if (onRegisterSaveButton) {
      onRegisterSaveButton(
        <button
          onClick={() => saveMutation.mutate()}
          disabled={saveMutation.isPending}
          className="bg-blue-600 hover:bg-blue-500 disabled:opacity-50 text-white font-medium px-3 py-1.5 rounded-lg transition-colors flex items-center gap-1.5 shadow-xs text-xs"
        >
          <Check className="w-3.5 h-3.5" />
          {saveMutation.isPending ? 'Saving...' : t('common.save_changes')}
        </button>
      );
    }
  }, [onRegisterSaveButton, saveMutation.isPending, t]);

  return (
    <div className="space-y-4">
      {saveSuccess && (
        <div className="p-3 bg-emerald-500/10 border border-emerald-500/30 text-emerald-400 rounded-xl text-xs flex items-center gap-2">
          <Check className="w-4 h-4" />
          <span>{t('server.applied_success')}</span>
        </div>
      )}

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
        {/* CARD 1: SESSION LIFETIMES & RE-AUTHENTICATION */}
        <div className="border subtle-border rounded-xl p-4 surface-card space-y-3.5 shadow-xs">
          <div className="border-b subtle-border pb-2">
            <h3 className="font-semibold text-xs text-slate-900 dark:text-white">{t('server.lifetimes_title')}</h3>
            <p className="text-[11px] text-dim">{t('server.lifetimes_desc')}</p>
          </div>

          <div className="space-y-3 text-xs">
            <div>
              <label className="text-subtle font-medium block mb-1">{t('server.max_session')}</label>
              <div className="flex gap-2">
                <input
                  type="number"
                  value={reauthVal}
                  onChange={(e) => setReauthVal(e.target.value)}
                  className="surface-input border subtle-border rounded-lg px-2.5 py-1.5 font-mono text-xs w-24 focus:outline-none focus:border-blue-500"
                />
                <select
                  value={reauthUnit}
                  onChange={(e) => setReauthUnit(e.target.value)}
                  className="surface-input border subtle-border rounded-lg px-2 py-1.5 text-xs focus:outline-none"
                >
                  <option value="h">{t('common.hours')}</option>
                  <option value="d">{t('common.days')}</option>
                  <option value="m">{t('common.minutes')}</option>
                </select>
                <div className="flex-1 flex items-center text-[11px] text-dim italic pl-1">
                  {t('server.max_session_hint')}
                </div>
              </div>
            </div>

            <div className="grid grid-cols-2 gap-3 pt-1 border-t subtle-border">
              <div>
                <label className="text-subtle font-medium block mb-1">{t('server.tunnel_key_renewal')}</label>
                <input
                  type="text"
                  value={rekeyIKE}
                  onChange={(e) => setRekeyIKE(e.target.value)}
                  className="w-full surface-input border subtle-border rounded-lg px-2.5 py-1.5 font-mono text-xs focus:outline-none"
                />
                <span className="text-[10px] text-dim block mt-0.5">{t('server.tunnel_key_renewal_hint')}</span>
              </div>

              <div>
                <label className="text-subtle font-medium block mb-1">{t('server.data_key_renewal')}</label>
                <input
                  type="text"
                  value={rekeyChild}
                  onChange={(e) => setRekeyChild(e.target.value)}
                  className="w-full surface-input border subtle-border rounded-lg px-2.5 py-1.5 font-mono text-xs focus:outline-none"
                />
                <span className="text-[10px] text-dim block mt-0.5">{t('server.data_key_renewal_hint')}</span>
              </div>
            </div>

            <div>
              <label className="text-subtle font-medium block mb-1">{t('server.idle_timeout')}</label>
              <div className="flex items-center gap-2">
                <input
                  type="text"
                  value={inactivity}
                  onChange={(e) => setInactivity(e.target.value)}
                  className="w-24 surface-input border subtle-border rounded-lg px-2.5 py-1.5 font-mono text-xs focus:outline-none"
                />
                <span className="text-[11px] text-dim">{t('server.idle_timeout_hint')}</span>
              </div>
            </div>
          </div>
        </div>

        {/* CARD 2: NETWORK ROAMING & TRAVERSAL */}
        <div className="border subtle-border rounded-xl p-4 surface-card space-y-3.5 shadow-xs">
          <div className="border-b subtle-border pb-2">
            <h3 className="font-semibold text-xs text-slate-900 dark:text-white">{t('server.roaming_title')}</h3>
            <p className="text-[11px] text-dim">{t('server.roaming_desc')}</p>
          </div>

          <div className="space-y-3 text-xs">
            {/* MOBIKE */}
            <div className="flex items-start justify-between gap-4 p-2 rounded-lg surface-header border subtle-border">
              <div>
                <div className="font-medium text-slate-900 dark:text-white">{t('server.mobike')}</div>
                <div className="text-[11px] text-dim mt-0.5">{t('server.mobike_hint')}</div>
              </div>
              <label className="relative inline-flex items-center cursor-pointer shrink-0 mt-1">
                <input
                  type="checkbox"
                  checked={mobike}
                  onChange={(e) => setMobike(e.target.checked)}
                  className="sr-only peer"
                />
                <div className="w-9 h-5 bg-slate-700 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-slate-300 after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-blue-600"></div>
              </label>
            </div>

            {/* NAT-T */}
            <div className="flex items-start justify-between gap-4 p-2 rounded-lg surface-header border subtle-border">
              <div>
                <div className="font-medium text-slate-900 dark:text-white">{t('server.force_encap')}</div>
                <div className="text-[11px] text-dim mt-0.5">{t('server.force_encap_hint')}</div>
              </div>
              <label className="relative inline-flex items-center cursor-pointer shrink-0 mt-1">
                <input
                  type="checkbox"
                  checked={encap}
                  onChange={(e) => setEncap(e.target.checked)}
                  className="sr-only peer"
                />
                <div className="w-9 h-5 bg-slate-700 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-slate-300 after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-blue-600"></div>
              </label>
            </div>

            {/* Fragmentation */}
            <div className="flex items-start justify-between gap-4 p-2 rounded-lg surface-header border subtle-border">
              <div>
                <div className="font-medium text-slate-900 dark:text-white">{t('server.fragmentation')}</div>
                <div className="text-[11px] text-dim mt-0.5">{t('server.fragmentation_hint')}</div>
              </div>
              <label className="relative inline-flex items-center cursor-pointer shrink-0 mt-1">
                <input
                  type="checkbox"
                  checked={fragmentation}
                  onChange={(e) => setFragmentation(e.target.checked)}
                  className="sr-only peer"
                />
                <div className="w-9 h-5 bg-slate-700 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-slate-300 after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-blue-600"></div>
              </label>
            </div>
          </div>
        </div>

        {/* CARD 3: CONNECTION HEALTH CHECK */}
        <div className="border subtle-border rounded-xl p-4 surface-card space-y-3.5 shadow-xs">
          <div className="border-b subtle-border pb-2">
            <h3 className="font-semibold text-xs text-slate-900 dark:text-white">{t('server.health_title')}</h3>
            <p className="text-[11px] text-dim">{t('server.health_desc')}</p>
          </div>

          <div className="grid grid-cols-1 sm:grid-cols-3 gap-3 text-xs">
            <div>
              <label className="text-subtle font-medium block mb-1">{t('server.ping_interval')}</label>
              <input
                type="text"
                value={dpdDelay}
                onChange={(e) => setDpdDelay(e.target.value)}
                className="w-full surface-input border subtle-border rounded-lg px-2.5 py-1.5 font-mono text-xs focus:outline-none"
              />
              <span className="text-[10px] text-dim block mt-0.5">{t('server.ping_interval_hint')}</span>
            </div>

            <div>
              <label className="text-subtle font-medium block mb-1">{t('server.timeout')}</label>
              <input
                type="text"
                value={dpdTimeout}
                onChange={(e) => setDpdTimeout(e.target.value)}
                className="w-full surface-input border subtle-border rounded-lg px-2.5 py-1.5 font-mono text-xs focus:outline-none"
              />
              <span className="text-[10px] text-dim block mt-0.5">{t('server.timeout_hint')}</span>
            </div>

            <div>
              <label className="text-subtle font-medium block mb-1">{t('server.offline_action')}</label>
              <select
                value={dpdAction}
                onChange={(e) => setDpdAction(e.target.value)}
                className="w-full surface-input border subtle-border rounded-lg px-2.5 py-1.5 text-xs focus:outline-none"
              >
                <option value="clear">{t('server.action_release')}</option>
                <option value="trap">{t('server.action_hold')}</option>
                <option value="restart">{t('server.action_retry')}</option>
              </select>
              <span className="text-[10px] text-dim block mt-0.5">{t('server.offline_action_hint')}</span>
            </div>
          </div>
        </div>

        {/* CARD 4: GATEWAY ENDPOINT & MTU TUNING */}
        <div className="border subtle-border rounded-xl p-4 surface-card space-y-3.5 shadow-xs">
          <div className="border-b subtle-border pb-2">
            <h3 className="font-semibold text-xs text-slate-900 dark:text-white">{t('server.endpoint_title')}</h3>
            <p className="text-[11px] text-dim">{t('server.endpoint_desc')}</p>
          </div>

          <div className="space-y-3 text-xs">
            <div>
              <label className="text-subtle font-medium block mb-1">{t('server.public_hostname')}</label>
              <input
                type="text"
                value={serverFqdn}
                onChange={(e) => setServerFqdn(e.target.value)}
                className="w-full surface-input border subtle-border rounded-lg px-2.5 py-1.5 font-mono text-xs focus:outline-none"
              />
              <span className="text-[11px] text-dim block mt-0.5">{t('server.public_hostname_hint')}</span>
            </div>

            <div className="flex items-center gap-3">
              <div className="w-1/2">
                <label className="text-subtle font-medium block mb-1">{t('server.mss_clamping')}</label>
                <input
                  type="number"
                  value={mssClamping}
                  onChange={(e) => setMssClamping(Number(e.target.value))}
                  className="w-full surface-input border subtle-border rounded-lg px-2.5 py-1.5 font-mono text-xs focus:outline-none"
                />
              </div>
              <div className="w-1/2 text-[11px] text-dim pt-4">{t('server.mss_clamping_hint')}</div>
            </div>
          </div>
        </div>
      </div>

      {/* CARD 5: ENCRYPTION & COMPATIBILITY */}
      <div className="border subtle-border rounded-xl p-4 surface-card space-y-3.5 shadow-xs text-xs">
        <div className="border-b subtle-border pb-2">
          <h3 className="font-semibold text-xs text-slate-900 dark:text-white">{t('server.ciphers_title')}</h3>
          <p className="text-[11px] text-dim">{t('server.ciphers_desc')}</p>
        </div>

        <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
          <label
            onClick={() => handlePresetChange('modern')}
            className={`flex flex-col p-3 border rounded-lg cursor-pointer transition-colors ${
              cipherPreset === 'modern'
                ? 'border-2 border-emerald-600 bg-emerald-600/5'
                : 'subtle-border surface-header hover:border-blue-500/50'
            }`}
          >
            <div className="flex items-center justify-between">
              <span className="font-bold text-xs text-slate-900 dark:text-white">{t('server.preset_modern')}</span>
              <input type="radio" checked={cipherPreset === 'modern'} readOnly />
            </div>
            <span className="text-[11px] text-subtle mt-1">{t('server.preset_modern_desc')}</span>
          </label>

          <label
            onClick={() => handlePresetChange('balanced')}
            className={`flex flex-col p-3 border rounded-lg cursor-pointer transition-colors ${
              cipherPreset === 'balanced'
                ? 'border-2 border-blue-600 bg-blue-600/5'
                : 'subtle-border surface-header hover:border-blue-500/50'
            }`}
          >
            <div className="flex items-center justify-between">
              <span className="font-bold text-xs text-blue-400">{t('server.preset_balanced')}</span>
              <input type="radio" checked={cipherPreset === 'balanced'} readOnly />
            </div>
            <span className="text-[11px] text-subtle mt-1">{t('server.preset_balanced_desc')}</span>
          </label>

          <label
            onClick={() => handlePresetChange('custom')}
            className={`flex flex-col p-3 border rounded-lg cursor-pointer transition-colors ${
              cipherPreset === 'custom'
                ? 'border-2 border-amber-500 bg-amber-500/5'
                : 'subtle-border surface-header hover:border-blue-500/50'
            }`}
          >
            <div className="flex items-center justify-between">
              <span className="font-bold text-xs text-slate-900 dark:text-white">{t('server.preset_custom')}</span>
              <input type="radio" checked={cipherPreset === 'custom'} readOnly />
            </div>
            <span className="text-[11px] text-subtle mt-1">{t('server.preset_custom_desc')}</span>
          </label>
        </div>

        <div className="space-y-2 pt-2 border-t subtle-border font-mono text-xs">
          <div>
            <label className="text-subtle text-[11px] font-sans block mb-0.5">
              {t('server.key_exchange_ciphers')}
            </label>
            <input
              type="text"
              value={ikeProposals}
              onChange={(e) => setIkeProposals(e.target.value)}
              className="w-full surface-input border subtle-border rounded-lg px-2.5 py-1.5 text-xs text-subtle focus:outline-none focus:text-white"
            />
          </div>
          <div>
            <label className="text-subtle text-[11px] font-sans block mb-0.5">
              {t('server.data_encryption_ciphers')}
            </label>
            <input
              type="text"
              value={espProposals}
              onChange={(e) => setEspProposals(e.target.value)}
              className="w-full surface-input border subtle-border rounded-lg px-2.5 py-1.5 text-xs text-subtle focus:outline-none focus:text-white"
            />
          </div>
        </div>
      </div>
    </div>
  );
};
