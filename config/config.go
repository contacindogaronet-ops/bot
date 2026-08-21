package config

import (
	"errors"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
	"github.com/rs/zerolog"
)

// Config holds all runtime parameters for the JARGO Userbot.
type Config struct {
	AppID                  int
	AppHash                string
	PhoneNumber            string
	SessionPath            string
	DownloadDir            string
	TriggerCmd             string
	ChunkSize              int
	MaxConcurrentDownloads int
	LogLevel               string
	LogPretty              bool
}

// LoadConfig parses environment variables and .env file with zero-alloc defaults.
func LoadConfig(log zerolog.Logger) (*Config, error) {
	// Attempt to load .env if available, ignore error if missing (e.g. running in pure env container)
	_ = godotenv.Load(".env")

	appIDStr := getEnv("TG_APP_ID", "")
	if appIDStr == "" {
		return nil, errors.New("TG_APP_ID is required (obtain from https://my.telegram.org)")
	}

	appID, err := strconv.Atoi(appIDStr)
	if err != nil || appID <= 0 {
		return nil, errors.New("invalid TG_APP_ID: must be a positive integer")
	}

	appHash := getEnv("TG_APP_HASH", "")
	if appHash == "" {
		return nil, errors.New("TG_APP_HASH is required (obtain from https://my.telegram.org)")
	}

	chunkSize, err := strconv.Atoi(getEnv("TG_CHUNK_SIZE", "524288")) // 512 KB default
	if err != nil || chunkSize <= 0 {
		chunkSize = 512 * 1024
	}

	maxConcurrent, err := strconv.Atoi(getEnv("TG_MAX_CONCURRENT_DOWNLOADS", "3"))
	if err != nil || maxConcurrent <= 0 {
		maxConcurrent = 3
	}

	triggerCmd := strings.TrimSpace(getEnv("TG_TRIGGER_CMD", "d"))
	if triggerCmd == "" {
		triggerCmd = "d"
	}

	cfg := &Config{
		AppID:                  appID,
		AppHash:                appHash,
		PhoneNumber:            getEnv("TG_PHONE_NUMBER", ""),
		SessionPath:            getEnv("TG_SESSION_PATH", "session.json"),
		DownloadDir:            getEnv("TG_DOWNLOAD_DIR", "downloads"),
		TriggerCmd:             triggerCmd,
		ChunkSize:              chunkSize,
		MaxConcurrentDownloads: maxConcurrent,
		LogLevel:               getEnv("LOG_LEVEL", "info"),
		LogPretty:              getEnvAsBool("LOG_PRETTY", true),
	}

	log.Debug().
		Int("app_id", cfg.AppID).
		Str("session_path", cfg.SessionPath).
		Str("download_dir", cfg.DownloadDir).
		Str("trigger", cfg.TriggerCmd).
		Int("chunk_size_bytes", cfg.ChunkSize).
		Int("max_concurrency", cfg.MaxConcurrentDownloads).
		Msg("Configuration loaded successfully")

	return cfg, nil
}

func getEnv(key, defaultVal string) string {
	if val, ok := os.LookupEnv(key); ok {
		trimmed := strings.TrimSpace(val)
		if trimmed != "" {
			return trimmed
		}
	}
	return defaultVal
}

func getEnvAsBool(key string, defaultVal bool) bool {
	if val, ok := os.LookupEnv(key); ok {
		b, err := strconv.ParseBool(strings.TrimSpace(val))
		if err == nil {
			return b
		}
	}
	return defaultVal
}
