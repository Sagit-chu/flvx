package dashruntime

type RelayRulePayload struct {
	ID          string             `json:"id"`
	Protocol    string             `json:"protocol"`
	Listen      string             `json:"listen"`
	Enabled     bool               `json:"enabled"`
	Description *string            `json:"description,omitempty"`
	StagePools  []StagePoolPayload `json:"stage_pools"`
	TargetPool  TargetPoolPayload  `json:"target_pool"`
	Traffic     *TrafficPayload    `json:"traffic,omitempty"`
}

type StagePoolPayload struct {
	Policy   string                `json:"policy"`
	Backends []StageBackendPayload `json:"backends"`
}

type StageBackendPayload struct {
	ID            string  `json:"id"`
	Server        string  `json:"server"`
	Token         string  `json:"token"`
	Enabled       bool    `json:"enabled"`
	Weight        int     `json:"weight"`
	BindInterface *string `json:"bind_interface,omitempty"`
}

type TargetPoolPayload struct {
	Policy   string                 `json:"policy"`
	Backends []TargetBackendPayload `json:"backends"`
}

type TargetBackendPayload struct {
	ID            string  `json:"id"`
	Address       string  `json:"address"`
	Enabled       bool    `json:"enabled"`
	Weight        int     `json:"weight"`
	BindInterface *string `json:"bind_interface,omitempty"`
}

type TrafficPayload struct {
	Accounting struct {
		Enabled bool `json:"enabled"`
	} `json:"accounting"`
	RateLimit struct {
		UpstreamBytesPerSecond   *int64 `json:"upstream_bytes_per_second,omitempty"`
		DownstreamBytesPerSecond *int64 `json:"downstream_bytes_per_second,omitempty"`
	} `json:"rate_limit"`
}
