// Package model defines GORM model structs for all database tables,
// providing a single source of truth for the schema that works
// transparently with both SQLite and PostgreSQL.
package model

import "database/sql"

// ─── Core Business Tables ────────────────────────────────────────────

// User maps to the "user" table. PostgreSQL treats "user" as a reserved
// word, so TableName() is required for correct quoting.
type User struct {
	ID            int64         `gorm:"primaryKey;autoIncrement"`
	User          string        `gorm:"column:user;type:varchar(100);not null"`
	Pwd           string        `gorm:"type:varchar(100);not null"`
	RoleID        int           `gorm:"column:role_id;not null"`
	ExpTime       int64         `gorm:"column:exp_time;not null"`
	Flow          int64         `gorm:"not null"`
	InFlow        int64         `gorm:"column:in_flow;not null;default:0"`
	OutFlow       int64         `gorm:"column:out_flow;not null;default:0"`
	FlowResetTime int64         `gorm:"column:flow_reset_time;not null"`
	Num           int           `gorm:"not null"`
	CreatedTime   int64         `gorm:"column:created_time;not null"`
	UpdatedTime   sql.NullInt64 `gorm:"column:updated_time"`
	Status        int           `gorm:"not null"`
}

func (User) TableName() string { return "user" }

// Forward maps to the "forward" table.
type Forward struct {
	ID          int64         `gorm:"primaryKey;autoIncrement"`
	UserID      int64         `gorm:"column:user_id;not null"`
	UserName    string        `gorm:"column:user_name;type:varchar(100);not null"`
	Name        string        `gorm:"type:varchar(100);not null"`
	TunnelID    int64         `gorm:"column:tunnel_id;not null"`
	RemoteAddr  string        `gorm:"column:remote_addr;type:text;not null"`
	Strategy    string        `gorm:"type:varchar(100);not null;default:'fifo'"`
	InFlow      int64         `gorm:"not null;default:0"`
	OutFlow     int64         `gorm:"column:out_flow;not null;default:0"`
	CreatedTime int64         `gorm:"column:created_time;not null"`
	UpdatedTime int64         `gorm:"column:updated_time;not null"`
	Status      int           `gorm:"not null"`
	Inx         int           `gorm:"not null;default:0"`
	SpeedID     sql.NullInt64 `gorm:"column:speed_id"`
}

func (Forward) TableName() string { return "forward" }

type ForwardPort struct {
	ID        int64          `gorm:"primaryKey;autoIncrement"`
	ForwardID int64          `gorm:"column:forward_id;not null"`
	NodeID    int64          `gorm:"column:node_id;not null"`
	Port      int            `gorm:"not null"`
	InIP      sql.NullString `gorm:"column:in_ip;type:text"`
}

func (ForwardPort) TableName() string { return "forward_port" }

type Node struct {
	ID            int64          `gorm:"primaryKey;autoIncrement"`
	Name          string         `gorm:"type:varchar(100);not null"`
	Remark        sql.NullString `gorm:"column:remark;type:text"`
	ExpiryTime    sql.NullInt64  `gorm:"column:expiry_time"`
	RenewalCycle  sql.NullString `gorm:"column:renewal_cycle;type:varchar(20)"`
	Secret        string         `gorm:"type:varchar(100);not null"`
	ServerIP      string         `gorm:"column:server_ip;type:varchar(100);not null"`
	ServerIPV4    sql.NullString `gorm:"column:server_ip_v4;type:varchar(100)"`
	ServerIPV6    sql.NullString `gorm:"column:server_ip_v6;type:varchar(100)"`
	ExtraIPs      sql.NullString `gorm:"column:extra_ips;type:text"`
	Port          string         `gorm:"type:text;not null"`
	InterfaceName sql.NullString `gorm:"column:interface_name;type:varchar(200)"`
	Version       sql.NullString `gorm:"type:varchar(100)"`
	HTTP          int            `gorm:"column:http;not null;default:0"`
	TLS           int            `gorm:"column:tls;not null;default:0"`
	Socks         int            `gorm:"not null;default:0"`
	CreatedTime   int64          `gorm:"column:created_time;not null"`
	UpdatedTime   sql.NullInt64  `gorm:"column:updated_time"`
	Status        int            `gorm:"not null"`
	TCPListenAddr string         `gorm:"column:tcp_listen_addr;type:varchar(100);not null;default:'[::]'"`
	UDPListenAddr string         `gorm:"column:udp_listen_addr;type:varchar(100);not null;default:'[::]'"`
	Inx           int            `gorm:"not null;default:0"`
	IsRemote      int            `gorm:"column:is_remote;default:0"`
	RemoteURL     sql.NullString `gorm:"column:remote_url;type:text"`
	RemoteToken   sql.NullString `gorm:"column:remote_token;type:text"`
	RemoteConfig  sql.NullString `gorm:"column:remote_config;type:text"`
}

