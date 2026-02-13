package config

import "os"

type Config struct {
	Addr        string
	DBType      string
	DBPath      string
	DatabaseURL string
	JWTSecret   string
	LogDir      string
}

func FromEnv() Config {
	cfg := Config{
		Addr:        getEnv("SERVER_ADDR", ":6365"),
		DBType:      getEnv("DB_TYPE", "sqlite"),
		DBPath:      getEnv("DB_PATH", "/app/data/gost.db"),
		DatabaseURL: getEnv("DATABASE_URL", ""),
		JWTSecret:   getEnv("JWT_SECRET", ""),
		LogDir:      getEnv("LOG_DIR", "/app/logs"),
	}

	return cfg
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
