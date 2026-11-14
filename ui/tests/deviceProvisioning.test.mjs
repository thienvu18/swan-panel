import test from 'node:test';
import assert from 'node:assert/strict';
import { buildDeviceProvisioningPayload, requiresDeviceOwnerSelection } from '../src/lib/deviceProvisioning.ts';

const base = {
  hostname: 'employee-laptop',
  deviceType: 'user',
  resourceGroupID: 'rg_1',
  ipAllocationType: 'dynamic',
  staticIP: '',
  platform: 'macos',
  isAdmin: true,
  ownerUserID: 'usr_123',
};

test('administrator provisioning a user device requires explicit owner selection', () => {
  assert.equal(requiresDeviceOwnerSelection(true, 'user'), true);
  assert.throws(
    () => buildDeviceProvisioningPayload({ ...base, ownerUserID: '' }),
    /owner must be selected/,
  );
  assert.equal(buildDeviceProvisioningPayload(base).user_id, 'usr_123');
});

test('company device never inherits an owner from a previous user selection', () => {
  assert.equal(requiresDeviceOwnerSelection(true, 'company'), false);
  const payload = buildDeviceProvisioningPayload({ ...base, deviceType: 'company' });
  assert.equal(payload.device_type, 'company');
  assert.equal(Object.hasOwn(payload, 'user_id'), false);
});

test('standard users provision their own devices without choosing another owner', () => {
  assert.equal(requiresDeviceOwnerSelection(false, 'user'), false);
  const payload = buildDeviceProvisioningPayload({ ...base, isAdmin: false, ownerUserID: '' });
  assert.equal(payload.device_type, 'user');
  assert.equal(Object.hasOwn(payload, 'user_id'), false);
});

test('device payload preserves network and platform fields', () => {
  const payload = buildDeviceProvisioningPayload({
    ...base,
    hostname: ' laptop.example ',
    ipAllocationType: 'static',
    staticIP: ' 10.20.0.15 ',
    platform: 'windows',
  });
  assert.deepEqual(payload, {
    hostname: 'laptop.example',
    device_type: 'user',
    resource_group_id: 'rg_1',
    ip_allocation_type: 'static',
    virtual_ip: '10.20.0.15',
    platform: 'windows',
    user_id: 'usr_123',
  });
});