func (Node) TableName() string { return "node" }

type SpeedLimit struct {
	ID          int64          `gorm:"primaryKey;autoIncrement"`
	Name        string         `gorm:"type:varchar(100);not null"`
	Speed       int            `gorm:"not null"`
	TunnelID    sql.NullInt64  `gorm:"column:tunnel_id"`
	TunnelName  sql.NullString `gorm:"column:tunnel_name;type:varchar(100)"`
	CreatedTime int64          `gorm:"column:created_time;not null"`
	UpdatedTime sql.NullInt64  `gorm:"column:updated_time"`
	Status      int            `gorm:"not null"`
}

func (SpeedLimit) TableName() string { return "speed_limit" }

type StatisticsFlow struct {
	ID          int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID      int64  `gorm:"column:user_id;not null" json:"userId"`
	Flow        int64  `gorm:"not null" json:"flow"`
	TotalFlow   int64  `gorm:"column:total_flow;not null" json:"totalFlow"`
	Time        string `gorm:"type:varchar(100);not null" json:"time"`
	CreatedTime int64  `gorm:"column:created_time;not null" json:"-"`
}

func (StatisticsFlow) TableName() string { return "statistics_flow" }

type Tunnel struct {
	ID           int64          `gorm:"primaryKey;autoIncrement"`
	Name         string         `gorm:"type:varchar(100);not null"`
	TrafficRatio float64        `gorm:"column:traffic_ratio;not null;default:1.0"`
	Type         int            `gorm:"not null"`
	Protocol     string         `gorm:"type:varchar(10);not null;default:'tls'"`
	Flow         int64          `gorm:"not null"`
	CreatedTime  int64          `gorm:"column:created_time;not null"`
	UpdatedTime  int64          `gorm:"column:updated_time;not null"`
	Status       int            `gorm:"not null"`
	InIP         sql.NullString `gorm:"column:in_ip;type:text"`
	Inx          int            `gorm:"not null;default:0"`
	IPPreference string         `gorm:"column:ip_preference;type:varchar(10);not null;default:''"`
}

func (Tunnel) TableName() string { return "tunnel" }

type ChainTunnel struct {
	ID        int64          `gorm:"primaryKey;autoIncrement"`
	TunnelID  int64          `gorm:"column:tunnel_id;not null"`
	ChainType string         `gorm:"column:chain_type;type:varchar(10);not null"`
	NodeID    int64          `gorm:"column:node_id;not null"`
	Port      sql.NullInt64  `gorm:"column:port"`
	Strategy  sql.NullString `gorm:"type:varchar(10)"`
	Inx       sql.NullInt64  `gorm:"column:inx"`
	Protocol  sql.NullString `gorm:"type:varchar(10)"`
	ConnectIP sql.NullString `gorm:"column:connect_ip;type:varchar(45)"`
}

func (ChainTunnel) TableName() string { return "chain_tunnel" }

