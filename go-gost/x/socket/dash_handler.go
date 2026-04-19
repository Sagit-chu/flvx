package socket

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

func sanitizeForDash(v interface{}) interface{} {
	switch val := v.(type) {
	case map[string]interface{}:
		newMap := make(map[string]interface{})
		for k, v := range val {
			if k == "failTimeout" {
				if s, ok := v.(string); ok {
					s = strings.TrimSuffix(s, "s")
					if i, err := strconv.ParseInt(s, 10, 64); err == nil {
						newMap[k] = i
						continue
					}
				}
			}
			newMap[k] = sanitizeForDash(v)
		}
		return newMap
	case []interface{}:
		newSlice := make([]interface{}, len(val))
		for i, v := range val {
			newSlice[i] = sanitizeForDash(v)
		}
		return newSlice
	default:
		return v
	}
}

func handleDashAddService(data interface{}) error {
	sanitized := sanitizeForDash(data)
	jsonData, _ := json.Marshal(sanitized)
	
	var svcs []interface{}
	if err := json.Unmarshal(jsonData, &svcs); err == nil {
		for _, s := range svcs {
			if err := CallDashAPI("POST", "/config/services", s); err != nil {
				return err
			}
		}
		return nil
	}
	return CallDashAPI("POST", "/config/services", sanitized)
}

func handleDashUpdateService(data interface{}) error {
	sanitized := sanitizeForDash(data)
	jsonData, _ := json.Marshal(sanitized)
	
	var svcs []map[string]interface{}
	if err := json.Unmarshal(jsonData, &svcs); err == nil {
		for _, s := range svcs {
			name, _ := s["name"].(string)
			if name == "" {
				continue
			}
			if err := CallDashAPI("POST", "/config/services", s); err != nil {
				if err2 := CallDashAPI("PUT", "/config/services/"+name, s); err2 != nil {
					return fmt.Errorf("POST err: %v; PUT err: %v", err, err2)
				}
			}
		}
		return nil
	}
	
	return CallDashAPI("POST", "/config/services", sanitized)
}

func handleDashDeleteService(data interface{}) error {
	jsonData, _ := json.Marshal(data)
	var req struct {
		Services []string `json:"services"`
	}
	if err := json.Unmarshal(jsonData, &req); err != nil {
		return err
	}
	for _, svc := range req.Services {
		_ = CallDashAPI("DELETE", "/config/services/"+svc, nil)
	}
	return nil
}

func handleDashAddChain(data interface{}) error {
	sanitized := sanitizeForDash(data)
	jsonData, _ := json.Marshal(sanitized)
	var items []interface{}
	if err := json.Unmarshal(jsonData, &items); err == nil {
		for _, item := range items {
			if err := CallDashAPI("POST", "/config/chains", item); err != nil {
				return err
			}
		}
		return nil
	}
	return CallDashAPI("POST", "/config/chains", sanitized)
}

func handleDashUpdateChain(data interface{}) error {
	sanitized := sanitizeForDash(data)
	jsonData, _ := json.Marshal(sanitized)
	var items []map[string]interface{}
	if err := json.Unmarshal(jsonData, &items); err == nil {
		for _, item := range items {
			name, _ := item["name"].(string)
			if name == "" {
				continue
			}
			if err := CallDashAPI("POST", "/config/chains", item); err != nil {
				if err2 := CallDashAPI("PUT", "/config/chains/"+name, item); err2 != nil {
					return fmt.Errorf("POST err: %v; PUT err: %v", err, err2)
				}
			}
		}
		return nil
	}
	return handleDashAddChain(data)
}

func handleDashDeleteChain(data interface{}) error {
	jsonData, _ := json.Marshal(data)
	var req struct {
		Chain string `json:"chain"`
	}
	if err := json.Unmarshal(jsonData, &req); err != nil {
		var name string
		if err := json.Unmarshal(jsonData, &name); err == nil {
			req.Chain = name
		}
	}
	if req.Chain == "" {
		return fmt.Errorf("chain name is missing")
	}
	return CallDashAPI("DELETE", "/config/chains/"+req.Chain, nil)
}

func handleDashAddLimiter(data interface{}) error {
	sanitized := sanitizeForDash(data)
	jsonData, _ := json.Marshal(sanitized)
	var items []interface{}
	if err := json.Unmarshal(jsonData, &items); err == nil {
		for _, item := range items {
			if err := CallDashAPI("POST", "/config/limiters", item); err != nil {
				return err
			}
		}
		return nil
	}
	return CallDashAPI("POST", "/config/limiters", sanitized)
}

func handleDashUpdateLimiter(data interface{}) error {
	sanitized := sanitizeForDash(data)
	jsonData, _ := json.Marshal(sanitized)
	var items []map[string]interface{}
	if err := json.Unmarshal(jsonData, &items); err == nil {
		for _, item := range items {
			name, _ := item["name"].(string)
			if name == "" {
				continue
			}
			if err := CallDashAPI("POST", "/config/limiters", item); err != nil {
				if err2 := CallDashAPI("PUT", "/config/limiters/"+name, item); err2 != nil {
					return fmt.Errorf("POST err: %v; PUT err: %v", err, err2)
				}
			}
		}
		return nil
	}
	return handleDashAddLimiter(data)
}

func handleDashDeleteLimiter(data interface{}) error {
	jsonData, _ := json.Marshal(data)
	var req struct {
		Limiter string `json:"limiter"`
	}
	if err := json.Unmarshal(jsonData, &req); err != nil {
		var name string
		if err := json.Unmarshal(jsonData, &name); err == nil {
			req.Limiter = name
		}
	}
	if req.Limiter == "" {
		return fmt.Errorf("limiter name is missing")
	}
	return CallDashAPI("DELETE", "/config/limiters/"+req.Limiter, nil)
}
