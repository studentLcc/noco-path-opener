package profilegen

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"noco-path-opener/internal/config"
)

func loadExistingConfigFile(path string) (config.Config, map[string]json.RawMessage, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return config.Config{}, nil, fmt.Errorf("read config: %w", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return config.Config{}, nil, fmt.Errorf("parse config: %w", err)
	}

	var cfg config.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return config.Config{}, nil, fmt.Errorf("parse config: %w", err)
	}
	if cfg.MaxGUIWindows == 0 {
		cfg.MaxGUIWindows = config.Default().MaxGUIWindows
	}

	return cfg, raw, nil
}

func appendProfileToConfigFile(path string, profile config.SyncProfile) error {
	cfg, raw, err := loadExistingConfigFile(path)
	if err != nil {
		return err
	}

	cfg.SyncProfiles = append(cfg.SyncProfiles, normalizeProfile(profile))
	if err := cfg.Validate(); err != nil {
		return err
	}

	cfgRaw, err := marshalConfigFields(cfg)
	if err != nil {
		return err
	}
	for key, value := range cfgRaw {
		raw[key] = value
	}

	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

func marshalConfigFields(cfg config.Config) (map[string]json.RawMessage, error) {
	data, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func normalizeProfile(profile config.SyncProfile) config.SyncProfile {
	profile.Name = strings.TrimSpace(profile.Name)
	profile.LocalBaseID = strings.TrimSpace(profile.LocalBaseID)
	profile.LocalTableID = strings.TrimSpace(profile.LocalTableID)
	profile.LocalLookupField = strings.TrimSpace(profile.LocalLookupField)
	profile.RemoteBaseID = strings.TrimSpace(profile.RemoteBaseID)
	profile.RemoteTableID = strings.TrimSpace(profile.RemoteTableID)
	profile.RemoteLookupField = strings.TrimSpace(profile.RemoteLookupField)

	for i, field := range profile.SyncFields {
		profile.SyncFields[i] = strings.TrimSpace(field)
	}

	return profile
}
