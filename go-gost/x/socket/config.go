package socket

import (
	"os"
	"sync"

	"github.com/go-gost/x/config"
)

// configMutex 保护配置文件的并发写入
var configMutex sync.Mutex

func saveConfig() error {
	configMutex.Lock()
	defer configMutex.Unlock()

	file := "gost.json"

	f, err := os.Create(file)
	if err != nil {
		return err
	}
	defer f.Close()

	if err := config.Global().Write(f, "json"); err != nil {
		return err
	}

	return nil
}
