package internal

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	Host         string
	Port         string
	DataDir      string
	DatabasePath string
	AdminKey     string
	APIKey       string
	LoginURL     string
	ChromeExec   string
	LogLevel     string
	BrowserHeadless bool
	NoVNCURL       string
}

var Cfg Config

func LoadConfig() {
	_ = godotenv.Load()
	dataDir := env("DATA_DIR", "./data")
	Cfg = Config{
		Host:         env("HOST", "0.0.0.0"),
		Port:         env("PORT", "18013"),
		DataDir:      dataDir,
		DatabasePath: env("DATABASE_PATH", filepath.Join(dataDir, "kling-creator-01.sqlite")),
		AdminKey:     env("KLING_CREATOR_ADMIN_KEY", env("ADMIN_KEY", "change-me-admin-key")),
		APIKey:       env("KLING_CREATOR_AUTH_KEY", env("API_KEY", "change-me-api-key")),
		LoginURL:     env("KLING_LOGIN_URL", "https://klingai.com/app"),
		ChromeExec:   env("CHROME_EXECUTABLE", ""),
		LogLevel:     env("LOG_LEVEL", "info"),
		BrowserHeadless: strings.ToLower(strings.TrimSpace(env("BROWSER_HEADLESS", "true"))) != "false",
		NoVNCURL:       strings.TrimSpace(env("NOVNC_URL", "")),
	}
	_ = os.MkdirAll(Cfg.DataDir, 0o755)
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
