-- SQLite Auto-generated schema
-- This will be executed automatically on startup if tables don't exist

CREATE TABLE IF NOT EXISTS forward (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id INTEGER NOT NULL,
  user_name VARCHAR(100) NOT NULL,
  name VARCHAR(100) NOT NULL,
  tunnel_id INTEGER NOT NULL,
  remote_addr TEXT NOT NULL,
  strategy VARCHAR(100) NOT NULL DEFAULT 'fifo',
  in_flow INTEGER NOT NULL DEFAULT 0,
  out_flow INTEGER NOT NULL DEFAULT 0,
  created_time INTEGER NOT NULL,
  updated_time INTEGER NOT NULL,
  status INTEGER NOT NULL,
  inx INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS forward_port (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  forward_id INTEGER NOT NULL,
  node_id INTEGER NOT NULL,
  port INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS node (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
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
  created_time INTEGER NOT NULL,
  updated_time INTEGER,
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
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name VARCHAR(100) NOT NULL,
  speed INTEGER NOT NULL,
  tunnel_id INTEGER NOT NULL,
  tunnel_name VARCHAR(100) NOT NULL,
  created_time INTEGER NOT NULL,
  updated_time INTEGER,
  status INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS statistics_flow (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id INTEGER NOT NULL,
  flow INTEGER NOT NULL,
  total_flow INTEGER NOT NULL,
  time VARCHAR(100) NOT NULL,
  created_time INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS tunnel (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name VARCHAR(100) NOT NULL,
  traffic_ratio REAL NOT NULL DEFAULT 1.0,
  type INTEGER NOT NULL,
  protocol VARCHAR(10) NOT NULL DEFAULT 'tls',
  flow INTEGER NOT NULL,
  created_time INTEGER NOT NULL,
  updated_time INTEGER NOT NULL,
  status INTEGER NOT NULL,
  in_ip TEXT,
  inx INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS chain_tunnel (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tunnel_id INTEGER NOT NULL ,
    chain_type VARCHAR(10) NOT NULL,
    node_id INTEGER NOT NULL ,
    port INTEGER,
    strategy VARCHAR(10),
    inx  INTEGER,
    protocol  VARCHAR(10)
);


CREATE TABLE IF NOT EXISTS user (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user VARCHAR(100) NOT NULL,
  pwd VARCHAR(100) NOT NULL,
  role_id INTEGER NOT NULL,
  exp_time INTEGER NOT NULL,
  flow INTEGER NOT NULL,
  in_flow INTEGER NOT NULL DEFAULT 0,
  out_flow INTEGER NOT NULL DEFAULT 0,
  flow_reset_time INTEGER NOT NULL,
  num INTEGER NOT NULL,
  created_time INTEGER NOT NULL,
  updated_time INTEGER,
  status INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS user_tunnel (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id INTEGER NOT NULL,
  tunnel_id INTEGER NOT NULL,
  speed_id INTEGER,
  num INTEGER NOT NULL,
  flow INTEGER NOT NULL,
  in_flow INTEGER NOT NULL DEFAULT 0,
  out_flow INTEGER NOT NULL DEFAULT 0,
  flow_reset_time INTEGER NOT NULL,
  exp_time INTEGER NOT NULL,
  status INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS tunnel_group (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name VARCHAR(100) NOT NULL,
  created_time INTEGER NOT NULL,
  updated_time INTEGER NOT NULL,
  status INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS user_group (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name VARCHAR(100) NOT NULL,
  created_time INTEGER NOT NULL,
  updated_time INTEGER NOT NULL,
  status INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS tunnel_group_tunnel (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  tunnel_group_id INTEGER NOT NULL,
  tunnel_id INTEGER NOT NULL,
  created_time INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS user_group_user (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_group_id INTEGER NOT NULL,
  user_id INTEGER NOT NULL,
  created_time INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS group_permission (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_group_id INTEGER NOT NULL,
  tunnel_group_id INTEGER NOT NULL,
  created_time INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS group_permission_grant (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_group_id INTEGER NOT NULL,
  tunnel_group_id INTEGER NOT NULL,
  user_tunnel_id INTEGER NOT NULL,
  created_by_group INTEGER NOT NULL DEFAULT 0,
  created_time INTEGER NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_tunnel_group_name ON tunnel_group(name);
CREATE UNIQUE INDEX IF NOT EXISTS idx_user_group_name ON user_group(name);
CREATE UNIQUE INDEX IF NOT EXISTS idx_tunnel_group_tunnel_unique ON tunnel_group_tunnel(tunnel_group_id, tunnel_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_user_group_user_unique ON user_group_user(user_group_id, user_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_group_permission_unique ON group_permission(user_group_id, tunnel_group_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_group_permission_grant_unique ON group_permission_grant(user_group_id, tunnel_group_id, user_tunnel_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_user_tunnel_unique ON user_tunnel(user_id, tunnel_id);

CREATE TABLE IF NOT EXISTS vite_config (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name VARCHAR(200) NOT NULL UNIQUE,
  value VARCHAR(200) NOT NULL,
  time INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS peer_share (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    node_id INTEGER NOT NULL,
    token TEXT NOT NULL UNIQUE,
    max_bandwidth INTEGER DEFAULT 0,
    expiry_time INTEGER DEFAULT 0,
    port_range_start INTEGER DEFAULT 0,
    port_range_end INTEGER DEFAULT 0,
    current_flow INTEGER DEFAULT 0,
    is_active INTEGER DEFAULT 1,
    created_time INTEGER NOT NULL,
    updated_time INTEGER NOT NULL,
    allowed_domains TEXT DEFAULT ''
);

