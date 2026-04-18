package socket

import (
	"encoding/json"
	"fmt"
)

func handleDashAddService(data interface{}) error {
	return CallDashAPI("POST", "/config/services", data)
}

func handleDashUpdateService(data interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	var svcs []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(jsonData, &svcs); err != nil {
		return err
	}
	for _, svc := range svcs {
		if svc.Name == "" {
			return fmt.Errorf("service name is missing")
		}
		// Assuming we only get one element or we send the full array?
		// panel sends an array of services
	}
	return CallDashAPI("POST", "/config/services", data)
}

func handleDashDeleteService(data interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	var req struct {
		Services []string `json:"services"`
	}
	if err := json.Unmarshal(jsonData, &req); err != nil {
		return err
	}
	for _, svc := range req.Services {
		if err := CallDashAPI("DELETE", "/config/services/"+svc, nil); err != nil {
			return err
		}
	}
	return nil
}

func handleDashAddChain(data interface{}) error {
	return CallDashAPI("POST", "/config/chains", data)
}

func handleDashUpdateChain(data interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	var req struct {
		Name string `json:"name"`
	}
	// Try parsing as {"chain": "name", "data": {...}}
	var updateReq struct {
		Chain string `json:"chain"`
	}
	if err := json.Unmarshal(jsonData, &updateReq); err == nil && updateReq.Chain != "" {
		req.Name = updateReq.Chain
	} else if err := json.Unmarshal(jsonData, &req); err != nil {
		return err
	}
	if req.Name == "" {
		return fmt.Errorf("chain name is missing")
	}
	return CallDashAPI("PUT", "/config/chains/"+req.Name, data)
}

func handleDashDeleteChain(data interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	var req struct {
		Chain string `json:"chain"`
	}
	if err := json.Unmarshal(jsonData, &req); err != nil {
		// try name string directly
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
	return CallDashAPI("POST", "/config/limiters", data)
}

func handleDashUpdateLimiter(data interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	var req struct {
		Name string `json:"name"`
	}
	// Try parsing as {"limiter": "name", "data": {...}}
	var updateReq struct {
		Limiter string `json:"limiter"`
	}
	if err := json.Unmarshal(jsonData, &updateReq); err == nil && updateReq.Limiter != "" {
		req.Name = updateReq.Limiter
	} else if err := json.Unmarshal(jsonData, &req); err != nil {
		return err
	}
	
	if req.Name == "" {
		return fmt.Errorf("limiter name is missing")
	}
	return CallDashAPI("PUT", "/config/limiters/"+req.Name, data)
}

func handleDashDeleteLimiter(data interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}
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
