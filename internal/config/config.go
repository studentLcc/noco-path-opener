package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type Config struct {
	Host              string        `json:"host"`
	Port              int           `json:"port"`
	AllowedRoots      []string      `json:"allowed_roots"`
	MaxGUIWindows     int           `json:"max_gui_windows"`
	NocoDBURL         string        `json:"nocodb_url"`
	NocoDBToken       string        `json:"nocodb_token"`
	RemoteNocoDBURL   string        `json:"remote_nocodb_url"`
	RemoteNocoDBToken string        `json:"remote_nocodb_token"`
	SyncProfiles      []SyncProfile `json:"sync_profiles"`
}

type SyncProfile struct {
	Name              string   `json:"name"`
	LocalBaseID       string   `json:"local_base_id"`
	LocalTableID      string   `json:"local_table_id"`
	LocalLookupField  string   `json:"local_lookup_field"`
	RemoteBaseID      string   `json:"remote_base_id"`
	RemoteTableID     string   `json:"remote_table_id"`
	RemoteLookupField string   `json:"remote_lookup_field"`
	SyncFields        []string `json:"sync_fields"`
}

func Default() Config {
	return Config{
		Host:              "0.0.0.0",
		Port:              6666,
		AllowedRoots:      []string{},
		MaxGUIWindows:     1,
		NocoDBURL:         "http://localhost:8080",
		NocoDBToken:       "",
		RemoteNocoDBURL:   "",
		RemoteNocoDBToken: "",
		SyncProfiles:      []SyncProfile{},
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
	if err := c.validateSyncProfiles(); err != nil {
		return err
	}
	return nil
}

func (c Config) validateSyncProfiles() error {
	if len(c.SyncProfiles) == 0 {
		return nil
	}
	if strings.TrimSpace(c.RemoteNocoDBURL) == "" {
		return fmt.Errorf("remote_nocodb_url is required when sync_profiles is not empty")
	}
	if strings.TrimSpace(c.RemoteNocoDBToken) == "" {
		return fmt.Errorf("remote_nocodb_token is required when sync_profiles is not empty")
	}

	names := make(map[string]int, len(c.SyncProfiles))
	for i, profile := range c.SyncProfiles {
		name := strings.TrimSpace(profile.Name)
		if name == "" {
			return fmt.Errorf("sync_profiles[%d].name must not be empty", i)
		}
		if original, exists := names[name]; exists {
			return fmt.Errorf("sync_profiles[%d].name duplicates sync_profiles[%d].name", i, original)
		}
		names[name] = i

		if strings.TrimSpace(profile.LocalBaseID) == "" {
			return fmt.Errorf("sync_profiles[%d].local_base_id must not be empty", i)
		}
		if strings.TrimSpace(profile.LocalTableID) == "" {
			return fmt.Errorf("sync_profiles[%d].local_table_id must not be empty", i)
		}
		if strings.TrimSpace(profile.LocalLookupField) == "" {
			return fmt.Errorf("sync_profiles[%d].local_lookup_field must not be empty", i)
		}
		if strings.TrimSpace(profile.RemoteBaseID) == "" {
			return fmt.Errorf("sync_profiles[%d].remote_base_id must not be empty", i)
		}
		if strings.TrimSpace(profile.RemoteTableID) == "" {
			return fmt.Errorf("sync_profiles[%d].remote_table_id must not be empty", i)
		}
		if strings.TrimSpace(profile.RemoteLookupField) == "" {
			return fmt.Errorf("sync_profiles[%d].remote_lookup_field must not be empty", i)
		}
		if len(profile.SyncFields) == 0 {
			return fmt.Errorf("sync_profiles[%d].sync_fields must contain at least one field", i)
		}

		fields := make(map[string]int, len(profile.SyncFields))
		for j, field := range profile.SyncFields {
			field = strings.TrimSpace(field)
			if field == "" {
				return fmt.Errorf("sync_profiles[%d].sync_fields[%d] must not be empty", i, j)
			}
			if original, exists := fields[field]; exists {
				return fmt.Errorf("sync_profiles[%d].sync_fields[%d] duplicates sync_profiles[%d].sync_fields[%d]", i, j, i, original)
			}
			fields[field] = j
		}
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
