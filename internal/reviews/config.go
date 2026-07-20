package reviews

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var appKeyPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

type configFile struct {
	Key          string `json:"key"`
	Name         string `json:"name"`
	AppID        string `json:"appId"`
	Country      string `json:"country"`
	PollInterval string `json:"pollInterval"`
	MaxPages     int    `json:"maxPages"`
	DataDir      string `json:"dataDir"`
	ListenAddr   string `json:"listenAddr"`
}

// LoadConfig reads and validates a single-app service configuration.
func LoadConfig(path string) (AppConfig, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return AppConfig{}, fmt.Errorf("resolve app config path: %w", err)
	}
	f, err := os.Open(path)
	if err != nil {
		return AppConfig{}, fmt.Errorf("open app config: %w", err)
	}
	defer f.Close()

	var raw configFile
	dec := json.NewDecoder(io.LimitReader(f, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&raw); err != nil {
		return AppConfig{}, fmt.Errorf("decode app config: %w", err)
	}
	if err := ensureJSONEOF(dec); err != nil {
		return AppConfig{}, fmt.Errorf("decode app config: %w", err)
	}
	if strings.TrimSpace(raw.DataDir) == "" {
		return AppConfig{}, errors.New("dataDir is required")
	}

	interval, err := time.ParseDuration(raw.PollInterval)
	if err != nil || interval <= 0 {
		return AppConfig{}, fmt.Errorf("pollInterval must be a positive duration")
	}
	dataDir := raw.DataDir
	if !filepath.IsAbs(dataDir) {
		dataDir = filepath.Join(filepath.Dir(path), dataDir)
	}
	app := AppConfig{
		Key: raw.Key, Name: raw.Name, AppID: raw.AppID, Country: strings.ToLower(raw.Country),
		PollInterval: interval, MaxPages: raw.MaxPages,
		DataDir: dataDir, ListenAddr: raw.ListenAddr,
	}
	if err := validateConfig(app); err != nil {
		return AppConfig{}, err
	}
	return app, nil
}

func validateConfig(app AppConfig) error {
	if !appKeyPattern.MatchString(app.Key) {
		return errors.New("app key must contain lowercase letters, numbers, and single hyphens only")
	}
	if strings.TrimSpace(app.Name) == "" {
		return errors.New("app name is required")
	}
	if _, err := strconv.ParseUint(app.AppID, 10, 64); err != nil {
		return errors.New("appId must be numeric")
	}
	if len(app.Country) != 2 || app.Country[0] < 'a' || app.Country[0] > 'z' || app.Country[1] < 'a' || app.Country[1] > 'z' {
		return errors.New("country must be a two-letter code")
	}
	if app.PollInterval <= 0 {
		return errors.New("pollInterval must be positive")
	}
	if app.MaxPages < 1 || app.MaxPages > 10 {
		return errors.New("maxPages must be between 1 and 10")
	}
	if strings.TrimSpace(app.DataDir) == "" {
		return errors.New("dataDir is required")
	}
	if strings.TrimSpace(app.ListenAddr) == "" {
		return errors.New("listenAddr is required")
	}
	return nil
}

func ensureJSONEOF(dec *json.Decoder) error {
	var extra any
	if err := dec.Decode(&extra); err == io.EOF {
		return nil
	} else if err != nil {
		return err
	}
	return errors.New("multiple JSON values")
}
