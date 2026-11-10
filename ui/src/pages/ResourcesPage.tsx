import React, { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { Download, Trash2, ShieldAlert, Laptop, Server, Network, Plus, X } from 'lucide-react';

interface DeviceItem {
  id: string;
  hostname: string;
  device_type: 'company' | 'user';
  user?: { email: string; name: string };
  resource_group?: { name: string; cidr_block: string };
  resource_group_id: string;
  virtual_ip: string;
  platform: string;
  is_revoked: boolean;
  ip_allocation_type: string;
}

interface ResourceGroupItem {
  id: string;
  name: string;
  cidr_block: string;
  capacity: number;
  enrolled_node_count: number;
}

export const ResourcesPage: React.FC = () => {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const [activeSubView, setActiveSubView] = useState<'devices' | 'groups'>('devices');
  const [isAddDeviceOpen, setIsAddDeviceOpen] = useState(false);
  const [isAddGroupOpen, setIsAddGroupOpen] = useState(false);

  // Form states for Add Device
  const [deviceType, setDeviceType] = useState<'company' | 'user'>('company');
  const [hostname, setHostname] = useState('');
  const [selectedGroupID, setSelectedGroupID] = useState('');
  const [ipAllocType, setIpAllocType] = useState<'dynamic' | 'static'>('dynamic');
  const [staticIP, setStaticIP] = useState('');
  const [platform, setPlatform] = useState('macos');

  // Form states for Add Group
  const [groupName, setGroupName] = useState('');
  const [groupCIDR, setGroupCIDR] = useState('');
  const [groupDesc, setGroupDesc] = useState('');

  // 1. Query Devices
  const { data: devicesData } = useQuery<{ devices: DeviceItem[] }>({
    queryKey: ['devices'],
    queryFn: async () => {
      const res = await fetch('/api/devices', {
        headers: { Authorization: `Bearer ${localStorage.getItem('token') || ''}` },
      });
      if (!res.ok) {
        return {
          devices: [
            {
              id: 'dev_1',
              hostname: 'db-primary-prod',
              device_type: 'company',
              resource_group: { name: 'Production VPC', cidr_block: '10.100.0.0/16' },
              resource_group_id: 'rg_1',
              virtual_ip: '10.100.1.5',
              platform: 'linux',
              is_revoked: false,
              ip_allocation_type: 'static',
            },
            {
              id: 'dev_2',
              hostname: 'thienvu-macbook',
              device_type: 'user',
              user: { name: 'Thien Vu', email: 'thienvu@company.com' },
              resource_group: { name: 'Engineering Subnet', cidr_block: '10.0.1.0/24' },
              resource_group_id: 'rg_2',
              virtual_ip: '10.0.1.15',
              platform: 'macos',
              is_revoked: false,
              ip_allocation_type: 'dynamic',
            },
            {
              id: 'dev_3',
              hostname: 'nam-thinkpad',
              device_type: 'user',
              user: { name: 'Nam Nguyen', email: 'nam@company.com' },
              resource_group: { name: 'Engineering Subnet', cidr_block: '10.0.1.0/24' },
              resource_group_id: 'rg_2',
              virtual_ip: '10.0.1.16',
              platform: 'windows',
              is_revoked: false,
              ip_allocation_type: 'dynamic',
            },
          ],
        };
      }
      return res.json();
    },
  });

  // 2. Query Resource Groups
  const { data: groupsData } = useQuery<{ resource_groups: ResourceGroupItem[] }>({
    queryKey: ['resource_groups'],
    queryFn: async () => {
      const res = await fetch('/api/resource-groups', {
        headers: { Authorization: `Bearer ${localStorage.getItem('token') || ''}` },
      });
      if (!res.ok) {
        return {
          resource_groups: [
            {
              id: 'rg_1',
              name: 'Production VPC',
              cidr_block: '10.100.0.0/16',
              capacity: 65534,
              enrolled_node_count: 1,
            },
            {
              id: 'rg_2',
              name: 'Engineering Subnet',
              cidr_block: '10.0.1.0/24',
              capacity: 254,
              enrolled_node_count: 2,
            },
          ],
        };
      }
      return res.json();
    },
  });

  const devices = devicesData?.devices || [];
  const resourceGroups = groupsData?.resource_groups || [];

  // Create Device Mutation
  const createDeviceMutation = useMutation({
    mutationFn: async () => {
      const payload = {
        hostname,
        device_type: deviceType,
        resource_group_id: selectedGroupID || resourceGroups[0]?.id,
        ip_allocation_type: ipAllocType,
        virtual_ip: ipAllocType === 'static' ? staticIP : '',
        platform,
      };
      const res = await fetch('/api/devices', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${localStorage.getItem('token') || ''}`,
        },
        body: JSON.stringify(payload),
      });
      if (!res.ok) throw new Error('Failed to create device');
      return res.json();
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['devices'] });
      setIsAddDeviceOpen(false);
      setHostname('');
      setStaticIP('');
    },
  });

  // Create Subnet Mutation
  const createGroupMutation = useMutation({
    mutationFn: async () => {
      const res = await fetch('/api/resource-groups', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${localStorage.getItem('token') || ''}`,
        },
        body: JSON.stringify({ name: groupName, cidr_block: groupCIDR, description: groupDesc }),
      });
      if (!res.ok) throw new Error('Failed to create subnet');
      return res.json();
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['resource_groups'] });
      setIsAddGroupOpen(false);
      setGroupName('');
      setGroupCIDR('');
      setGroupDesc('');
    },
  });

  const handleDownload = (dev: DeviceItem, os: string) => {
    window.open(`/api/devices/${dev.id}/config/${os}?token=${localStorage.getItem('token') || ''}`, '_blank');
  };

  const handleRevoke = async (dev: DeviceItem) => {
    if (confirm(`Revoke security certificate for ${dev.hostname}? The tunnel will immediately terminate.`)) {
      await fetch(`/api/devices/${dev.id}/revoke`, {
        method: 'POST',
        headers: { Authorization: `Bearer ${localStorage.getItem('token') || ''}` },
      });
      queryClient.invalidateQueries({ queryKey: ['devices'] });
    }
  };

  return (
    <div className="space-y-4">
      {/* Subnav & Action */}
      <div className="flex items-center justify-between">
        <div className="flex border subtle-border rounded-lg p-0.5 surface-header text-xs">
          <button
            onClick={() => setActiveSubView('devices')}
            className={`px-3 py-1.5 rounded-md font-medium transition-colors ${
              activeSubView === 'devices' ? 'bg-blue-600 text-white shadow-xs' : 'text-subtle hover:text-current'
            }`}
          >
            {t('resources.devices')} ({devices.length})
          </button>
          <button
            onClick={() => setActiveSubView('groups')}
            className={`px-3 py-1.5 rounded-md font-medium transition-colors ${
              activeSubView === 'groups' ? 'bg-blue-600 text-white shadow-xs' : 'text-subtle hover:text-current'
            }`}
          >
            {t('resources.resource_groups')} ({resourceGroups.length})
          </button>
        </div>

        <button
          onClick={() => (activeSubView === 'devices' ? setIsAddDeviceOpen(true) : setIsAddGroupOpen(true))}
          className="bg-blue-600 hover:bg-blue-500 text-white font-medium px-3 py-1.5 rounded-lg transition-colors flex items-center gap-1.5 shadow-xs text-xs"
        >
          <Plus className="w-3.5 h-3.5" />
          {activeSubView === 'devices' ? t('resources.add_device') : t('resources.add_group')}
        </button>
      </div>

      {/* VIEW A: DEVICES TABLE */}
      {activeSubView === 'devices' && (
        <div className="border subtle-border rounded-xl overflow-hidden surface-card shadow-xs">
          <div className="overflow-x-auto">
            <table className="w-full text-left text-xs min-w-[700px]">
              <thead className="border-b subtle-border surface-header text-subtle font-semibold text-[11px]">
                <tr>
                  <th className="py-2.5 px-3.5">{t('resources.device_hostname')}</th>
                  <th className="py-2.5 px-3.5">{t('resources.ownership')}</th>
                  <th className="py-2.5 px-3.5">{t('resources.resource_group')}</th>
                  <th className="py-2.5 px-3.5">{t('resources.virtual_ip')}</th>
                  <th className="py-2.5 px-3.5">{t('resources.platform')}</th>
                  <th className="py-2.5 px-3.5 text-right">{t('common.actions')}</th>
                </tr>
              </thead>
              <tbody className="divide-y subtle-border">
                {devices.map((dev) => (
                  <tr key={dev.id} className="hover-row transition-colors">
                    <td className="py-2.5 px-3.5 font-medium text-white flex items-center gap-2">
                      {dev.device_type === 'company' ? (
                        <Server className="w-3.5 h-3.5 text-purple-400" />
                      ) : (
                        <Laptop className="w-3.5 h-3.5 text-blue-400" />
                      )}
                      {dev.hostname}
                    </td>
                    <td className="py-2.5 px-3.5">
                      {dev.device_type === 'company' ? (
                        <span className="border border-purple-500/30 text-purple-400 bg-purple-500/10 px-2 py-0.5 rounded-md text-[10px] font-mono">
                          {t('resources.company_resource')}
                        </span>
                      ) : (
                        <span className="text-subtle font-mono text-[11px]">
                          {dev.user?.email || 'User Device'}
                        </span>
                      )}
                    </td>
                    <td className="py-2.5 px-3.5 text-subtle font-mono">
                      {dev.resource_group?.name || 'Default Subnet'}
                    </td>
                    <td className="py-2.5 px-3.5 font-mono text-blue-400">
                      {dev.virtual_ip} <span className="text-[10px] text-dim font-sans">({dev.ip_allocation_type})</span>
                    </td>
                    <td className="py-2.5 px-3.5 text-subtle uppercase font-mono text-[11px]">{dev.platform}</td>
                    <td className="py-2.5 px-3.5 text-right space-x-1">
                      <button
                        onClick={() => handleDownload(dev, dev.platform === 'linux' ? 'linux' : dev.platform === 'windows' ? 'windows' : 'apple')}
                        className="p-1.5 hover:bg-blue-500/10 rounded-md text-blue-500 transition-colors inline-block"
                        title={t('resources.download_apple')}
                      >
                        <Download className="w-3.5 h-3.5" />
                      </button>
                      <button
                        onClick={() => handleRevoke(dev)}
                        className="p-1.5 hover:bg-rose-500/10 rounded-md text-rose-500 transition-colors inline-block"
                        title={t('dashboard.self_revoke')}
                      >
                        <ShieldAlert className="w-3.5 h-3.5" />
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* VIEW B: RESOURCE GROUPS TABLE */}
      {activeSubView === 'groups' && (
        <div className="border subtle-border rounded-xl overflow-hidden surface-card shadow-xs">
          <div className="overflow-x-auto">
            <table className="w-full text-left font-mono text-xs min-w-[600px]">
              <thead className="border-b subtle-border surface-header text-subtle font-sans font-semibold text-[11px]">
                <tr>
                  <th className="py-2.5 px-3.5">{t('resources.resource_group')}</th>
                  <th className="py-2.5 px-3.5">{t('resources.cidr_range')}</th>
                  <th className="py-2.5 px-3.5">{t('resources.capacity')}</th>
                  <th className="py-2.5 px-3.5">{t('resources.enrolled_nodes')}</th>
                  <th className="py-2.5 px-3.5 text-right font-sans">{t('common.actions')}</th>
                </tr>
              </thead>
              <tbody className="divide-y subtle-border">
                {resourceGroups.map((rg) => (
                  <tr key={rg.id} className="hover-row">
                    <td className="py-2.5 px-3.5 font-sans font-medium text-white flex items-center gap-2">
                      <Network className="w-3.5 h-3.5 text-subtle" />
                      {rg.name}
                    </td>
                    <td className="py-2.5 px-3.5 text-subtle">{rg.cidr_block}</td>
                    <td className="py-2.5 px-3.5 text-dim">{rg.capacity.toLocaleString()} available</td>
                    <td className="py-2.5 px-3.5 text-emerald-500 font-sans">{rg.enrolled_node_count} nodes</td>
                    <td className="py-2.5 px-3.5 text-right font-sans">
                      <button
                        onClick={() => alert('Editing subnet CIDR')}
                        className="p-1.5 hover:bg-slate-500/10 rounded-md text-subtle hover:text-current"
                      >
                        <Trash2 className="w-3.5 h-3.5" />
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* MODAL: ADD DEVICE */}
      {isAddDeviceOpen && (
        <div className="fixed inset-0 bg-black/60 z-50 flex items-center justify-center p-4">
          <div className="surface-card border subtle-border rounded-xl max-w-sm w-full p-5 space-y-4 shadow-2xl">
            <div className="flex items-center justify-between border-b subtle-border pb-2.5">
              <div>
                <h3 className="font-bold text-sm text-white">{t('resources.provision_title')}</h3>
                <div className="text-[11px] text-dim">{t('resources.provision_desc')}</div>
              </div>
              <button onClick={() => setIsAddDeviceOpen(false)} className="text-subtle hover:text-current">
                <X className="w-4 h-4" />
              </button>
            </div>

            <div className="space-y-3 text-xs">
              <div>
                <label className="text-subtle block mb-1">{t('resources.device_ownership')}</label>
                <div className="grid grid-cols-2 gap-2">
                  <label
                    onClick={() => setDeviceType('company')}
                    className={`flex items-center gap-2 p-2 border rounded-lg cursor-pointer ${
                      deviceType === 'company' ? 'border-blue-500 bg-blue-600/10' : 'subtle-border surface-header'
                    }`}
                  >
                    <div>
                      <div className="font-medium text-white">{t('resources.company_node')}</div>
                      <div className="text-[10px] text-dim">{t('resources.company_node_desc')}</div>
                    </div>
                  </label>
                  <label
                    onClick={() => setDeviceType('user')}
                    className={`flex items-center gap-2 p-2 border rounded-lg cursor-pointer ${
                      deviceType === 'user' ? 'border-blue-500 bg-blue-600/10' : 'subtle-border surface-header'
                    }`}
                  >
                    <div>
                      <div className="font-medium text-white">{t('resources.user_device')}</div>
                      <div className="text-[10px] text-dim">{t('resources.user_device_desc')}</div>
                    </div>
                  </label>
                </div>
              </div>

              <div>
                <label className="text-subtle block mb-1">{t('resources.device_hostname')}</label>
                <input
                  type="text"
                  value={hostname}
                  onChange={(e) => setHostname(e.target.value)}
                  placeholder="e.g. k8s-worker-01 or thienvu-macbook"
                  className="w-full surface-input border subtle-border rounded-lg px-2.5 py-1.5 font-mono text-xs focus:outline-none focus:border-blue-500"
                />
              </div>

              <div>
                <label className="text-subtle block mb-1">{t('resources.subnet_cidr')}</label>
                <select
                  value={selectedGroupID}
                  onChange={(e) => setSelectedGroupID(e.target.value)}
                  className="w-full surface-input border subtle-border rounded-lg px-2.5 py-1.5 text-xs focus:outline-none"
                >
                  {resourceGroups.map((g) => (
                    <option key={g.id} value={g.id}>
                      {g.name} ({g.cidr_block})
                    </option>
                  ))}
                </select>
              </div>

              <div>
                <label className="text-subtle block mb-1">{t('resources.ip_allocation')}</label>
                <div className="flex gap-4 text-xs mb-1.5">
                  <label className="flex items-center gap-1.5 cursor-pointer">
                    <input
                      type="radio"
                      checked={ipAllocType === 'dynamic'}
                      onChange={() => setIpAllocType('dynamic')}
                    />
                    {t('resources.dynamic_pool')}
                  </label>
                  <label className="flex items-center gap-1.5 cursor-pointer">
                    <input
                      type="radio"
                      checked={ipAllocType === 'static'}
                      onChange={() => setIpAllocType('static')}
                    />
                    {t('resources.static_fixed_ip')}
                  </label>
                </div>
                {ipAllocType === 'static' && (
                  <input
                    type="text"
                    value={staticIP}
                    onChange={(e) => setStaticIP(e.target.value)}
                    placeholder="e.g. 10.100.1.20"
                    className="w-full surface-input border subtle-border rounded-lg px-2.5 py-1.5 font-mono text-xs focus:outline-none"
                  />
                )}
              </div>

              <div>
                <label className="text-subtle block mb-1">{t('resources.platform')}</label>
                <select
                  value={platform}
                  onChange={(e) => setPlatform(e.target.value)}
                  className="w-full surface-input border subtle-border rounded-lg px-2.5 py-1.5 text-xs focus:outline-none"
                >
                  <option value="macos">macOS / iOS (Apple Profile)</option>
                  <option value="windows">Windows 10/11 (PowerShell)</option>
                  <option value="linux">Linux (swanctl.conf)</option>
                </select>
              </div>
            </div>

            <div className="flex gap-2 pt-2 border-t subtle-border">
              <button
                onClick={() => createDeviceMutation.mutate()}
                disabled={!hostname || createDeviceMutation.isPending}
                className="flex-1 bg-blue-600 hover:bg-blue-500 disabled:opacity-50 text-white font-medium py-1.5 rounded-lg shadow-xs transition-colors text-xs"
              >
                {createDeviceMutation.isPending ? 'Provisioning...' : t('resources.create_and_download')}
              </button>
              <button
                onClick={() => setIsAddDeviceOpen(false)}
                className="px-3 py-1.5 border subtle-border rounded-lg hover:bg-slate-500/10 text-xs"
              >
                {t('common.cancel')}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* MODAL: ADD SUBNET */}
      {isAddGroupOpen && (
        <div className="fixed inset-0 bg-black/60 z-50 flex items-center justify-center p-4">
          <div className="surface-card border subtle-border rounded-xl max-w-sm w-full p-5 space-y-4 shadow-2xl">
            <div className="flex items-center justify-between border-b subtle-border pb-2.5">
              <h3 className="font-bold text-sm text-white">{t('resources.add_group')}</h3>
              <button onClick={() => setIsAddGroupOpen(false)} className="text-subtle hover:text-current">
                <X className="w-4 h-4" />
              </button>
            </div>

            <div className="space-y-3 text-xs">
              <div>
                <label className="text-subtle block mb-1">{t('common.name')}</label>
                <input
                  type="text"
                  value={groupName}
                  onChange={(e) => setGroupName(e.target.value)}
                  placeholder="e.g. Staging VPC"
                  className="w-full surface-input border subtle-border rounded-lg px-2.5 py-1.5 text-xs focus:outline-none"
                />
              </div>

              <div>
                <label className="text-subtle block mb-1">{t('resources.cidr_range')}</label>
                <input
                  type="text"
                  value={groupCIDR}
                  onChange={(e) => setGroupCIDR(e.target.value)}
                  placeholder="e.g. 10.50.0.0/16"
                  className="w-full surface-input border subtle-border rounded-lg px-2.5 py-1.5 font-mono text-xs focus:outline-none"
                />
              </div>

              <div>
                <label className="text-subtle block mb-1">{t('common.description')}</label>
                <input
                  type="text"
                  value={groupDesc}
                  onChange={(e) => setGroupDesc(e.target.value)}
                  placeholder="e.g. Staging infrastructure subnet"
                  className="w-full surface-input border subtle-border rounded-lg px-2.5 py-1.5 text-xs focus:outline-none"
                />
              </div>
            </div>

            <div className="flex gap-2 pt-2 border-t subtle-border">
              <button
                onClick={() => createGroupMutation.mutate()}
                disabled={!groupName || !groupCIDR || createGroupMutation.isPending}
                className="flex-1 bg-blue-600 hover:bg-blue-500 disabled:opacity-50 text-white font-medium py-1.5 rounded-lg shadow-xs transition-colors text-xs"
              >
                {t('common.save')}
              </button>
              <button
                onClick={() => setIsAddGroupOpen(false)}
                className="px-3 py-1.5 border subtle-border rounded-lg hover:bg-slate-500/10 text-xs"
              >
                {t('common.cancel')}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};
