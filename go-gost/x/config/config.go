package config

import (
	"encoding/json"
	"io"
	"sync"
	"time"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

var (
	v = viper.GetViper()
)

func init() {
	v.SetConfigName("gost")
	v.AddConfigPath("/etc/gost/")
	v.AddConfigPath("$HOME/.gost/")
	v.AddConfigPath(".")
}

var (
	global    = &Config{}
	globalMux sync.RWMutex
)

func Global() *Config {
	globalMux.RLock()
	defer globalMux.RUnlock()

	cfg := &Config{}
	*cfg = *global
	return cfg
}

func Set(c *Config) {
	globalMux.Lock()
	defer globalMux.Unlock()

	global = c
}

func OnUpdate(f func(c *Config) error) error {
	globalMux.Lock()
	defer globalMux.Unlock()

	return f(global)
}

type LogConfig struct {
	Output   string             `yaml:",omitempty" json:"output,omitempty"`
	Level    string             `yaml:",omitempty" json:"level,omitempty"`
	Format   string             `yaml:",omitempty" json:"format,omitempty"`
	Rotation *LogRotationConfig `yaml:",omitempty" json:"rotation,omitempty"`
}

type LogRotationConfig struct {
	// MaxSize is the maximum size in megabytes of the log file before it gets
	// rotated. It defaults to 100 megabytes.
	MaxSize int `yaml:"maxSize,omitempty" json:"maxSize,omitempty"`
	// MaxAge is the maximum number of days to retain old log files based on the
	// timestamp encoded in their filename.  Note that a day is defined as 24
	// hours and may not exactly correspond to calendar days due to daylight
	// savings, leap seconds, etc. The default is not to remove old log files
	// based on age.
	MaxAge int `yaml:"maxAge,omitempty" json:"maxAge,omitempty"`
	// MaxBackups is the maximum number of old log files to retain.  The default
	// is to retain all old log files (though MaxAge may still cause them to get
	// deleted.)
	MaxBackups int `yaml:"maxBackups,omitempty" json:"maxBackups,omitempty"`
	// LocalTime determines if the time used for formatting the timestamps in
	// backup files is the computer's local time. The default is to use UTC
	// time.
	LocalTime bool `yaml:"localTime,omitempty" json:"localTime,omitempty"`
	// Compress determines if the rotated log files should be compressed
	// using gzip. The default is not to perform compression.
	Compress bool `yaml:"compress,omitempty" json:"compress,omitempty"`
}

type LoggerConfig struct {
	Name string     `json:"name"`
	Log  *LogConfig `yaml:",omitempty" json:"log,omitempty"`
}

type ProfilingConfig struct {
	Addr string `json:"addr"`
}

type APIConfig struct {
	Addr       string      `json:"addr"`
	PathPrefix string      `yaml:"pathPrefix,omitempty" json:"pathPrefix,omitempty"`
	AccessLog  bool        `yaml:"accesslog,omitempty" json:"accesslog,omitempty"`
	Auth       *AuthConfig `yaml:",omitempty" json:"auth,omitempty"`
	Auther     string      `yaml:",omitempty" json:"auther,omitempty"`
}

type MetricsConfig struct {
	Addr   string      `json:"addr"`
	Path   string      `yaml:",omitempty" json:"path,omitempty"`
	Auth   *AuthConfig `yaml:",omitempty" json:"auth,omitempty"`
	Auther string      `yaml:",omitempty" json:"auther,omitempty"`
}

type TLSConfig struct {
	CertFile   string      `yaml:"certFile,omitempty" json:"certFile,omitempty"`
	KeyFile    string      `yaml:"keyFile,omitempty" json:"keyFile,omitempty"`
	CAFile     string      `yaml:"caFile,omitempty" json:"caFile,omitempty"`
	Secure     bool        `yaml:",omitempty" json:"secure,omitempty"`
	ServerName string      `yaml:"serverName,omitempty" json:"serverName,omitempty"`
	Options    *TLSOptions `yaml:",omitempty" json:"options,omitempty"`

	// for auto-generated default certificate.
	Validity     time.Duration `yaml:",omitempty" json:"validity,omitempty"`
	CommonName   string        `yaml:"commonName,omitempty" json:"commonName,omitempty"`
	Organization string        `yaml:",omitempty" json:"organization,omitempty"`
}

type TLSOptions struct {
	MinVersion   string   `yaml:"minVersion,omitempty" json:"minVersion,omitempty"`
	MaxVersion   string   `yaml:"maxVersion,omitempty" json:"maxVersion,omitempty"`
	CipherSuites []string `yaml:"cipherSuites,omitempty" json:"cipherSuites,omitempty"`
	ALPN         []string `yaml:"alpn,omitempty" json:"alpn,omitempty"`
}

type PluginConfig struct {
	Type    string        `json:"type"`
	Addr    string        `json:"addr"`
	TLS     *TLSConfig    `yaml:",omitempty" json:"tls,omitempty"`
	Timeout time.Duration `yaml:",omitempty" json:"timeout,omitempty"`
	Token   string        `yaml:",omitempty" json:"token,omitempty"`
}

type AutherConfig struct {
	Name   string        `json:"name"`
	Auths  []*AuthConfig `yaml:",omitempty" json:"auths,omitempty"`
	Reload time.Duration `yaml:",omitempty" json:"reload,omitempty"`
	File   *FileLoader   `yaml:",omitempty" json:"file,omitempty"`
	Redis  *RedisLoader  `yaml:",omitempty" json:"redis,omitempty"`
	HTTP   *HTTPLoader   `yaml:"http,omitempty" json:"http,omitempty"`
	Plugin *PluginConfig `yaml:",omitempty" json:"plugin,omitempty"`
}

type AuthConfig struct {
	Username string `json:"username"`
	Password string `yaml:",omitempty" json:"password,omitempty"`
}

type SelectorConfig struct {
	Strategy    string        `json:"strategy"`
	MaxFails    int           `yaml:"maxFails" json:"maxFails"`
	FailTimeout time.Duration `yaml:"failTimeout" json:"failTimeout"`
}

type AdmissionConfig struct {
	Name string `json:"name"`
	// Deprecated: use whitelist instead
	Reverse   bool          `yaml:",omitempty" json:"reverse,omitempty"`
	Whitelist bool          `yaml:",omitempty" json:"whitelist,omitempty"`
	Matchers  []string      `yaml:",omitempty" json:"matchers,omitempty"`
	Reload    time.Duration `yaml:",omitempty" json:"reload,omitempty"`
	File      *FileLoader   `yaml:",omitempty" json:"file,omitempty"`
	Redis     *RedisLoader  `yaml:",omitempty" json:"redis,omitempty"`
	HTTP      *HTTPLoader   `yaml:"http,omitempty" json:"http,omitempty"`
	Plugin    *PluginConfig `yaml:",omitempty" json:"plugin,omitempty"`
}

type BypassConfig struct {
	Name string `json:"name"`
	// Deprecated: use whitelist instead
	Reverse   bool          `yaml:",omitempty" json:"reverse,omitempty"`
	Whitelist bool          `yaml:",omitempty" json:"whitelist,omitempty"`
	Matchers  []string      `yaml:",omitempty" json:"matchers,omitempty"`
	Reload    time.Duration `yaml:",omitempty" json:"reload,omitempty"`
	File      *FileLoader   `yaml:",omitempty" json:"file,omitempty"`
	Redis     *RedisLoader  `yaml:",omitempty" json:"redis,omitempty"`
	HTTP      *HTTPLoader   `yaml:"http,omitempty" json:"http,omitempty"`
	Plugin    *PluginConfig `yaml:",omitempty" json:"plugin,omitempty"`
}

type FileLoader struct {
	Path string `json:"path"`
}

type RedisLoader struct {
	Addr     string `json:"addr"`
	DB       int    `yaml:",omitempty" json:"db,omitempty"`
	Username string `yaml:",omitempty" json:"username,omitempty"`
	Password string `yaml:",omitempty" json:"password,omitempty"`
	Key      string `yaml:",omitempty" json:"key,omitempty"`
	Type     string `yaml:",omitempty" json:"type,omitempty"`
}

type HTTPLoader struct {
	URL     string        `yaml:"url" json:"url"`
	Timeout time.Duration `yaml:",omitempty" json:"timeout,omitempty"`
}

type NameserverConfig struct {
	Addr     string        `json:"addr"`
	Chain    string        `yaml:",omitempty" json:"chain,omitempty"`
	Prefer   string        `yaml:",omitempty" json:"prefer,omitempty"`
	ClientIP string        `yaml:"clientIP,omitempty" json:"clientIP,omitempty"`
	Hostname string        `yaml:",omitempty" json:"hostname,omitempty"`
	TTL      time.Duration `yaml:",omitempty" json:"ttl,omitempty"`
	Timeout  time.Duration `yaml:",omitempty" json:"timeout,omitempty"`
	Async    bool          `yaml:",omitempty" json:"async,omitempty"`
	Only     string        `yaml:",omitempty" json:"only,omitempty"`
}

type ResolverConfig struct {
	Name        string              `json:"name"`
	Nameservers []*NameserverConfig `yaml:",omitempty" json:"nameservers,omitempty"`
	Plugin      *PluginConfig       `yaml:",omitempty" json:"plugin,omitempty"`
}

type HostMappingConfig struct {
	IP       string   `json:"ip"`
	Hostname string   `json:"hostname"`
	Aliases  []string `yaml:",omitempty" json:"aliases,omitempty"`
}

type HostsConfig struct {
	Name     string               `json:"name"`
	Mappings []*HostMappingConfig `yaml:",omitempty" json:"mappings,omitempty"`
	Reload   time.Duration        `yaml:",omitempty" json:"reload,omitempty"`
	File     *FileLoader          `yaml:",omitempty" json:"file,omitempty"`
	Redis    *RedisLoader         `yaml:",omitempty" json:"redis,omitempty"`
	HTTP     *HTTPLoader          `yaml:"http,omitempty" json:"http,omitempty"`
	Plugin   *PluginConfig        `yaml:",omitempty" json:"plugin,omitempty"`
}

type IngressRuleConfig struct {
	Hostname string `json:"hostname"`
	Endpoint string `json:"endpoint"`
}

type IngressConfig struct {
	Name   string               `json:"name"`
	Rules  []*IngressRuleConfig `yaml:",omitempty" json:"rules,omitempty"`
	Reload time.Duration        `yaml:",omitempty" json:"reload,omitempty"`
	File   *FileLoader          `yaml:",omitempty" json:"file,omitempty"`
	Redis  *RedisLoader         `yaml:",omitempty" json:"redis,omitempty"`
	HTTP   *HTTPLoader          `yaml:"http,omitempty" json:"http,omitempty"`
	Plugin *PluginConfig        `yaml:",omitempty" json:"plugin,omitempty"`
}

type SDConfig struct {
	Name   string        `json:"name"`
	Plugin *PluginConfig `yaml:",omitempty" json:"plugin,omitempty"`
}

type RouterRouteConfig struct {
	// Deprecated: use dst instead
	Net     string `yaml:",omitempty" json:"net,omitempty"`
	Dst     string `yaml:",omitempty" json:"dst,omitempty"`
	Gateway string `json:"gateway"`
}

type RouterConfig struct {
	Name   string               `json:"name"`
	Routes []*RouterRouteConfig `yaml:",omitempty" json:"routes,omitempty"`
	Reload time.Duration        `yaml:",omitempty" json:"reload,omitempty"`
	File   *FileLoader          `yaml:",omitempty" json:"file,omitempty"`
	Redis  *RedisLoader         `yaml:",omitempty" json:"redis,omitempty"`
	HTTP   *HTTPLoader          `yaml:"http,omitempty" json:"http,omitempty"`
	Plugin *PluginConfig        `yaml:",omitempty" json:"plugin,omitempty"`
}

type RecorderConfig struct {
	Name   string         `json:"name"`
	File   *FileRecorder  `yaml:",omitempty" json:"file,omitempty"`
	TCP    *TCPRecorder   `yaml:"tcp,omitempty" json:"tcp,omitempty"`
	HTTP   *HTTPRecorder  `yaml:"http,omitempty" json:"http,omitempty"`
	Redis  *RedisRecorder `yaml:",omitempty" json:"redis,omitempty"`
	Plugin *PluginConfig  `yaml:",omitempty" json:"plugin,omitempty"`
}

type FileRecorder struct {
	Path     string             `json:"path"`
	Sep      string             `yaml:",omitempty" json:"sep,omitempty"`
	Rotation *LogRotationConfig `yaml:",omitempty" json:"rotation,omitempty"`
}

type TCPRecorder struct {
	Addr    string        `json:"addr"`
	Timeout time.Duration `json:"timeout"`
}

type HTTPRecorder struct {
	URL     string            `yaml:"url" json:"url"`
	Timeout time.Duration     `yaml:",omitempty" json:"timeout,omitempty"`
	Header  map[string]string `yaml:",omitempty" json:"header,omitempty"`
}

type RedisRecorder struct {
	Addr     string `json:"addr"`
	DB       int    `yaml:",omitempty" json:"db,omitempty"`
	Username string `yaml:",omitempty" json:"username,omitempty"`
	Password string `yaml:",omitempty" json:"password,omitempty"`
	Key      string `yaml:",omitempty" json:"key,omitempty"`
	Type     string `yaml:",omitempty" json:"type,omitempty"`
}

type RecorderObject struct {
	Name     string         `json:"name"`
	Record   string         `json:"record"`
	Metadata map[string]any `yaml:",omitempty" json:"metadata,omitempty"`
}

type LimiterConfig struct {
	Name   string        `json:"name"`
	Limits []string      `yaml:",omitempty" json:"limits,omitempty"`
	Reload time.Duration `yaml:",omitempty" json:"reload,omitempty"`
	File   *FileLoader   `yaml:",omitempty" json:"file,omitempty"`
	Redis  *RedisLoader  `yaml:",omitempty" json:"redis,omitempty"`
	HTTP   *HTTPLoader   `yaml:"http,omitempty" json:"http,omitempty"`
	Plugin *PluginConfig `yaml:",omitempty" json:"plugin,omitempty"`
}

type ObserverConfig struct {
	Name   string        `json:"name"`
	Plugin *PluginConfig `yaml:",omitempty" json:"plugin,omitempty"`
}

type ListenerConfig struct {
	Type       string            `json:"type"`
	Chain      string            `yaml:",omitempty" json:"chain,omitempty"`
	ChainGroup *ChainGroupConfig `yaml:"chainGroup,omitempty" json:"chainGroup,omitempty"`
	Auther     string            `yaml:",omitempty" json:"auther,omitempty"`
	Authers    []string          `yaml:",omitempty" json:"authers,omitempty"`
	Auth       *AuthConfig       `yaml:",omitempty" json:"auth,omitempty"`
	TLS        *TLSConfig        `yaml:",omitempty" json:"tls,omitempty"`
	Metadata   map[string]any    `yaml:",omitempty" json:"metadata,omitempty"`
}

type HandlerConfig struct {
	Type       string            `json:"type"`
	Retries    int               `yaml:",omitempty" json:"retries,omitempty"`
	Chain      string            `yaml:",omitempty" json:"chain,omitempty"`
	ChainGroup *ChainGroupConfig `yaml:"chainGroup,omitempty" json:"chainGroup,omitempty"`
	Auther     string            `yaml:",omitempty" json:"auther,omitempty"`
	Authers    []string          `yaml:",omitempty" json:"authers,omitempty"`
	Auth       *AuthConfig       `yaml:",omitempty" json:"auth,omitempty"`
	TLS        *TLSConfig        `yaml:",omitempty" json:"tls,omitempty"`
	Limiter    string            `yaml:",omitempty" json:"limiter,omitempty"`
	Observer   string            `yaml:",omitempty" json:"observer,omitempty"`
	Metadata   map[string]any    `yaml:",omitempty" json:"metadata,omitempty"`
}

type ForwarderConfig struct {
	// Deprecated: use hop instead
	Name string `yaml:",omitempty" json:"name,omitempty"`
	// the referenced hop name
	Hop      string               `yaml:",omitempty" json:"hop,omitempty"`
	Selector *SelectorConfig      `yaml:",omitempty" json:"selector,omitempty"`
	Nodes    []*ForwardNodeConfig `json:"nodes"`
}

type ForwardNodeConfig struct {
	Name     string   `yaml:",omitempty" json:"name,omitempty"`
	Addr     string   `yaml:",omitempty" json:"addr,omitempty"`
	Network  string   `yaml:",omitempty" json:"network,omitempty"`
	Bypass   string   `yaml:",omitempty" json:"bypass,omitempty"`
	Bypasses []string `yaml:",omitempty" json:"bypasses,omitempty"`
	// Deprecated: use matcher instead
	Protocol string `yaml:",omitempty" json:"protocol,omitempty"`
	// Deprecated: use matcher instead
	Host string `yaml:",omitempty" json:"host,omitempty"`
	// Deprecated: use matcher instead
	Path string `yaml:",omitempty" json:"path,omitempty"`
	// Deprecated: use matcher instead
	Filter  *NodeFilterConfig  `yaml:",omitempty" json:"filter,omitempty"`
	Matcher *NodeMatcherConfig `yaml:",omitempty" json:"matcher,omitempty"`
	// Deprecated: use http.auth instead
	Auth     *AuthConfig     `yaml:",omitempty" json:"auth,omitempty"`
	HTTP     *HTTPNodeConfig `yaml:",omitempty" json:"http,omitempty"`
	TLS      *TLSNodeConfig  `yaml:",omitempty" json:"tls,omitempty"`
	Metadata map[string]any  `yaml:",omitempty" json:"metadata,omitempty"`
}

type HTTPURLRewriteConfig struct {
	Match       string
	Replacement string
}

type HTTPBodyRewriteConfig struct {
	// filter by MIME types
	Type        string
	Match       string
	Replacement string
}

type NodeFilterConfig struct {
	Host     string `yaml:",omitempty" json:"host,omitempty"`
	Protocol string `yaml:",omitempty" json:"protocol,omitempty"`
	Path     string `yaml:",omitempty" json:"path,omitempty"`
}

type NodeMatcherConfig struct {
	Rule     string `yaml:",omitempty" json:"rule,omitempty"`
	Priority int    `yaml:",omitempty" json:"priority,omitempty"`
}

type HTTPNodeConfig struct {
	// rewrite host header
	Host string `yaml:",omitempty" json:"host,omitempty"`
	// Deprecated: use requestHeader instead
	Header map[string]string `yaml:",omitempty" json:"header,omitempty"`
	// additional request header
	RequestHeader map[string]string `yaml:"requestHeader,omitempty" json:"requestHeader,omitempty"`
	// additional response header
	ResponseHeader map[string]string `yaml:"responseHeader,omitempty" json:"responseHeader,omitempty"`
	// Deprecated: use rewriteURL instead
	Rewrite []HTTPURLRewriteConfig `yaml:",omitempty" json:"rewrite,omitempty"`
	// rewrite URL
	RewriteURL []HTTPURLRewriteConfig `yaml:"rewriteURL,omitempty" json:"rewriteURL,omitempty"`
	// rewrite response body
	RewriteBody []HTTPBodyRewriteConfig `yaml:"rewriteBody,omitempty" json:"rewriteBody,omitempty"`
	// HTTP basic auth
	Auth *AuthConfig `yaml:",omitempty" json:"auth,omitempty"`
}

type TLSNodeConfig struct {
	ServerName string      `yaml:"serverName,omitempty" json:"serverName,omitempty"`
	Secure     bool        `yaml:",omitempty" json:"secure,omitempty"`
	Options    *TLSOptions `yaml:",omitempty" json:"options,omitempty"`
}

type DialerConfig struct {
	Type     string         `json:"type"`
	Auth     *AuthConfig    `yaml:",omitempty" json:"auth,omitempty"`
	TLS      *TLSConfig     `yaml:",omitempty" json:"tls,omitempty"`
	Metadata map[string]any `yaml:",omitempty" json:"metadata,omitempty"`
}

type ConnectorConfig struct {
	Type     string         `json:"type"`
	Auth     *AuthConfig    `yaml:",omitempty" json:"auth,omitempty"`
	TLS      *TLSConfig     `yaml:",omitempty" json:"tls,omitempty"`
	Metadata map[string]any `yaml:",omitempty" json:"metadata,omitempty"`
}

type SockOptsConfig struct {
	Mark int `yaml:",omitempty" json:"mark,omitempty"`
}

type ServiceConfig struct {
	Name string `json:"name"`
	Addr string `yaml:",omitempty" json:"addr,omitempty"`
	// Deprecated: use metadata.interface instead
	Interface string `yaml:",omitempty" json:"interface,omitempty"`
	// Deprecated: use metadata.so_mark instead
	SockOpts   *SockOptsConfig   `yaml:"sockopts,omitempty" json:"sockopts,omitempty"`
	Admission  string            `yaml:",omitempty" json:"admission,omitempty"`
	Admissions []string          `yaml:",omitempty" json:"admissions,omitempty"`
	Bypass     string            `yaml:",omitempty" json:"bypass,omitempty"`
	Bypasses   []string          `yaml:",omitempty" json:"bypasses,omitempty"`
	Resolver   string            `yaml:",omitempty" json:"resolver,omitempty"`
	Hosts      string            `yaml:",omitempty" json:"hosts,omitempty"`
	Limiter    string            `yaml:",omitempty" json:"limiter,omitempty"`
	CLimiter   string            `yaml:"climiter,omitempty" json:"climiter,omitempty"`
	RLimiter   string            `yaml:"rlimiter,omitempty" json:"rlimiter,omitempty"`
	Logger     string            `yaml:",omitempty" json:"logger,omitempty"`
	Loggers    []string          `yaml:",omitempty" json:"loggers,omitempty"`
	Observer   string            `yaml:",omitempty" json:"observer,omitempty"`
	Recorders  []*RecorderObject `yaml:",omitempty" json:"recorders,omitempty"`
	Handler    *HandlerConfig    `yaml:",omitempty" json:"handler,omitempty"`
	Listener   *ListenerConfig   `yaml:",omitempty" json:"listener,omitempty"`
	Forwarder  *ForwarderConfig  `yaml:",omitempty" json:"forwarder,omitempty"`
	Metadata   map[string]any    `yaml:",omitempty" json:"metadata,omitempty"`
	// service status, read-only
	Status *ServiceStatus `yaml:",omitempty" json:"status,omitempty"`
}

type ServiceStatus struct {
	CreateTime int64          `yaml:"createTime" json:"createTime"`
	State      string         `yaml:"state" json:"state"`
	Events     []ServiceEvent `yaml:",omitempty" json:"events,omitempty"`
	Stats      *ServiceStats  `yaml:",omitempty" json:"stats,omitempty"`
}

type ServiceEvent struct {
	Time int64  `yaml:"time" json:"time"`
	Msg  string `yaml:"msg" json:"msg"`
}

type ServiceStats struct {
	TotalConns   uint64 `yaml:"totalConns" json:"totalConns"`
	CurrentConns uint64 `yaml:"currentConns" json:"currentConns"`
	TotalErrs    uint64 `yaml:"totalErrs" json:"totalErrs"`
	InputBytes   uint64 `yaml:"inputBytes" json:"inputBytes"`
	OutputBytes  uint64 `yaml:"outputBytes" json:"outputBytes"`
}

type ChainConfig struct {
	Name     string         `json:"name"`
	Hops     []*HopConfig   `json:"hops"`
	Metadata map[string]any `yaml:",omitempty" json:"metadata,omitempty"`
}

type ChainGroupConfig struct {
	Chains   []string        `yaml:",omitempty" json:"chains,omitempty"`
	Selector *SelectorConfig `yaml:",omitempty" json:"selector,omitempty"`
}

type HopConfig struct {
	Name      string          `json:"name"`
	Interface string          `yaml:",omitempty" json:"interface,omitempty"`
	SockOpts  *SockOptsConfig `yaml:"sockopts,omitempty" json:"sockopts,omitempty"`
	Selector  *SelectorConfig `yaml:",omitempty" json:"selector,omitempty"`
	Bypass    string          `yaml:",omitempty" json:"bypass,omitempty"`
	Bypasses  []string        `yaml:",omitempty" json:"bypasses,omitempty"`
	Resolver  string          `yaml:",omitempty" json:"resolver,omitempty"`
	Hosts     string          `yaml:",omitempty" json:"hosts,omitempty"`
	Nodes     []*NodeConfig   `yaml:",omitempty" json:"nodes,omitempty"`
	Reload    time.Duration   `yaml:",omitempty" json:"reload,omitempty"`
	File      *FileLoader     `yaml:",omitempty" json:"file,omitempty"`
	Redis     *RedisLoader    `yaml:",omitempty" json:"redis,omitempty"`
	HTTP      *HTTPLoader     `yaml:"http,omitempty" json:"http,omitempty"`
	Plugin    *PluginConfig   `yaml:",omitempty" json:"plugin,omitempty"`
	Metadata  map[string]any  `yaml:",omitempty" json:"metadata,omitempty"`
}

type NodeConfig struct {
	Name      string           `json:"name"`
	Addr      string           `yaml:",omitempty" json:"addr,omitempty"`
	Network   string           `yaml:",omitempty" json:"network,omitempty"`
	Bypass    string           `yaml:",omitempty" json:"bypass,omitempty"`
	Bypasses  []string         `yaml:",omitempty" json:"bypasses,omitempty"`
	Resolver  string           `yaml:",omitempty" json:"resolver,omitempty"`
	Hosts     string           `yaml:",omitempty" json:"hosts,omitempty"`
	Connector *ConnectorConfig `yaml:",omitempty" json:"connector,omitempty"`
	Dialer    *DialerConfig    `yaml:",omitempty" json:"dialer,omitempty"`
	Interface string           `yaml:",omitempty" json:"interface,omitempty"`
	Netns     string           `yaml:",omitempty" json:"netns,omitempty"`
	SockOpts  *SockOptsConfig  `yaml:"sockopts,omitempty" json:"sockopts,omitempty"`
	// Deprecated: use matcher instead
	Filter   *NodeFilterConfig  `yaml:",omitempty" json:"filter,omitempty"`
	Matcher  *NodeMatcherConfig `yaml:",omitempty" json:"matcher,omitempty"`
	HTTP     *HTTPNodeConfig    `yaml:",omitempty" json:"http,omitempty"`
	TLS      *TLSNodeConfig     `yaml:",omitempty" json:"tls,omitempty"`
	Metadata map[string]any     `yaml:",omitempty" json:"metadata,omitempty"`
}

type Config struct {
	Services   []*ServiceConfig   `json:"services"`
	Chains     []*ChainConfig     `yaml:",omitempty" json:"chains,omitempty"`
	Hops       []*HopConfig       `yaml:",omitempty" json:"hops,omitempty"`
	Authers    []*AutherConfig    `yaml:",omitempty" json:"authers,omitempty"`
	Admissions []*AdmissionConfig `yaml:",omitempty" json:"admissions,omitempty"`
	Bypasses   []*BypassConfig    `yaml:",omitempty" json:"bypasses,omitempty"`
	Resolvers  []*ResolverConfig  `yaml:",omitempty" json:"resolvers,omitempty"`
	Hosts      []*HostsConfig     `yaml:",omitempty" json:"hosts,omitempty"`
	Ingresses  []*IngressConfig   `yaml:",omitempty" json:"ingresses,omitempty"`
	Routers    []*RouterConfig    `yaml:",omitempty" json:"routers,omitempty"`
	SDs        []*SDConfig        `yaml:"sds,omitempty" json:"sds,omitempty"`
	Recorders  []*RecorderConfig  `yaml:",omitempty" json:"recorders,omitempty"`
	Limiters   []*LimiterConfig   `yaml:",omitempty" json:"limiters,omitempty"`
	CLimiters  []*LimiterConfig   `yaml:"climiters,omitempty" json:"climiters,omitempty"`
	RLimiters  []*LimiterConfig   `yaml:"rlimiters,omitempty" json:"rlimiters,omitempty"`
	Observers  []*ObserverConfig  `yaml:",omitempty" json:"observers,omitempty"`
	Loggers    []*LoggerConfig    `yaml:",omitempty" json:"loggers,omitempty"`
	TLS        *TLSConfig         `yaml:",omitempty" json:"tls,omitempty"`
	Log        *LogConfig         `yaml:",omitempty" json:"log,omitempty"`
	Profiling  *ProfilingConfig   `yaml:",omitempty" json:"profiling,omitempty"`
	API        *APIConfig         `yaml:",omitempty" json:"api,omitempty"`
	Metrics    *MetricsConfig     `yaml:",omitempty" json:"metrics,omitempty"`
}

func (c *Config) Load() error {
	if err := v.ReadInConfig(); err != nil {
		return err
	}

	return v.Unmarshal(c)
}

func (c *Config) Read(r io.Reader) error {
	if err := v.ReadConfig(r); err != nil {
		return err
	}

	return v.Unmarshal(c)
}

func (c *Config) ReadFile(file string) error {
	v.SetConfigFile(file)
	if err := v.ReadInConfig(); err != nil {
		return err
	}
	return v.Unmarshal(c)
}

func (c *Config) Write(w io.Writer, format string) error {
	switch format {
	case "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		enc.Encode(c)
		return nil
	case "yaml":
		fallthrough
	default:
		enc := yaml.NewEncoder(w)
		defer enc.Close()
		enc.SetIndent(2)

		return enc.Encode(c)
	}
}
