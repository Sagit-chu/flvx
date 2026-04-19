package socket

import (
	"encoding/json"
	"os"
	"sync"

	"github.com/go-gost/x/config"
)

// configMutex 保护配置文件的并发写入
var configMutex sync.Mutex

var (
	// IsDashMode indicates if the current agent is running in Dash mode.
	IsDashMode bool
)

func isDashRuntime() bool {
	return IsDashMode
}

func InitDashMode() {
	// Parse config.json to check if engine is "dash"
	configBytes, err := os.ReadFile("config.json")
	if err != nil {
		return
	}
	var cfg struct {
		Engine string `json:"engine"`
	}
	if err := json.Unmarshal(configBytes, &cfg); err != nil {
		return
	}
	IsDashMode = cfg.Engine == "dash"
}

func saveConfig() error {
	configMutex.Lock()
	defer configMutex.Unlock()

	file := "gost.json"

	cfg := config.Global()

	// Use a map to ensure we have full control over the JSON structure
	// and specifically avoid "services": null
	services := cfg.Services
	if services == nil {
		services = []*config.ServiceConfig{}
	}

	data := map[string]interface{}{
		"services": services,
		"chains":   cfg.Chains,
		"authers":  cfg.Authers,
		"limiters": cfg.Limiters,
		"api":      cfg.API,
	}

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(file, jsonData, 0644)
}
