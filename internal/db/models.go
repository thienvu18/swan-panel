package db

import (
	"time"
)

// OIDCProvider represents an OpenID Connect identity provider (Google, Microsoft, Keycloak, etc.)
type OIDCProvider struct {
	ID           string    `gorm:"primaryKey;size:64" json:"id"`
	Name         string    `gorm:"size:128;not null" json:"name"`
	ProviderType string    `gorm:"size:32;not null" json:"provider_type"` // 'google', 'microsoft', 'keycloak', 'custom'
	IssuerURL    string    `gorm:"size:512;not null" json:"issuer_url"`
	ClientID     string    `gorm:"size:256;not null" json:"client_id"`
	ClientSecret string    `gorm:"size:512;not null" json:"-"` // Never serialized to JSON
	IsActive     bool      `gorm:"default:true" json:"is_active"`
	CreatedAt    time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt    time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// User represents an internal user (OIDC SSO or local admin/member)
type User struct {
	ID            string        `gorm:"primaryKey;size:64" json:"id"`
	Email         string        `gorm:"size:256;uniqueIndex;not null" json:"email"`
	Name          string        `gorm:"size:128;not null" json:"name"`
	AuthType      string        `gorm:"size:16;default:'oidc'" json:"auth_type"` // 'oidc' or 'local'
	PasswordHash  string        `gorm:"size:256" json:"-"`                       // Stored only for local users (bcrypt)
	IDPProviderID *string       `gorm:"size:64" json:"idp_provider_id,omitempty"`
	IDPProvider   *OIDCProvider `gorm:"foreignKey:IDPProviderID;constraint:OnDelete:SET NULL" json:"idp_provider,omitempty"`
	IsAdmin       bool          `gorm:"default:false" json:"is_admin"`
	Status        string        `gorm:"size:32;default:'PENDING'" json:"status"` // 'PENDING', 'ACTIVE', 'SUSPENDED'
	UserGroups    []UserGroup   `gorm:"many2many:user_group_members;" json:"user_groups,omitempty"`
	Devices       []Device      `gorm:"foreignKey:UserID" json:"devices,omitempty"`
	CreatedAt     time.Time     `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt     time.Time     `gorm:"autoUpdateTime" json:"updated_at"`
}

// UserGroup represents an organizational team or role (e.g., DevOps, Backend, Sales)
type UserGroup struct {
	ID          string    `gorm:"primaryKey;size:64" json:"id"`
	Name        string    `gorm:"size:128;uniqueIndex;not null" json:"name"`
	Description string    `gorm:"size:512" json:"description"`
	Users       []User    `gorm:"many2many:user_group_members;" json:"users,omitempty"`
	CreatedAt   time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// ResourceGroup represents an internal network subnet or VPC (Subnet CIDR)
type ResourceGroup struct {
	ID          string    `gorm:"primaryKey;size:64" json:"id"`
	Name        string    `gorm:"size:128;uniqueIndex;not null" json:"name"`
	CIDRBlock   string    `gorm:"size:64;not null" json:"cidr_block"` // e.g. 10.100.0.0/16
	Description string    `gorm:"size:512" json:"description"`
	Devices     []Device  `gorm:"foreignKey:ResourceGroupID" json:"devices,omitempty"`
	CreatedAt   time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// Device represents a network peer (Company Node or User Device)
type Device struct {
	ID               string        `gorm:"primaryKey;size:64" json:"id"`
	Hostname         string        `gorm:"size:128;uniqueIndex;not null" json:"hostname"`
	DeviceType       string        `gorm:"size:32;not null" json:"device_type"` // 'company' (Node) or 'user' (Device)
	UserID           *string       `gorm:"size:64" json:"user_id,omitempty"`
	User             *User         `gorm:"foreignKey:UserID;constraint:OnDelete:SET NULL" json:"user,omitempty"`
	ResourceGroupID  string        `gorm:"size:64;not null" json:"resource_group_id"`
	ResourceGroup    ResourceGroup `gorm:"foreignKey:ResourceGroupID" json:"resource_group"`
	IPAllocationType string        `gorm:"size:16;not null" json:"ip_allocation_type"` // 'dynamic' or 'static'
	VirtualIP        string        `gorm:"size:64;not null" json:"virtual_ip"`
	Platform         string        `gorm:"size:32;not null" json:"platform"` // 'macos', 'ios', 'windows', 'linux', 'android'
	CertFingerprint  string        `gorm:"size:128" json:"cert_fingerprint,omitempty"`
	IsRevoked        bool          `gorm:"default:false" json:"is_revoked"`
	LastSeenAt       *time.Time    `json:"last_seen_at,omitempty"`
	CreatedAt        time.Time     `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt        time.Time     `gorm:"autoUpdateTime" json:"updated_at"`
}

// AccessRule defines network access control policies (ALLOW or DENY exclusions)
type AccessRule struct {
	ID          string    `gorm:"primaryKey;size:64" json:"id"`
	SubjectType string    `gorm:"size:32;not null" json:"subject_type"` // 'user_group', 'user', 'device'
	SubjectID   string    `gorm:"size:64;not null" json:"subject_id"`
	Action      string    `gorm:"size:16;not null" json:"action"`      // 'ALLOW' or 'DENY'
	TargetType  string    `gorm:"size:32;not null" json:"target_type"` // 'resource_group', 'cidr_ip'
	TargetValue string    `gorm:"size:128;not null" json:"target_value"`
	RuleNature  string    `gorm:"size:32;not null" json:"rule_nature"` // 'group_policy', 'direct_grant', 'user_exclusion'
	CreatedAt   time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// DNSSettings configures MagicDNS domain resolution and upstream forwarders
type DNSSettings struct {
	ID                uint      `gorm:"primaryKey;default:1" json:"id"`
	MagicDNSSuffix    string    `gorm:"size:64;default:'.strongnet'" json:"magic_dns_suffix"`
	UpstreamResolvers string    `gorm:"size:256;default:'1.1.1.1, 8.8.8.8'" json:"upstream_resolvers"`
	IsMagicDNSEnabled bool      `gorm:"default:true" json:"is_magic_dns_enabled"`
	UpdatedAt         time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// DNSOverride maps an internal custom hostname directly to a private IP
type DNSOverride struct {
	ID            string    `gorm:"primaryKey;size:64" json:"id"`
	Hostname      string    `gorm:"size:256;uniqueIndex;not null" json:"hostname"` // e.g. gitlab.internal
	RecordType    string    `gorm:"size:16;default:'A'" json:"record_type"`
	TargetAddress string    `gorm:"size:64;not null" json:"target_address"`
	CreatedAt     time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt     time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// StrongswanSettings controls the strongSwan data plane parameters
type StrongswanSettings struct {
	ID                uint      `gorm:"primaryKey;default:1" json:"id"`
	ServerEndpoint    string    `gorm:"size:256;default:'vpn.company.com'" json:"server_endpoint"`
	ServerIdentity    string    `gorm:"size:256;default:'vpn.company.com'" json:"server_identity"`
	ReauthTime        string    `gorm:"size:32;default:'12h'" json:"reauth_time"`
	RekeyTimeIKE      string    `gorm:"size:32;default:'4h'" json:"rekey_time_ike"`
	RekeyTimeChild    string    `gorm:"size:32;default:'1h'" json:"rekey_time_child"`
	InactivityTimeout string    `gorm:"size:32;default:'30m'" json:"inactivity_timeout"`
	MobikeEnabled     bool      `gorm:"default:true" json:"mobike_enabled"`
	ForceEncapNATT    bool      `gorm:"default:false" json:"force_encap_natt"`
	Fragmentation     string    `gorm:"size:16;default:'yes'" json:"fragmentation"`
	DPDDelay          string    `gorm:"size:32;default:'30s'" json:"dpd_delay"`
	DPDTimeout        string    `gorm:"size:32;default:'120s'" json:"dpd_timeout"`
	DPDAction         string    `gorm:"size:32;default:'clear'" json:"dpd_action"`
	CipherPreset      string    `gorm:"size:32;default:'balanced'" json:"cipher_preset"`
	IKEProposals      string    `gorm:"size:512;default:'aes256gcm16-prfsha384-ecp384,aes256-sha256-modp2048'" json:"ike_proposals"`
	ESPProposals      string    `gorm:"size:512;default:'aes256gcm16,aes128gcm16,chacha20poly1305'" json:"esp_proposals"`
	MSSClamping       int       `gorm:"default:1360" json:"mss_clamping"`
	UpdatedAt         time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}
