export interface CurrentUser {
  id: string;
  email: string;
  name: string;
  is_admin: boolean;
  status: string;
}

export interface Device {
  id: string;
  hostname: string;
  device_type: 'company' | 'user';
  user_id?: string;
  resource_group_id: string;
  ip_allocation_type: 'dynamic' | 'static';
  virtual_ip: string;
  platform: 'macos' | 'windows' | 'linux' | 'ios' | 'android';
  cert_fingerprint: string;
  is_revoked: boolean;
  created_at: string;
  updated_at: string;
}

export interface ResourceGroup {
  id: string;
  name: string;
  cidr_block: string;
  description?: string;
  created_at: string;
}

export interface AccessRule {
  id: string;
  subject_type: 'user_group' | 'user' | 'device';
  subject_id: string;
  action: 'ALLOW' | 'DENY';
  target_type: 'resource_group' | 'cidr_ip';
  target_value: string;
  rule_nature: string;
  created_at: string;
}

export interface SessionInfo {
  ike_name: string;
  peer_identity: string;
  virtual_ip: string;
  remote_wan: string;
  uptime: string;
  cipher_suite: string;
  established_seconds: number;
}
