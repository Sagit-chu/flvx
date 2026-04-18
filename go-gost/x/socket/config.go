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

	// Ensure API service is present for Dash REST API
	if isDashRuntime() {
		apiFound := false
		for _, s := range cfg.Services {
			if s.Name == "api" {
				apiFound = true
				break
			}
		}
		if !apiFound {
			cfg.Services = append(cfg.Services, &config.ServiceConfig{
				Name: "api",
				Addr: "127.0.0.1:19090",
				Handler: &config.HandlerConfig{
					Type: "auto",
				},
				Listener: &config.ListenerConfig{
					Type: "tcp",
				},
			})
		}
	}

	f, err := os.Create(file)
	if err != nil {
		return err
	}
	defer f.Close()

	if err := cfg.Write(f, "json"); err != nil {
		return err
	}

	return nil
}