type UserTunnel struct {
	ID            int64         `gorm:"primaryKey;autoIncrement"`
	UserID        int64         `gorm:"column:user_id;not null;uniqueIndex:idx_user_tunnel_unique"`
	TunnelID      int64         `gorm:"column:tunnel_id;not null;uniqueIndex:idx_user_tunnel_unique"`
	SpeedID       sql.NullInt64 `gorm:"column:speed_id"`
	Num           int           `gorm:"not null"`
	Flow          int64         `gorm:"not null"`
	InFlow        int64         `gorm:"column:in_flow;not null;default:0"`
	OutFlow       int64         `gorm:"column:out_flow;not null;default:0"`
	FlowResetTime int64         `gorm:"column:flow_reset_time;not null"`
	ExpTime       int64         `gorm:"column:exp_time;not null"`
	Status        int           `gorm:"not null"`
}

func (UserTunnel) TableName() string { return "user_tunnel" }

type TunnelGroup struct {
	ID          int64  `gorm:"primaryKey;autoIncrement"`
	Name        string `gorm:"type:varchar(100);not null;uniqueIndex:idx_tunnel_group_name"`
	CreatedTime int64  `gorm:"column:created_time;not null"`
	UpdatedTime int64  `gorm:"column:updated_time;not null"`
	Status      int    `gorm:"not null"`
}

func (TunnelGroup) TableName() string { return "tunnel_group" }

type UserGroup struct {
	ID          int64  `gorm:"primaryKey;autoIncrement"`
	Name        string `gorm:"type:varchar(100);not null;uniqueIndex:idx_user_group_name"`
	CreatedTime int64  `gorm:"column:created_time;not null"`
	UpdatedTime int64  `gorm:"column:updated_time;not null"`
	Status      int    `gorm:"not null"`
}

func (UserGroup) TableName() string { return "user_group" }

type TunnelGroupTunnel struct {
	ID            int64 `gorm:"primaryKey;autoIncrement"`
	TunnelGroupID int64 `gorm:"column:tunnel_group_id;not null;uniqueIndex:idx_tunnel_group_tunnel_unique"`
	TunnelID      int64 `gorm:"column:tunnel_id;not null;uniqueIndex:idx_tunnel_group_tunnel_unique"`
	CreatedTime   int64 `gorm:"column:created_time;not null"`
}

func (TunnelGroupTunnel) TableName() string { return "tunnel_group_tunnel" }

type UserGroupUser struct {
	ID          int64 `gorm:"primaryKey;autoIncrement"`
	UserGroupID int64 `gorm:"column:user_group_id;not null;uniqueIndex:idx_user_group_user_unique"`
	UserID      int64 `gorm:"column:user_id;not null;uniqueIndex:idx_user_group_user_unique"`
	CreatedTime int64 `gorm:"column:created_time;not null"`
}

func (UserGroupUser) TableName() string { return "user_group_user" }

type GroupPermission struct {
	ID            int64 `gorm:"primaryKey;autoIncrement"`
	UserGroupID   int64 `gorm:"column:user_group_id;not null;uniqueIndex:idx_group_permission_unique"`
	TunnelGroupID int64 `gorm:"column:tunnel_group_id;not null;uniqueIndex:idx_group_permission_unique"`
	CreatedTime   int64 `gorm:"column:created_time;not null"`
}

func (GroupPermission) TableName() string { return "group_permission" }

type GroupPermissionGrant struct {
	ID             int64 `gorm:"primaryKey;autoIncrement"`
	UserGroupID    int64 `gorm:"column:user_group_id;not null;uniqueIndex:idx_group_permission_grant_unique"`
	TunnelGroupID  int64 `gorm:"column:tunnel_group_id;not null;uniqueIndex:idx_group_permission_grant_unique"`
	UserTunnelID   int64 `gorm:"column:user_tunnel_id;not null;uniqueIndex:idx_group_permission_grant_unique"`
	CreatedByGroup int   `gorm:"column:created_by_group;not null;default:0"`
	CreatedTime    int64 `gorm:"column:created_time;not null"`
}

func (GroupPermissionGrant) TableName() string { return "group_permission_grant" }

type ViteConfig struct {
	ID    int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	Name  string `gorm:"type:varchar(200);not null;uniqueIndex" json:"name"`
	Value string `gorm:"type:text;not null" json:"value"`
	Time  int64  `gorm:"not null" json:"time"`
}

