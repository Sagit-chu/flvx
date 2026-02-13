CREATE TABLE IF NOT EXISTS forward (
  id SERIAL PRIMARY KEY,
  user_id INTEGER NOT NULL,
  user_name VARCHAR(100) NOT NULL,
  name VARCHAR(100) NOT NULL,
  tunnel_id INTEGER NOT NULL,
  remote_addr TEXT NOT NULL,
  strategy VARCHAR(100) NOT NULL DEFAULT 'fifo',
  in_flow BIGINT NOT NULL DEFAULT 0,
  out_flow BIGINT NOT NULL DEFAULT 0,
  created_time BIGINT NOT NULL,
  updated_time BIGINT NOT NULL,
  status INTEGER NOT NULL,
  inx INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS forward_port (
  id SERIAL PRIMARY KEY,
  forward_id INTEGER NOT NULL,
  node_id INTEGER NOT NULL,
  port INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS node (
  id SERIAL PRIMARY KEY,
  name VARCHAR(100) NOT NULL,
  secret VARCHAR(100) NOT NULL,
  server_ip VARCHAR(100) NOT NULL,
  server_ip_v4 VARCHAR(100),
  server_ip_v6 VARCHAR(100),
  port TEXT NOT NULL,
  interface_name VARCHAR(200),
  version VARCHAR(100),
  http INTEGER NOT NULL DEFAULT 0,
  tls INTEGER NOT NULL DEFAULT 0,
  socks INTEGER NOT NULL DEFAULT 0,
  created_time BIGINT NOT NULL,
  updated_time BIGINT,
  status INTEGER NOT NULL,
  tcp_listen_addr VARCHAR(100) NOT NULL DEFAULT '[::]',
  udp_listen_addr VARCHAR(100) NOT NULL DEFAULT '[::]',
  inx INTEGER NOT NULL DEFAULT 0,
  is_remote INTEGER DEFAULT 0,
  remote_url TEXT,
  remote_token TEXT,
  remote_config TEXT
);

CREATE TABLE IF NOT EXISTS speed_limit (
  id SERIAL PRIMARY KEY,
  name VARCHAR(100) NOT NULL,
  speed INTEGER NOT NULL,
  tunnel_id INTEGER NOT NULL,
  tunnel_name VARCHAR(100) NOT NULL,
  created_time BIGINT NOT NULL,
  updated_time BIGINT,
  status INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS statistics_flow (
  id SERIAL PRIMARY KEY,
  user_id INTEGER NOT NULL,
  flow BIGINT NOT NULL,
  total_flow BIGINT NOT NULL,
  time VARCHAR(100) NOT NULL,
  created_time BIGINT NOT NULL
);

CREATE TABLE IF NOT EXISTS tunnel (
  id SERIAL PRIMARY KEY,
  name VARCHAR(100) NOT NULL,
  traffic_ratio DOUBLE PRECISION NOT NULL DEFAULT 1.0,
  type INTEGER NOT NULL,
  protocol VARCHAR(10) NOT NULL DEFAULT 'tls',
  flow BIGINT NOT NULL,
  created_time BIGINT NOT NULL,
  updated_time BIGINT NOT NULL,
  status INTEGER NOT NULL,
  in_ip TEXT,
  inx INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS chain_tunnel (
    id SERIAL PRIMARY KEY,
    tunnel_id INTEGER NOT NULL,
    chain_type VARCHAR(10) NOT NULL,
    node_id INTEGER NOT NULL,
    port INTEGER,
    strategy VARCHAR(10),
    inx INTEGER,
    protocol VARCHAR(10)
);

CREATE TABLE IF NOT EXISTS "user" (
  id SERIAL PRIMARY KEY,
  "user" VARCHAR(100) NOT NULL,
  pwd VARCHAR(100) NOT NULL,
  role_id INTEGER NOT NULL,
  exp_time BIGINT NOT NULL,
  flow BIGINT NOT NULL,
  in_flow BIGINT NOT NULL DEFAULT 0,
  out_flow BIGINT NOT NULL DEFAULT 0,
  flow_reset_time BIGINT NOT NULL,
  num INTEGER NOT NULL,
  created_time BIGINT NOT NULL,
  updated_time BIGINT,
  status INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS user_tunnel (
  id SERIAL PRIMARY KEY,
  user_id INTEGER NOT NULL,
  tunnel_id INTEGER NOT NULL,
  speed_id INTEGER,
  num INTEGER NOT NULL,
  flow BIGINT NOT NULL,
  in_flow BIGINT NOT NULL DEFAULT 0,
  out_flow BIGINT NOT NULL DEFAULT 0,
  flow_reset_time BIGINT NOT NULL,
  exp_time BIGINT NOT NULL,
  status INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS tunnel_group (
  id SERIAL PRIMARY KEY,
  name VARCHAR(100) NOT NULL,
  created_time BIGINT NOT NULL,
  updated_time BIGINT NOT NULL,
  status INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS user_group (
  id SERIAL PRIMARY KEY,
  name VARCHAR(100) NOT NULL,
  created_time BIGINT NOT NULL,
  updated_time BIGINT NOT NULL,
  status INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS tunnel_group_tunnel (
  id SERIAL PRIMARY KEY,
  tunnel_group_id INTEGER NOT NULL,
  tunnel_id INTEGER NOT NULL,
  created_time BIGINT NOT NULL
);

CREATE TABLE IF NOT EXISTS user_group_user (
  id SERIAL PRIMARY KEY,
  user_group_id INTEGER NOT NULL,
  user_id INTEGER NOT NULL,
  created_time BIGINT NOT NULL
);

CREATE TABLE IF NOT EXISTS group_permission (
  id SERIAL PRIMARY KEY,
  user_group_id INTEGER NOT NULL,
  tunnel_group_id INTEGER NOT NULL,
  created_time BIGINT NOT NULL
);

CREATE TABLE IF NOT EXISTS group_permission_grant (
  id SERIAL PRIMARY KEY,
  user_group_id INTEGER NOT NULL,
  tunnel_group_id INTEGER NOT NULL,
  user_tunnel_id INTEGER NOT NULL,
  created_by_group INTEGER NOT NULL DEFAULT 0,
  created_time BIGINT NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_tunnel_group_name ON tunnel_group(name);
CREATE UNIQUE INDEX IF NOT EXISTS idx_user_group_name ON user_group(name);
CREATE UNIQUE INDEX IF NOT EXISTS idx_tunnel_group_tunnel_unique ON tunnel_group_tunnel(tunnel_group_id, tunnel_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_user_group_user_unique ON user_group_user(user_group_id, user_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_group_permission_unique ON group_permission(user_group_id, tunnel_group_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_group_permission_grant_unique ON group_permission_grant(user_group_id, tunnel_group_id, user_tunnel_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_user_tunnel_unique ON user_tunnel(user_id, tunnel_id);

CREATE TABLE IF NOT EXISTS vite_config (
  id SERIAL PRIMARY KEY,
  name VARCHAR(200) NOT NULL UNIQUE,
  value VARCHAR(200) NOT NULL,
  time BIGINT NOT NULL
);

CREATE TABLE IF NOT EXISTS peer_share (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    node_id INTEGER NOT NULL,
    token TEXT NOT NULL UNIQUE,
    max_bandwidth INTEGER DEFAULT 0,
    expiry_time BIGINT DEFAULT 0,
    port_range_start INTEGER DEFAULT 0,
    port_range_end INTEGER DEFAULT 0,
    current_flow BIGINT DEFAULT 0,
    is_active INTEGER DEFAULT 1,
    created_time BIGINT NOT NULL,
    updated_time BIGINT NOT NULL,
    allowed_domains TEXT DEFAULT '',
    allowed_ips TEXT DEFAULT ''
);

CREATE TABLE IF NOT EXISTS peer_share_runtime (
    id SERIAL PRIMARY KEY,
    share_id INTEGER NOT NULL,
    node_id INTEGER NOT NULL,
    reservation_id TEXT NOT NULL UNIQUE,
    resource_key TEXT NOT NULL UNIQUE,
    binding_id TEXT NOT NULL DEFAULT '',
    role TEXT NOT NULL DEFAULT '',
    chain_name TEXT NOT NULL DEFAULT '',
    service_name TEXT NOT NULL DEFAULT '',
    protocol TEXT NOT NULL DEFAULT 'tls',
    strategy TEXT NOT NULL DEFAULT 'round',
    port INTEGER NOT NULL DEFAULT 0,
    target TEXT NOT NULL DEFAULT '',
    applied INTEGER NOT NULL DEFAULT 0,
    status INTEGER NOT NULL DEFAULT 1,
    created_time BIGINT NOT NULL,
    updated_time BIGINT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_peer_share_runtime_share_node_status ON peer_share_runtime(share_id, node_id, status);
CREATE INDEX IF NOT EXISTS idx_peer_share_runtime_binding_id ON peer_share_runtime(binding_id);

CREATE TABLE IF NOT EXISTS federation_tunnel_binding (
    id SERIAL PRIMARY KEY,
    tunnel_id INTEGER NOT NULL,
    node_id INTEGER NOT NULL,
    chain_type INTEGER NOT NULL,
    hop_inx INTEGER NOT NULL DEFAULT 0,
    remote_url TEXT NOT NULL,
    resource_key TEXT NOT NULL UNIQUE,
    remote_binding_id TEXT NOT NULL,
    allocated_port INTEGER NOT NULL,
    status INTEGER NOT NULL DEFAULT 1,
    created_time BIGINT NOT NULL,
    updated_time BIGINT NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_federation_tunnel_binding_unique ON federation_tunnel_binding(tunnel_id, node_id, chain_type, hop_inx);
CREATE INDEX IF NOT EXISTS idx_federation_tunnel_binding_tunnel ON federation_tunnel_binding(tunnel_id, status);
