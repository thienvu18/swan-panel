export type ProvisionedDeviceType = 'company' | 'user';
export type DeviceIPAllocationType = 'dynamic' | 'static';

export interface DeviceProvisioningInput {
  hostname: string;
  deviceType: ProvisionedDeviceType;
  resourceGroupID: string;
  ipAllocationType: DeviceIPAllocationType;
  staticIP: string;
  platform: string;
  isAdmin: boolean;
  ownerUserID: string;
}

export function requiresDeviceOwnerSelection(isAdmin: boolean, deviceType: ProvisionedDeviceType): boolean {
  return isAdmin && deviceType === 'user';
}

export function buildDeviceProvisioningPayload(input: DeviceProvisioningInput) {
  if (requiresDeviceOwnerSelection(input.isAdmin, input.deviceType) && !input.ownerUserID.trim()) {
    throw new Error('An owner must be selected for a user device');
  }

  return {
    hostname: input.hostname.trim(),
    device_type: input.deviceType,
    resource_group_id: input.resourceGroupID,
    ip_allocation_type: input.ipAllocationType,
    virtual_ip: input.ipAllocationType === 'static' ? input.staticIP.trim() : '',
    platform: input.platform,
    ...(requiresDeviceOwnerSelection(input.isAdmin, input.deviceType)
      ? { user_id: input.ownerUserID }
      : {}),
  };
}
