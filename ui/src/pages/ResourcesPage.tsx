import React, { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { Download, Trash2, ShieldAlert, Laptop, Server, Network, Plus, Search, Filter, X } from 'lucide-react';
import { Modal } from '../components/Modal';
import { CopyButton } from '../components/CopyButton';
import { EmptyState } from '../components/EmptyState';

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
  const [searchQuery, setSearchQuery] = useState('');
  const [platformFilter, setPlatformFilter] = useState<'all' | 'macos' | 'windows' | 'linux' | 'ios' | 'android'>('all');
  const [ownershipFilter, setOwnershipFilter] = useState<'all' | 'company' | 'user'>('all');

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
      if (!res.ok) throw new Error('Failed to fetch devices');
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
      if (!res.ok) throw new Error('Failed to fetch resource groups');
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
      if (!res.ok) {
        const errData = await res.json().catch(() => ({}));
        throw new Error(errData.error || t('common.error_occurred') || 'Failed to create device');
      }
      return res.json();
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['devices'] });
      setIsAddDeviceOpen(false);
      createDeviceMutation.reset();
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
      if (!res.ok) {
        const errData = await res.json().catch(() => ({}));
        throw new Error(errData.error || t('common.error_occurred') || 'Failed to create subnet');
      }
      return res.json();
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['resource_groups'] });
      setIsAddGroupOpen(false);
      createGroupMutation.reset();
      setGroupName('');
      setGroupCIDR('');
      setGroupDesc('');
    },
  });

  const getDownloadTooltip = (plat: string) => {
    if (plat === 'windows') return t('resources.download_windows');
    if (plat === 'linux') return t('resources.download_linux');
    return t('resources.download_apple');
  };

  const filteredDevices = devices.filter((dev) => {
    if (platformFilter !== 'all' && dev.platform?.toLowerCase() !== platformFilter) return false;
    if (ownershipFilter !== 'all' && dev.device_type !== ownershipFilter) return false;
    if (!searchQuery.trim()) return true;
    const q = searchQuery.toLowerCase();
    return (
      dev.hostname.toLowerCase().includes(q) ||
      dev.virtual_ip.toLowerCase().includes(q) ||
      dev.platform.toLowerCase().includes(q) ||
      (dev.user?.email && dev.user.email.toLowerCase().includes(q)) ||
      (dev.resource_group?.name && dev.resource_group.name.toLowerCase().includes(q))
    );
  });

  const filteredGroups = resourceGroups.filter((g) => {
    if (!searchQuery.trim()) return true;
    const q = searchQuery.toLowerCase();
    return g.name.toLowerCase().includes(q) || g.cidr_block.toLowerCase().includes(q);
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

  const deleteGroupMutation = useMutation({
    mutationFn: async (id: string) => {
      const res = await fetch(`/api/resource-groups/${id}`, {
        method: 'DELETE',
        headers: { Authorization: `Bearer ${localStorage.getItem('token') || ''}` },
      });
      if (!res.ok) throw new Error('Failed to delete resource group');
      return res.json();
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['resource_groups'] });
    },
  });

  return (
    <div className="space-y-4">
      {/* Subnav & Action */}
      <div className="flex flex-wrap items-center justify-between gap-3">
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

        <div className="flex items-center gap-2 flex-1 max-w-sm ml-auto">
          <div className="relative w-full">
            <Search className="w-3.5 h-3.5 text-subtle absolute left-2.5 top-1/2 -translate-y-1/2" />
            <input
              type="text"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              placeholder={activeSubView === 'devices' ? "Search devices by host, IP, platform..." : "Search subnets by name, CIDR..."}
              className="w-full surface-input border subtle-border rounded-lg pl-8 pr-2.5 py-1.5 text-xs focus:outline-none focus:border-blue-500"
            />
          </div>
          <button
            onClick={() => (activeSubView === 'devices' ? setIsAddDeviceOpen(true) : setIsAddGroupOpen(true))}
            className="bg-blue-600 hover:bg-blue-500 text-white font-medium px-3 py-1.5 rounded-lg transition-colors flex items-center gap-1.5 shadow-xs text-xs shrink-0"
          >
            <Plus className="w-3.5 h-3.5" />
            {activeSubView === 'devices' ? t('resources.add_device') : t('resources.add_group')}
          </button>
        </div>
      </div>

      {/* VIEW A: DEVICES TABLE */}
      {activeSubView === 'devices' && (
        <div className="space-y-2">
          {/* Faceted Filter Chips */}
          <div className="flex flex-wrap items-center gap-1.5 text-[11px] px-1">
            <span className="text-subtle font-medium flex items-center gap-1 mr-1">
              <Filter className="w-3 h-3" />
              OS:
            </span>
            {(['all', 'macos', 'windows', 'linux', 'ios', 'android'] as const).map((p) => {
              const count = p === 'all' ? devices.length : devices.filter((d) => d.platform?.toLowerCase() === p).length;
              const isSelected = platformFilter === p;
              return (
                <button
                  key={p}
                  type="button"
                  data-filter-os={p}
                  onClick={() => setPlatformFilter(p)}
                  className={`px-2 py-0.5 rounded-md border subtle-border transition-colors uppercase font-mono text-[10px] ${
                    isSelected
                      ? 'bg-blue-600/15 border-blue-500/40 text-blue-500 font-semibold'
                      : 'text-subtle hover:text-current hover:bg-slate-500/10'
                  }`}
                >
                  {p} {count > 0 && <span className="opacity-75">({count})</span>}
                </button>
              );
            })}

            <span className="text-subtle font-medium ml-2 mr-1">Type:</span>
            {(['all', 'company', 'user'] as const).map((tType) => {
              const count = tType === 'all' ? devices.length : devices.filter((d) => d.device_type === tType).length;
              const isSelected = ownershipFilter === tType;
              return (
                <button
                  key={tType}
                  type="button"
                  onClick={() => setOwnershipFilter(tType)}
                  className={`px-2 py-0.5 rounded-md border subtle-border transition-colors capitalize text-[10px] ${
                    isSelected
                      ? 'bg-blue-600/15 border-blue-500/40 text-blue-500 font-semibold'
                      : 'text-subtle hover:text-current hover:bg-slate-500/10'
                  }`}
                >
                  {tType === 'all' ? 'All' : tType === 'company' ? 'Company' : 'User'} ({count})
                </button>
              );
            })}

            {(platformFilter !== 'all' || ownershipFilter !== 'all' || searchQuery) && (
              <button
                type="button"
                onClick={() => {
                  setPlatformFilter('all');
                  setOwnershipFilter('all');
                  setSearchQuery('');
                }}
                className="ml-auto text-rose-500 hover:text-rose-400 flex items-center gap-1 text-[11px] font-medium"
              >
                <X className="w-3 h-3" /> Reset filters
              </button>
            )}
          </div>

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
                  {filteredDevices.length === 0 ? (
                    <tr>
                      <td colSpan={6} className="py-8">
                        <EmptyState
                          icon={Laptop}
                          title={searchQuery || platformFilter !== 'all' || ownershipFilter !== 'all' ? 'No matching devices' : 'No devices enrolled yet'}
                          description={searchQuery || platformFilter !== 'all' || ownershipFilter !== 'all' ? `No devices match the active filter criteria.` : "Click '+ Add Device' to provision a VPN profile."}
                          actionLabel={searchQuery || platformFilter !== 'all' || ownershipFilter !== 'all' ? 'Clear Filters' : t('resources.add_device')}
                          actionIcon={searchQuery || platformFilter !== 'all' || ownershipFilter !== 'all' ? X : Plus}
                          onAction={searchQuery || platformFilter !== 'all' || ownershipFilter !== 'all' ? () => { setSearchQuery(''); setPlatformFilter('all'); setOwnershipFilter('all'); } : () => setIsAddDeviceOpen(true)}
                          className="border-none shadow-none bg-transparent"
                        />
                      </td>
                    </tr>
                  ) : (
                    filteredDevices.map((dev) => (
                      <tr key={dev.id} className="hover-row transition-colors">
                        <td className="py-2.5 px-3.5 font-medium text-slate-900 dark:text-white">
                          <div className="flex items-center gap-2">
                            {dev.device_type === 'company' ? (
                              <Server className="w-3.5 h-3.5 text-purple-400 shrink-0" />
                            ) : (
                              <Laptop className="w-3.5 h-3.5 text-blue-400 shrink-0" />
                            )}
                            <span className="truncate">{dev.hostname}</span>
                            <CopyButton value={dev.hostname} label={`hostname ${dev.hostname}`} />
                          </div>
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
                          <div className="flex items-center gap-1.5">
                            <span>{dev.virtual_ip}</span>
                            <CopyButton value={dev.virtual_ip} label={`Virtual IP ${dev.virtual_ip}`} />
                            <span className="text-[10px] text-dim font-sans">({dev.ip_allocation_type})</span>
                          </div>
                        </td>
                        <td className="py-2.5 px-3.5 text-subtle uppercase font-mono text-[11px]">{dev.platform}</td>
                        <td className="py-2.5 px-3.5 text-right space-x-1">
                          <button
                            onClick={() => handleDownload(dev, dev.platform === 'linux' ? 'linux' : dev.platform === 'windows' ? 'windows' : 'apple')}
                            className="p-1.5 hover:bg-blue-500/10 rounded-md text-blue-500 transition-colors inline-block"
                            title={getDownloadTooltip(dev.platform)}
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
                    ))
                  )}
                </tbody>
              </table>
            </div>
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
                  <th className="py-2.5 px-3.5">{t('resources.capacity')} &amp; Pool Usage</th>
                  <th className="py-2.5 px-3.5">{t('resources.enrolled_nodes')}</th>
                  <th className="py-2.5 px-3.5 text-right font-sans">{t('common.actions')}</th>
                </tr>
              </thead>
              <tbody className="divide-y subtle-border">
                {filteredGroups.length === 0 ? (
                  <tr>
                    <td colSpan={5} className="py-8">
                      <EmptyState
                        icon={Network}
                        title={searchQuery ? 'No matching subnets' : 'No resource subnets configured yet'}
                        description={searchQuery ? `No subnets match '${searchQuery}'.` : "Create an IP pool to allocate client virtual IP addresses."}
                        actionLabel={searchQuery ? 'Clear Search' : t('resources.add_group')}
                        actionIcon={searchQuery ? X : Plus}
                        onAction={searchQuery ? () => setSearchQuery('') : () => setIsAddGroupOpen(true)}
                        className="border-none shadow-none bg-transparent"
                      />
                    </td>
                  </tr>
                ) : (
                  filteredGroups.map((rg) => {
                    const totalCap = rg.capacity || 0;
                    const enrolled = rg.enrolled_node_count || 0;
                    const pct = totalCap > 0 ? Math.min(100, Math.round((enrolled / totalCap) * 100)) : 0;
                    const barColor = pct >= 90 ? 'bg-rose-500' : pct >= 70 ? 'bg-amber-500' : 'bg-emerald-500';

                    return (
                      <tr key={rg.id} className="hover-row">
                        <td className="py-2.5 px-3.5 font-sans font-medium text-slate-900 dark:text-white flex items-center gap-2">
                          <Network className="w-3.5 h-3.5 text-subtle" />
                          {rg.name}
                        </td>
                        <td className="py-2.5 px-3.5 text-subtle">
                          <div className="flex items-center gap-1.5">
                            <span>{rg.cidr_block}</span>
                            <CopyButton value={rg.cidr_block} label={`CIDR ${rg.cidr_block}`} />
                          </div>
                        </td>
                        <td className="py-2.5 px-3.5">
                          <div className="w-36 font-sans">
                            <div className="flex items-center justify-between text-[10px] mb-1">
                              <span className="font-mono font-medium text-slate-900 dark:text-white">{pct}% leased</span>
                              <span className="text-dim">{enrolled} / {totalCap.toLocaleString()}</span>
                            </div>
                            <div className="w-full bg-slate-500/20 rounded-full h-1.5 overflow-hidden">
                              <div
                                className={`h-full rounded-full transition-all duration-300 ${barColor}`}
                                style={{ width: `${Math.max(4, pct)}%` }}
                              />
                            </div>
                          </div>
                        </td>
                        <td className="py-2.5 px-3.5 text-emerald-500 font-sans">{rg.enrolled_node_count || 0} nodes</td>
                        <td className="py-2.5 px-3.5 text-right font-sans">
                          <button
                            onClick={() => {
                              if (confirm(`Delete subnet '${rg.name}'?`)) {
                                deleteGroupMutation.mutate(rg.id);
                              }
                            }}
                            className="p-1.5 hover:bg-rose-500/10 rounded-md text-subtle hover:text-rose-500 transition-colors"
                            title="Delete Subnet"
                            aria-label="Delete Subnet"
                          >
                            <Trash2 className="w-3.5 h-3.5" />
                          </button>
                        </td>
                      </tr>
                    );
                  })
                )}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* MODAL: ADD DEVICE */}
      <Modal
        isOpen={isAddDeviceOpen}
        onClose={() => {
          setIsAddDeviceOpen(false);
          createDeviceMutation.reset();
        }}
        title={t('resources.provision_title')}
        error={(createDeviceMutation.error as Error)?.message}
      >
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
                  <div className="font-medium text-slate-900 dark:text-white">{t('resources.company_node')}</div>
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
                  <div className="font-medium text-slate-900 dark:text-white">{t('resources.user_device')}</div>
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
            onClick={() => {
              setIsAddDeviceOpen(false);
              createDeviceMutation.reset();
            }}
            className="px-3 py-1.5 border subtle-border rounded-lg hover:bg-slate-500/10 text-xs"
          >
            {t('common.cancel')}
          </button>
        </div>
      </Modal>

      {/* MODAL: ADD SUBNET */}
      <Modal
        isOpen={isAddGroupOpen}
        onClose={() => {
          setIsAddGroupOpen(false);
          createGroupMutation.reset();
        }}
        title={t('resources.add_group')}
        error={(createGroupMutation.error as Error)?.message}
      >
        <div className="space-y-3 text-xs">
          <div>
            <label className="text-subtle block mb-1">{t('common.name')}</label>
            <input
              type="text"
              value={groupName}
              onChange={(e) => setGroupName(e.target.value)}
              placeholder="e.g. Staging VPC"
              className="w-full surface-input border subtle-border rounded-lg px-2.5 py-1.5 text-xs focus:outline-none focus:border-blue-500"
            />
          </div>

          <div>
            <label className="text-subtle block mb-1">{t('resources.cidr_range')}</label>
            <input
              type="text"
              value={groupCIDR}
              onChange={(e) => setGroupCIDR(e.target.value)}
              placeholder="e.g. 10.50.0.0/16"
              className="w-full surface-input border subtle-border rounded-lg px-2.5 py-1.5 font-mono text-xs focus:outline-none focus:border-blue-500"
            />
          </div>

          <div>
            <label className="text-subtle block mb-1">{t('common.description')}</label>
            <input
              type="text"
              value={groupDesc}
              onChange={(e) => setGroupDesc(e.target.value)}
              placeholder="e.g. Staging infrastructure subnet"
              className="w-full surface-input border subtle-border rounded-lg px-2.5 py-1.5 text-xs focus:outline-none focus:border-blue-500"
            />
          </div>
        </div>

        <div className="flex gap-2 pt-2 border-t subtle-border">
          <button
            onClick={() => createGroupMutation.mutate()}
            disabled={!groupName || !groupCIDR || createGroupMutation.isPending}
            className="flex-1 bg-blue-600 hover:bg-blue-500 disabled:opacity-50 text-white font-medium py-1.5 rounded-lg shadow-xs transition-colors text-xs"
          >
            {createGroupMutation.isPending ? 'Saving...' : t('common.save')}
          </button>
          <button
            onClick={() => {
              setIsAddGroupOpen(false);
              createGroupMutation.reset();
            }}
            className="px-3 py-1.5 border subtle-border rounded-lg hover:bg-slate-500/10 text-xs"
          >
            {t('common.cancel')}
          </button>
        </div>
      </Modal>
    </div>
  );
};