func (ViteConfig) TableName() string { return "vite_config" }

type Announcement struct {
	ID          int64         `gorm:"primaryKey;autoIncrement" json:"id"`
	Content     string        `gorm:"type:text;not null" json:"content"`
	Enabled     int           `gorm:"not null;default:1" json:"enabled"`
	CreatedTime int64         `gorm:"column:created_time;not null" json:"created_time"`
	UpdatedTime sql.NullInt64 `gorm:"column:updated_time" json:"updated_time,omitempty"`
}

func (Announcement) TableName() string { return "announcement" }

type SchemaVersion struct {
	Version int `gorm:"not null;default:0"`
}

func (SchemaVersion) TableName() string { return "schema_version" }

type PeerShare struct {
	ID             int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	Name           string `gorm:"type:text;not null" json:"name"`
	NodeID         int64  `gorm:"column:node_id;not null" json:"nodeId"`
	Token          string `gorm:"type:text;not null;uniqueIndex" json:"token"`
	MaxBandwidth   int64  `gorm:"column:max_bandwidth;default:0" json:"maxBandwidth"`
	ExpiryTime     int64  `gorm:"column:expiry_time;default:0" json:"expiryTime"`
	PortRangeStart int    `gorm:"column:port_range_start;default:0" json:"portRangeStart"`
	PortRangeEnd   int    `gorm:"column:port_range_end;default:0" json:"portRangeEnd"`
	CurrentFlow    int64  `gorm:"column:current_flow;default:0" json:"currentFlow"`
	IsActive       int    `gorm:"column:is_active;default:1" json:"isActive"`
	CreatedTime    int64  `gorm:"column:created_time;not null" json:"createdTime"`
	UpdatedTime    int64  `gorm:"column:updated_time;not null" json:"updatedTime"`
	AllowedDomains string `gorm:"column:allowed_domains;type:text;default:''" json:"allowedDomains"`
	AllowedIPs     string `gorm:"column:allowed_ips;type:text;default:''" json:"allowedIps"`
}

func (PeerShare) TableName() string { return "peer_share" }

type PeerShareRuntime struct {
	ID            int64  `gorm:"primaryKey;autoIncrement"`
	ShareID       int64  `gorm:"column:share_id;not null;index:idx_peer_share_runtime_share_node_status"`
	NodeID        int64  `gorm:"column:node_id;not null;index:idx_peer_share_runtime_share_node_status"`
	ReservationID string `gorm:"column:reservation_id;type:text;not null;uniqueIndex"`
	ResourceKey   string `gorm:"column:resource_key;type:text;not null;uniqueIndex"`
	BindingID     string `gorm:"column:binding_id;type:text;not null;default:'';index:idx_peer_share_runtime_binding_id"`
	Role          string `gorm:"type:text;not null;default:''"`
	ChainName     string `gorm:"column:chain_name;type:text;not null;default:''"`
	ServiceName   string `gorm:"column:service_name;type:text;not null;default:''"`
	Protocol      string `gorm:"type:text;not null;default:'tls'"`
	Strategy      string `gorm:"type:text;not null;default:'round'"`
	Port          int    `gorm:"not null;default:0"`
	Target        string `gorm:"type:text;not null;default:''"`
	Applied       int    `gorm:"not null;default:0"`
	Status        int    `gorm:"not null;default:1;index:idx_peer_share_runtime_share_node_status"`
	CreatedTime   int64  `gorm:"column:created_time;not null"`
	UpdatedTime   int64  `gorm:"column:updated_time;not null"`
}

func (PeerShareRuntime) TableName() string { return "peer_share_runtime" }

