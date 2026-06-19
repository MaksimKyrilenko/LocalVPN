package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Config конфигурация клиента
type Config struct {
	ServerURL  string `json:"server_url"`
	AutoStart  bool   `json:"auto_start"`
	MinimizeToTray bool `json:"minimize_to_tray"`
}

// DefaultConfig возвращает конфигурацию по умолчанию
func DefaultConfig() *Config {
	return &Config{
		ServerURL:      "http://localhost:8080",
		AutoStart:      false,
		MinimizeToTray: true,
	}
}

// configPath возвращает путь к файлу конфигурации
func configPath() string {
	// На Windows: %APPDATA%\MeshVPN\config.json
	appData := os.Getenv("APPDATA")
	if appData == "" {
		appData = os.Getenv("HOME")
	}
	return filepath.Join(appData, "MeshVPN", "config.json")
}

// Load загружает конфигурацию из файла
func Load() (*Config, error) {
	path := configPath()
	
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Возвращаем конфиг по умолчанию
			cfg := DefaultConfig()
			return cfg, cfg.Save()
		}
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// Save сохраняет конфигурацию в файл
func (c *Config) Save() error {
	path := configPath()
	
	// Создаем директорию если не существует
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}
