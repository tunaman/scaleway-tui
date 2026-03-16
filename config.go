package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/scaleway/scaleway-sdk-go/scw"
)

// ─────────────────────────────────────────────
// Config — SDK-native profile loading
//
// The Scaleway SDK reads ~/.config/scw/config.yaml natively via scw.LoadConfig().
// We use scw.Config.GetProfile(name) to load individual profiles by name, and
// scw.WithProfile(p) to build a fully-configured client from each one.
// This means we never need to parse YAML ourselves or depend on gopkg.in/yaml.v3
// for Scaleway credentials.
//
// Our own ~/.config/scw-tui/config.yaml only stores UI preferences (last profile).
// ─────────────────────────────────────────────

// tuiConfig is stored at ~/.config/scw-tui/config.yaml.
// It is intentionally minimal — credentials live only in the Scaleway config.
type tuiConfig struct {
	ActiveProfile string `json:"active_profile"`
}

// loadTUIConfig reads our TUI config. Returns empty struct on first run.
func loadTUIConfig() tuiConfig {
	home, err := os.UserHomeDir()
	if err != nil {
		return tuiConfig{}
	}
	data, err := os.ReadFile(filepath.Join(home, ".config", "scw-tui", "config.json"))
	if err != nil {
		return tuiConfig{}
	}
	var cfg tuiConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return tuiConfig{}
	}
	return cfg
}

// saveTUIConfig writes our TUI config (best-effort).
func saveTUIConfig(cfg tuiConfig) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	dir := filepath.Join(home, ".config", "scw-tui")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(dir, "config.json"), data, 0o600)
}

// ─────────────────────────────────────────────
// Client factory — uses SDK Profile directly
// ─────────────────────────────────────────────

// buildClients constructs a Scaleway API client and a MinIO/S3 client from a
// named profile loaded via the SDK's own config parser.
func buildClients(cfg *scw.Config, profileName string) (*scw.Client, *minio.Client, string, error) {
	prof, err := cfg.GetProfile(profileName)
	if err != nil {
		return nil, nil, "", fmt.Errorf("profile %q: %w", profileName, err)
	}

	if prof.SecretKey == nil || *prof.SecretKey == "" {
		return nil, nil, "", fmt.Errorf("profile %q has no secret_key", profileName)
	}
	if prof.AccessKey == nil || *prof.AccessKey == "" {
		return nil, nil, "", fmt.Errorf("profile %q has no access_key", profileName)
	}

	scwClient, err := scw.NewClient(scw.WithProfile(prof))
	if err != nil {
		return nil, nil, "", fmt.Errorf("building Scaleway client: %w", err)
	}

	// Derive S3 endpoint from profile region, falling back to nl-ams.
	region := "nl-ams"
	if prof.DefaultRegion != nil && *prof.DefaultRegion != "" {
		region = string(*prof.DefaultRegion)
	}

	mc, err := minio.New(fmt.Sprintf("s3.%s.scw.cloud", region), &minio.Options{
		Creds:  credentials.NewStaticV4(*prof.AccessKey, *prof.SecretKey, ""),
		Secure: true,
	})
	if err != nil {
		return nil, nil, "", fmt.Errorf("building S3 client: %w", err)
	}

	return scwClient, mc, region, nil
}