type FederationTunnelBinding struct {
	ID              int64  `gorm:"primaryKey;autoIncrement"`
	TunnelID        int64  `gorm:"column:tunnel_id;not null;uniqueIndex:idx_federation_tunnel_binding_unique;index:idx_federation_tunnel_binding_tunnel"`
	NodeID          int64  `gorm:"column:node_id;not null;uniqueIndex:idx_federation_tunnel_binding_unique"`
	ChainType       int    `gorm:"column:chain_type;not null;uniqueIndex:idx_federation_tunnel_binding_unique"`
	HopInx          int    `gorm:"column:hop_inx;not null;default:0;uniqueIndex:idx_federation_tunnel_binding_unique"`
	RemoteURL       string `gorm:"column:remote_url;type:text;not null"`
	ResourceKey     string `gorm:"column:resource_key;type:text;not null;uniqueIndex"`
	RemoteBindingID string `gorm:"column:remote_binding_id;type:text;not null"`
	AllocatedPort   int    `gorm:"column:allocated_port;not null"`
	Status          int    `gorm:"not null;default:1;index:idx_federation_tunnel_binding_tunnel"`
	CreatedTime     int64  `gorm:"column:created_time;not null"`
	UpdatedTime     int64  `gorm:"column:updated_time;not null"`
}

func (FederationTunnelBinding) TableName() string { return "federation_tunnel_binding" }

// ─── Backup / Import-Export Structs ──────────────────────────────────
// These are not GORM models; they define the JSON wire format for the
// backup/restore API and MUST keep their existing json tags unchanged.

// BackupData represents the full backup structure.
type BackupData struct {
	Version      string              `json:"version"`
	ExportedAt   int64               `json:"exportedAt"`
	Users        []UserBackup        `json:"users,omitempty"`
	Nodes        []NodeBackup        `json:"nodes,omitempty"`
	Tunnels      []TunnelBackup      `json:"tunnels,omitempty"`
	Forwards     []ForwardBackup     `json:"forwards,omitempty"`
	UserTunnels  []UserTunnelBackup  `json:"userTunnels,omitempty"`
	SpeedLimits  []SpeedLimitBackup  `json:"speedLimits,omitempty"`
	TunnelGroups []TunnelGroupBackup `json:"tunnelGroups,omitempty"`
	UserGroups   []UserGroupBackup   `json:"userGroups,omitempty"`
	Permissions  []PermissionBackup  `json:"permissions,omitempty"`
	Configs      map[string]string   `json:"configs,omitempty"`
}

type UserBackup struct {
	ID            int64  `json:"id"`
	User          string `json:"user"`
	Pwd           string `json:"pwd"`
	RoleID        int    `json:"roleId"`
	ExpTime       int64  `json:"expTime"`
	Flow          int64  `json:"flow"`
	InFlow        int64  `json:"inFlow"`
	OutFlow       int64  `json:"outFlow"`
	FlowResetTime int64  `json:"flowResetTime"`
	Num           int    `json:"num"`
	CreatedTime   int64  `json:"createdTime"`
	UpdatedTime   int64  `json:"updatedTime,omitempty"`
	Status        int    `json:"status"`
}

type NodeBackup struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	Remark        string `json:"remark,omitempty"`
	ExpiryTime    int64  `json:"expiryTime,omitempty"`
	RenewalCycle  string `json:"renewalCycle,omitempty"`
	Secret        string `json:"secret"`
	ServerIP      string `json:"serverIp"`
	ServerIPv4    string `json:"serverIpV4,omitempty"`
	ServerIPv6    string `json:"serverIpV6,omitempty"`
	ExtraIPs      string `json:"extraIPs,omitempty"`
	Port          string `json:"port"`
	InterfaceName string `json:"interfaceName,omitempty"`
	Version       string `json:"version,omitempty"`
	HTTP          int    `json:"http"`
	TLS           int    `json:"tls"`
	Socks         int    `json:"socks"`
	CreatedTime   int64  `json:"createdTime"`
	UpdatedTime   int64  `json:"updatedTime,omitempty"`
	Status        int    `json:"status"`
	TCPListenAddr string `json:"tcpListenAddr"`
	UDPListenAddr string `json:"udpListenAddr"`
	Inx           int    `json:"inx"`
	IsRemote      int    `json:"isRemote"`
	RemoteURL     string `json:"remoteUrl,omitempty"`
	RemoteToken   string `json:"remoteToken,omitempty"`
	RemoteConfig  string `json:"remoteConfig,omitempty"`
}

