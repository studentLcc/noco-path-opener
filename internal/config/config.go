package config

import (
	"encoding/json"
	"fmt"
	"os"
)

type Config struct {
	Host          string   `json:"host"`
	Port          int      `json:"port"`
	AllowedRoots  []string `json:"allowed_roots"`
	MaxGUIWindows int      `json:"max_gui_windows"`
	NocoDBURL     string   `json:"nocodb_url"`
	NocoDBToken   string   `json:"nocodb_token"`
}

func Default() Config {
	return Config{
		Host:          "0.0.0.0",
		Port:          6666,
		AllowedRoots:  []string{},
		MaxGUIWindows: 1,
		NocoDBURL:     "http://localhost:8080",
		NocoDBToken:   "",
	}
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			cfg := Default()
			if err := writeDefault(path, cfg); err != nil {
				return Config{}, err
			}
			return cfg, nil
		}
		return Config{}, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	if cfg.MaxGUIWindows == 0 {
		cfg.MaxGUIWindows = Default().MaxGUIWindows
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) Validate() error {
	if c.Host == "" {
		return fmt.Errorf("host must not be empty")
	}
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}
	if c.AllowedRoots == nil {
		return fmt.Errorf("allowed_roots must be an array")
	}
	for i, root := range c.AllowedRoots {
		if root == "" {
			return fmt.Errorf("allowed_roots[%d] must not be empty", i)
		}
	}
	if c.MaxGUIWindows < 1 {
		return fmt.Errorf("max_gui_windows must be at least 1")
	}
	return nil
}

func writeDefault(path string, cfg Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}
