// Package config owns typed client preferences, independent of service runtime state.
package config

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/dbpath"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/networkprefs"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverfeatures"
)

const maxFileBytes = 256 * 1024

const SingleProcess = "single-process"
const Distributed = "distributed"

type Values struct {
	Mode            *string `json:"mode,omitempty"`
	AppDBPath       *string `json:"app-db-path,omitempty"`
	ServerToken     *string `json:"server-token,omitempty"`
	ServerURL       *string `json:"server-url,omitempty"`
	Host            *string `json:"host,omitempty"`
	Port            *int    `json:"port,omitempty"`
	CollectorDBPath *string `json:"collector-db-path,omitempty"`
	ServerDBPath    *string `json:"server-db-path,omitempty"`
}

type Settings struct {
	Mode            string
	AppDBPath       string
	ServerToken     string
	ServerURL       string
	Host            string
	Port            int
	CollectorDBPath string
	ServerDBPath    string
}

func Path(explicit string) (string, error) {
	if explicit == "" {
		explicit = os.Getenv("TOKENINSIGHTS_CONFIG_PATH")
	}
	if explicit == "" {
		base := os.Getenv("XDG_CONFIG_HOME")
		if base == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			base = filepath.Join(home, ".config")
		}
		explicit = filepath.Join(base, "tokeninsights", "config.json")
	}
	return filepath.Abs(explicit)
}

func Defaults() Settings {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		base = filepath.Join(os.Getenv("HOME"), ".local", "share")
	}
	return Settings{Host: networkprefs.DefaultHost, Port: networkprefs.DefaultPort, CollectorDBPath: filepath.Join(base, "tokeninsights", "collector.sqlite"), ServerDBPath: filepath.Join(base, "tokeninsights", "server.duckdb")}
}

func Read(path string) (Values, error) {
	var values Values
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return values, nil
	}
	if err != nil {
		return values, err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return values, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxFileBytes {
		return values, fmt.Errorf("invalid config file")
	}
	body, err := io.ReadAll(io.LimitReader(f, maxFileBytes+1))
	if err != nil {
		return values, err
	}
	// Reject duplicate keys/null as well as unknown keys and trailing documents.
	decoder := json.NewDecoder(bytes.NewReader(body))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return values, fmt.Errorf("config must be a JSON object")
	}
	seen := make(map[string]bool)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return values, fmt.Errorf("invalid config JSON")
		}
		key, ok := token.(string)
		if !ok || seen[key] {
			return values, fmt.Errorf("duplicate config key")
		}
		seen[key] = true
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return values, fmt.Errorf("invalid config value for %s", key)
		}
	}
	if _, err := decoder.Token(); err != nil {
		return values, fmt.Errorf("invalid config JSON")
	}
	decoder = json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&values); err != nil {
		return Values{}, fmt.Errorf("invalid config: %w", err)
	}
	if err := decoder.Decode(new(json.RawMessage)); err != io.EOF {
		return Values{}, fmt.Errorf("expected one config document")
	}
	if err := values.Validate(); err != nil {
		return Values{}, err
	}
	return values, nil
}

func (v Values) Validate() error {
	if v.Mode != nil && *v.Mode != SingleProcess && *v.Mode != Distributed {
		return fmt.Errorf("invalid mode; use single-process or distributed")
	}
	if v.ServerToken != nil && (*v.ServerToken == "" || strings.TrimSpace(*v.ServerToken) != *v.ServerToken || strings.ContainsAny(*v.ServerToken, "\r\n\x00\t ")) {
		return fmt.Errorf("invalid server-token")
	}
	if v.ServerURL != nil {
		if err := ValidateURL(*v.ServerURL); err != nil {
			return err
		}
	}
	if v.Host != nil {
		if *v.Host == "" {
			return fmt.Errorf("host must not be empty")
		}
		if err := networkprefs.ValidateHost(*v.Host); err != nil {
			return err
		}
	}
	if v.Port != nil && (*v.Port < 0 || *v.Port > 65535) {
		return fmt.Errorf("port must be between 0 and 65535")
	}
	for _, p := range []*string{v.CollectorDBPath, v.ServerDBPath, v.AppDBPath} {
		if p != nil && (strings.TrimSpace(*p) == "" || strings.ContainsRune(*p, 0)) {
			return fmt.Errorf("database path must not be empty or contain NUL")
		}
	}
	return nil
}