type TunnelBackup struct {
	ID           int64               `json:"id"`
	Name         string              `json:"name"`
	TrafficRatio float64             `json:"trafficRatio"`
	Type         int                 `json:"type"`
	Protocol     string              `json:"protocol"`
	Flow         int64               `json:"flow"`
	CreatedTime  int64               `json:"createdTime"`
	UpdatedTime  int64               `json:"updatedTime"`
	Status       int                 `json:"status"`
	InIP         string              `json:"inIp,omitempty"`
	Inx          int                 `json:"inx"`
	IPPreference string              `json:"ipPreference,omitempty"`
	ChainTunnels []ChainTunnelBackup `json:"chainTunnels,omitempty"`
}

type ChainTunnelBackup struct {
	ID        int64  `json:"id"`
	TunnelID  int64  `json:"tunnelId"`
	ChainType string `json:"chainType"`
	NodeID    int64  `json:"nodeId"`
	Port      int    `json:"port,omitempty"`
	Strategy  string `json:"strategy,omitempty"`
	Inx       int    `json:"inx,omitempty"`
	Protocol  string `json:"protocol,omitempty"`
}

type ForwardBackup struct {
	ID           int64                `json:"id"`
	UserID       int64                `json:"userId"`
	UserName     string               `json:"userName"`
	Name         string               `json:"name"`
	TunnelID     int64                `json:"tunnelId"`
	RemoteAddr   string               `json:"remoteAddr"`
	Strategy     string               `json:"strategy"`
	InFlow       int64                `json:"inFlow"`
	OutFlow      int64                `json:"outFlow"`
	CreatedTime  int64                `json:"createdTime"`
	UpdatedTime  int64                `json:"updatedTime"`
	Status       int                  `json:"status"`
	Inx          int                  `json:"inx"`
	SpeedID      *int64               `json:"speedId,omitempty"`
	ForwardPorts *[]ForwardPortBackup `json:"forwardPorts,omitempty"`
}

type ForwardPortBackup struct {
	NodeID int64 `json:"nodeId"`
	Port   int   `json:"port"`
}

type UserTunnelBackup struct {
	ID            int64 `json:"id"`
	UserID        int64 `json:"userId"`
	TunnelID      int64 `json:"tunnelId"`
	SpeedID       int64 `json:"speedId,omitempty"`
	Num           int   `json:"num"`
	Flow          int64 `json:"flow"`
	InFlow        int64 `json:"inFlow"`
	OutFlow       int64 `json:"outFlow"`
	FlowResetTime int64 `json:"flowResetTime"`
	ExpTime       int64 `json:"expTime"`
	Status        int   `json:"status"`
}

type SpeedLimitBackup struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Speed       int64  `json:"speed"`
	TunnelID    *int64 `json:"tunnelId,omitempty"`
	TunnelName  string `json:"tunnelName,omitempty"`
	CreatedTime int64  `json:"createdTime"`
	UpdatedTime int64  `json:"updatedTime,omitempty"`
	Status      int    `json:"status"`
}

type TunnelGroupBackup struct {
	ID          int64   `json:"id"`
	Name        string  `json:"name"`
	CreatedTime int64   `json:"createdTime"`
	UpdatedTime int64   `json:"updatedTime"`
	Status      int     `json:"status"`
	Tunnels     []int64 `json:"tunnels,omitempty"`
}

type UserGroupBackup struct {
	ID          int64   `json:"id"`
	Name        string  `json:"name"`
	CreatedTime int64   `json:"createdTime"`
	UpdatedTime int64   `json:"updatedTime"`
	Status      int     `json:"status"`
	Users       []int64 `json:"users,omitempty"`
}

type PermissionBackup struct {
	ID             int64                   `json:"id"`
	UserGroupID    int64                   `json:"userGroupId"`
	TunnelGroupID  int64                   `json:"tunnelGroupId"`
	CreatedTime    int64                   `json:"createdTime"`
	CreatedByGroup int                     `json:"createdByGroup"`
	Grants         []PermissionGrantBackup `json:"grants,omitempty"`
}

