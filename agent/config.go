package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

const configFileName = "config.json"

const installDir = `C:\Program Files\LanAgent`
const logsDir = `C:\Program Files\LanAgent\logs`
const dataDir = `C:\Program Files\LanAgent\data`
const filesDir = `C:\Program Files\LanAgent\data\files`
const oldDataDir = `C:\LanAgent`

type Config struct {
	ServerURL    string `json:"server_url"`
	Token        string `json:"token"`
	DeviceID     string `json:"device_id"`
	AgentName    string `json:"agent_name"`
	ListenPort   int    `json:"listen_port"`
	EnrollSecret string `json:"enroll_secret"`
}

func getConfigPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(exe), configFileName), nil
}

func LoadConfig() (*Config, error) {
	path, err := getConfigPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	if cfg.ServerURL == "" || cfg.Token == "" {
		return nil, errors.New("server_url and token are required")
	}

	return &cfg, nil
}

func SaveConfig(cfg *Config) error {
	path, err := getConfigPath()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

func migrateOldData() {
	if _, err := os.Stat(oldDataDir); os.IsNotExist(err) {
		return
	}

	os.MkdirAll(dataDir, 0755)
	os.MkdirAll(logsDir, 0755)

	entries, err := os.ReadDir(oldDataDir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		src := oldDataDir + `\` + entry.Name()
		dst := dataDir + `\` + entry.Name()
		if entry.Name() == "setup.log" || entry.Name() == "agent.log" {
			dst = logsDir + `\` + entry.Name()
		}
		os.Rename(src, dst)
	}

	os.RemoveAll(oldDataDir)
}