func ValidateURL(value string) error {
	if value == "" {
		return nil
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed == nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" || strings.TrimSpace(value) != value {
		return fmt.Errorf("invalid server-url; use an HTTP/HTTPS endpoint without credentials, query or fragment")
	}
	return nil
}

func Resolve(path string, environment bool) (Settings, error) {
	return ResolveWithOverrides(path, environment, Values{})
}

func ResolveWithOverrides(path string, environment bool, overrides Values) (Settings, error) {
	values, err := Read(path)
	if err != nil {
		return Settings{}, err
	}
	return resolveValues(values, filepath.Dir(path), environment, overrides)
}

func ResolveValues(v Values, base string, environment bool) (Settings, error) {
	return resolveValues(v, base, environment, Values{})
}
func resolveValues(v Values, base string, environment bool, overrides Values) (Settings, error) {
	if err := v.Validate(); err != nil {
		return Settings{}, err
	}
	s := Defaults()
	if v.Mode != nil {
		s.Mode = *v.Mode
	}
	if v.ServerToken != nil {
		s.ServerToken = *v.ServerToken
	}
	if v.ServerURL != nil {
		s.ServerURL = *v.ServerURL
	}
	if v.Host != nil {
		s.Host = *v.Host
	}
	if v.Port != nil {
		s.Port = *v.Port
	}
	resolvePath := func(value string) string {
		if filepath.IsAbs(value) {
			return value
		}
		return filepath.Join(base, value)
	}
	if v.AppDBPath != nil {
		s.AppDBPath = resolvePath(*v.AppDBPath)
	}
	if v.CollectorDBPath != nil {
		s.CollectorDBPath = resolvePath(*v.CollectorDBPath)
	}
	if v.ServerDBPath != nil {
		s.ServerDBPath = resolvePath(*v.ServerDBPath)
	}
	if environment {
		if value, ok := os.LookupEnv("TOKENINSIGHTS_MODE"); ok && overrides.Mode == nil {
			s.Mode = value
		}
		if value, ok := os.LookupEnv("TOKENINSIGHTS_ACCESS_TOKEN"); ok && overrides.ServerToken == nil {
			s.ServerToken = value
		}
		if value, ok := os.LookupEnv("TOKENINSIGHTS_SERVER_URL"); ok && overrides.ServerURL == nil {
			s.ServerURL = value
		}
		if value, ok := os.LookupEnv("TOKENINSIGHTS_HOST"); ok && overrides.Host == nil {
			s.Host = value
		}
		if value, ok := os.LookupEnv("TOKENINSIGHTS_PORT"); ok && overrides.Port == nil {
			port, err := strconv.Atoi(value)
			if err != nil {
				return Settings{}, fmt.Errorf("invalid TOKENINSIGHTS_PORT")
			}
			s.Port = port
		}
		for key, target := range map[string]*string{"TOKENINSIGHTS_COLLECTOR_DB_PATH": &s.CollectorDBPath, "TOKENINSIGHTS_SERVER_DB_PATH": &s.ServerDBPath, "TOKENINSIGHTS_APP_DB_PATH": &s.AppDBPath} {
			if value := os.Getenv(key); value != "" {
				absolute, err := filepath.Abs(value)
				if err != nil {
					return Settings{}, err
				}
				*target = absolute
			}
		}
	}
	if overrides.Mode != nil {
		s.Mode = *overrides.Mode
	}
	if overrides.AppDBPath != nil {
		s.AppDBPath = *overrides.AppDBPath
	}
	if overrides.ServerURL != nil {
		s.ServerURL = *overrides.ServerURL
	}
	if overrides.ServerToken != nil {
		s.ServerToken = *overrides.ServerToken
	}
	if overrides.Host != nil {
		s.Host = *overrides.Host
	}
	if overrides.Port != nil {
		s.Port = *overrides.Port
	}
	if overrides.CollectorDBPath != nil {
		s.CollectorDBPath = *overrides.CollectorDBPath
	}
	if overrides.ServerDBPath != nil {
		s.ServerDBPath = *overrides.ServerDBPath
	}
	if err := (Values{Mode: pointerMode(s.EffectiveMode()), AppDBPath: optionalPath(s.AppDBPath), ServerURL: &s.ServerURL, Host: &s.Host, Port: &s.Port, CollectorDBPath: &s.CollectorDBPath, ServerDBPath: &s.ServerDBPath}).Validate(); err != nil {
		return Settings{}, err
	}
	if s.ServerToken != "" {
		if err := (Values{ServerToken: &s.ServerToken}).Validate(); err != nil {
			return Settings{}, err
		}
	}
	return s, nil
}