type PermissionGrantBackup struct {
	ID             int64 `json:"id"`
	UserGroupID    int64 `json:"userGroupId"`
	TunnelGroupID  int64 `json:"tunnelGroupId"`
	UserTunnelID   int64 `json:"userTunnelId"`
	CreatedTime    int64 `json:"createdTime"`
	CreatedByGroup int   `json:"createdByGroup"`
}

// ImportResult contains the result of an import operation.
type ImportResult struct {
	UsersImported        int         `json:"usersImported"`
	NodesImported        int         `json:"nodesImported"`
	TunnelsImported      int         `json:"tunnelsImported"`
	ForwardsImported     int         `json:"forwardsImported"`
	UserTunnelsImported  int         `json:"userTunnelsImported"`
	SpeedLimitsImported  int         `json:"speedLimitsImported"`
	TunnelGroupsImported int         `json:"tunnelGroupsImported"`
	UserGroupsImported   int         `json:"userGroupsImported"`
	PermissionsImported  int         `json:"permissionsImported"`
	ConfigsImported      int         `json:"configsImported"`
	AutoBackup           *BackupData `json:"autoBackup,omitempty"`
}

// ─── View Structs (used by Repository, not GORM models) ─────────────
// These are used for JOIN query results that don't map 1:1 to a table.

// ForwardRecord is a minimal forward view used by control plane and flow policy.
type ForwardRecord struct {
	ID         int64
	UserID     int64
	UserName   string
	Name       string
	TunnelID   int64
	RemoteAddr string
	Strategy   string
	Status     int
	SpeedID    sql.NullInt64
}

// TunnelRecord is a minimal tunnel view used by control plane.
type TunnelRecord struct {
	ID           int64
	Type         int
	Status       int
	Flow         int64
	TrafficRatio float64
}

// ForwardPortRecord is a forward port mapping used by control plane.
type ForwardPortRecord struct {
	NodeID int64
	Port   int
	InIP   string
}

// NodeRecord is a node view used by control plane.
type NodeRecord struct {
	ID            int64
	Name          string
	ServerIP      string
	ServerIPv4    string
	ServerIPv6    string
	ExtraIPs      string
	Status        int
	PortRange     string
	TCPListenAddr string
	UDPListenAddr string
	InterfaceName string
	IsRemote      int
	RemoteURL     string
	RemoteToken   string
	RemoteConfig  string
}

type ChainNodeRecord struct {
	ChainType int
	Inx       int64
	NodeID    int64
	Port      int
	NodeName  string
	Protocol  string
	Strategy  string
	ConnectIP string
}

type UserTunnelLimiterInfo struct {
	UserTunnelID int64
	LimiterID    *int64
	Speed        *int
}

// UserFlowSnapshot holds a user's current flow counters (used by stats job).
type UserFlowSnapshot struct {
	UserID  int64
	InFlow  int64
	OutFlow int64
}

// ExpiredUserTunnel holds minimal info for an expired user_tunnel row.
type ExpiredUserTunnel struct {
	ID       int64
	UserID   int64
	TunnelID int64
}

// UserTunnelDetail is a joined view of user_tunnel + tunnel + speed_limit.
type UserTunnelDetail struct {
	ID            int64
	UserID        int64
	TunnelID      int64
	TunnelName    string
	Status        int
	TunnelFlow    int
	Flow          int64
	InFlow        int64
	OutFlow       int64
	Num           int
	FlowResetTime int64
	ExpTime       int64
	SpeedID       sql.NullInt64
	SpeedLimit    sql.NullString
	Speed         sql.NullInt64
}

// UserForwardDetail is a joined view of forward + tunnel.
type UserForwardDetail struct {
	ID         int64
	Name       string
	TunnelID   int64
	TunnelName string
	InIP       string
	InPort     sql.NullInt64
	RemoteAddr string
	InFlow     int64
	OutFlow    int64
	Status     int
	CreatedAt  int64
}
