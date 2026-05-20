package profilegen

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"noco-path-opener/internal/config"
)

type Options struct {
	ConfigPath        string
	Write             bool
	In                io.Reader
	Out               io.Writer
	Err               io.Writer
	HTTPClient        *http.Client
	LocalFieldLister  FieldLister
	RemoteFieldLister FieldLister
}

func Run(ctx context.Context, opts Options) error {
	configPath := strings.TrimSpace(opts.ConfigPath)
	if configPath == "" {
		configPath = "config.json"
	}

	cfg, _, err := loadExistingConfigFile(configPath)
	if err != nil {
		return err
	}

	in := opts.In
	if in == nil {
		in = os.Stdin
	}
	out := opts.Out
	if out == nil {
		out = os.Stdout
	}
	errOut := opts.Err
	if errOut == nil {
		errOut = os.Stderr
	}

	prompt := newPrompter(in, errOut)
	localURL, localToken, remoteURL, remoteToken, err := resolveCredentials(prompt, cfg)
	if err != nil {
		return err
	}

	localLister := opts.LocalFieldLister
	if localLister == nil {
		localLister = NewMetadataClient(MetadataConfig{
			BaseURL:    localURL,
			Token:      localToken,
			HTTPClient: opts.HTTPClient,
		})
	}
	remoteLister := opts.RemoteFieldLister
	if remoteLister == nil {
		remoteLister = NewMetadataClient(MetadataConfig{
			BaseURL:    remoteURL,
			Token:      remoteToken,
			HTTPClient: opts.HTTPClient,
		})
	}

	profile, err := generateProfile(ctx, prompt, localLister, remoteLister)
	if err != nil {
		return err
	}

	if opts.Write {
		return appendProfileToConfigFile(configPath, profile)
	}
	return writeProfileJSON(out, profile)
}

func resolveCredentials(prompt *prompter, cfg config.Config) (string, string, string, string, error) {
	localURL, err := configuredOrPrompt(prompt, cfg.NocoDBURL, "Local NocoDB URL: ")
	if err != nil {
		return "", "", "", "", err
	}
	localToken, err := configuredOrPrompt(prompt, cfg.NocoDBToken, "Local NocoDB token: ")
	if err != nil {
		return "", "", "", "", err
	}
	remoteURL, err := configuredOrPrompt(prompt, cfg.RemoteNocoDBURL, "Remote NocoDB URL: ")
	if err != nil {
		return "", "", "", "", err
	}
	remoteToken, err := configuredOrPrompt(prompt, cfg.RemoteNocoDBToken, "Remote NocoDB token: ")
	if err != nil {
		return "", "", "", "", err
	}
	return localURL, localToken, remoteURL, remoteToken, nil
}

func configuredOrPrompt(prompt *prompter, configured string, label string) (string, error) {
	value := strings.TrimSpace(configured)
	if value != "" {
		return value, nil
	}
	return prompt.required(label)
}

func writeProfileJSON(w io.Writer, profile config.SyncProfile) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(profile); err != nil {
		return fmt.Errorf("write profile JSON: %w", err)
	}
	return nil
}