// ValidateDestination checks complete runtime preferences, allowing incremental config setup.
func pointerMode(value string) *string { return &value }
func optionalPath(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
func (s Settings) EffectiveMode() string {
	if s.Mode != "" {
		return s.Mode
	}
	if s.ServerURL != "" {
		return Distributed
	}
	return SingleProcess
}
func (s Settings) ExpectedKind() serverfeatures.Kind {
	if s.EffectiveMode() == Distributed {
		return serverfeatures.Hosted
	}
	return serverfeatures.Personal
}
func (s Settings) ApplicationPath() string {
	if s.AppDBPath != "" {
		return s.AppDBPath
	}
	path, err := dbpath.Canonical(s.ServerDBPath)
	if err != nil {
		path = s.ServerDBPath
	}
	return filepath.Join(filepath.Dir(path), "app.sqlite")
}
func (s Settings) ValidateDestination() error {
	if s.EffectiveMode() != SingleProcess && s.EffectiveMode() != Distributed {
		return fmt.Errorf("invalid mode")
	}
	if s.EffectiveMode() == Distributed {
		if s.ServerURL == "" || s.ServerToken == "" {
			return fmt.Errorf("distributed mode requires server-url and server-token")
		}
		if err := ValidateURL(s.ServerURL); err != nil {
			return err
		}
	} else if s.ServerURL != "" || s.ServerToken != "" {
		return fmt.Errorf("single-process mode cannot use remote credentials or server-url")
	}
	return nil
}

func (v Values) Get(key string, base string) (string, error) {
	s, err := ResolveValues(v, base, false)
	if err != nil {
		return "", err
	}
	switch key {
	case "mode":
		return s.EffectiveMode(), nil
	case "app-db-path":
		return s.ApplicationPath(), nil
	case "server-token":
		if s.ServerToken == "" {
			return "unset", nil
		}
		return "configured", nil
	case "server-url":
		return s.ServerURL, nil
	case "host":
		return s.Host, nil
	case "port":
		return strconv.Itoa(s.Port), nil
	case "collector-db-path":
		return s.CollectorDBPath, nil
	case "server-db-path":
		return s.ServerDBPath, nil
	default:
		return "", fmt.Errorf("unknown config key %q", key)
	}
}

func (v *Values) Set(key, value string, remove bool) error {
	stringValue := func() *string {
		if remove {
			return nil
		}
		return &value
	}
	switch key {
	case "mode":
		v.Mode = stringValue()
	case "server-token":
		v.ServerToken = stringValue()
	case "server-url":
		v.ServerURL = stringValue()
	case "host":
		v.Host = stringValue()
	case "port":
		v.Port = nil
		if !remove {
			port, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("port must be an integer")
			}
			v.Port = &port
		}
	case "collector-db-path", "server-db-path", "app-db-path":
		if !remove {
			if strings.TrimSpace(value) == "" {
				return fmt.Errorf("empty database path")
			}
			absolute, err := filepath.Abs(value)
			if err != nil {
				return err
			}
			value = absolute
		}
		switch key {
		case "collector-db-path":
			v.CollectorDBPath = stringValue()
		case "app-db-path":
			v.AppDBPath = stringValue()
		default:
			v.ServerDBPath = stringValue()
		}
	default:
		return fmt.Errorf("unknown config key %q", key)
	}
	return v.Validate()
}

func Update(ctx context.Context, path string, change func(*Values) error) error {
	// Validate against a read before creating any directories or lock files.
	initial, err := Read(path)
	if err != nil {
		return err
	}
	if err := change(&initial); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	release, err := db.AcquireWriterLock(ctx, path)
	if err != nil {
		return err
	}
	defer release()
	values, err := Read(path)
	if err != nil {
		return err
	}
	if err := change(&values); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".config-*")
	if err != nil {
		return err
	}
	defer func() { _ = file.Close(); _ = os.Remove(file.Name()) }()
	if err := json.NewEncoder(file).Encode(values); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(file.Name(), path); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer func() { _ = dir.Close() }()
	return dir.Sync()
}
