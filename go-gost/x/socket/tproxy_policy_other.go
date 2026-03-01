//go:build !linux

package socket

import "errors"

type tproxyPolicyRequest struct {
	Port  int `json:"port"`
	Mark  int `json:"mark"`
	Table int `json:"table"`
}

func (r *tproxyPolicyRequest) normalize() error {
	if r == nil {
		return errors.New("策略路由参数不能为空")
	}
	if r.Port <= 0 || r.Port > 65535 {
		return errors.New("invalid port")
	}
	if r.Mark <= 0 {
		r.Mark = 11
	}
	if r.Table <= 0 {
		r.Table = 111
	}
	return nil
}

func probeTProxyCapability() (map[string]interface{}, error) {
	capability := map[string]interface{}{
		"supported": false,
		"platform":  "non-linux",
	}
	return capability, errors.New("tproxy is only supported on linux")
}

func ensureTProxyPolicy(req tproxyPolicyRequest) error {
	_ = req
	return errors.New("tproxy is only supported on linux")
}

func deleteTProxyPolicy(req tproxyPolicyRequest) error {
	_ = req
	return errors.New("tproxy is only supported on linux")
}

func restoreTProxyPolicies() error {
	return nil
}

func getTProxyPolicyStateSummary() (map[string]interface{}, error) {
	return map[string]interface{}{
		"stateFile": "",
		"entries":   []map[string]interface{}{},
	}, nil
}
