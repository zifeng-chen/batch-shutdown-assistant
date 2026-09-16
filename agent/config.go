package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
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
	ServerURL  string `json:"server_url"`
	Token      string `json:"token"`
	DeviceID   string `json:"device_id"`
	AgentName  string `json:"agent_name"`
	ListenPort int    `json:"listen_port"`
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

var encryptionKey = []byte("LanAgent2024Secr") // 16 bytes for AES-128

func EncryptToken(plaintext string) (string, error) {
	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func DecryptToken(encoded string) (string, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", errors.New("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
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
