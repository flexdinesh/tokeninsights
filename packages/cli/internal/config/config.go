// Package config resolves command-specific client preferences at the executable boundary.
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
)

const maxFileBytes = 256 * 1024

type CollectorValues struct {
	DBPath *string `json:"db-path,omitempty"`
}
type LocalValues struct {
	AppDBPath    *string `json:"app-db-path,omitempty"`
	ServerDBPath *string `json:"server-db-path,omitempty"`
	Host         *string `json:"host,omitempty"`
	Port         *int    `json:"port,omitempty"`
}
type DistributedValues struct {
	ServerURL   *string `json:"server-url,omitempty"`
	ServerToken *string `json:"server-token,omitempty"`
}
type Values struct {
	Collector   CollectorValues   `json:"collector,omitempty"`
	InProcess   LocalValues       `json:"in-process,omitempty"`
	Distributed DistributedValues `json:"distributed,omitempty"`
}

// Overrides describes flags, never the persisted configuration.
type Overrides struct {
	AppDBPath, ServerDBPath, CollectorDBPath, Host, ServerURL, ServerToken *string
	Port                                                                   *int
}
type LocalSettings struct {
	AppDBPath, ServerDBPath, CollectorDBPath, Host string
	Port                                           int
}
type SyncSettings struct{ CollectorDBPath, ServerURL, ServerToken string }
type BrowseSettings struct{ ServerURL string }

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
func Defaults() LocalSettings {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		base = filepath.Join(os.Getenv("HOME"), ".local", "share")
	}
	return LocalSettings{Host: networkprefs.DefaultHost, Port: networkprefs.DefaultPort,
		CollectorDBPath: filepath.Join(base, "tokeninsights", "collector.sqlite"),
		ServerDBPath:    filepath.Join(base, "tokeninsights", "server.sqlite")}
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
	decoder := json.NewDecoder(bytes.NewReader(body))
	if err := objectKeys(decoder); err != nil {
		return values, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return values, fmt.Errorf("expected one config document")
	}
	decoder = json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&values); err != nil {
		return Values{}, fmt.Errorf("invalid grouped config; use collector, in-process and distributed groups (flat config and mode are removed)")
	}
	return values, nil
}

// Validate every object, including nested groups, without echoing private values.
func objectKeys(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return fmt.Errorf("config must contain JSON objects")
	}
	return objectBody(decoder)
}
func objectBody(decoder *json.Decoder) error {
	seen := make(map[string]bool)
	for decoder.More() {
		token, err := decoder.Token()
		key, ok := token.(string)
		if err != nil || !ok || seen[key] {
			return fmt.Errorf("duplicate or invalid config key")
		}
		seen[key] = true
		value, err := decoder.Token()
		if err != nil || value == nil {
			return fmt.Errorf("invalid config value")
		}
		if delimiter, ok := value.(json.Delim); ok {
			if delimiter != '{' {
				return fmt.Errorf("config arrays are unsupported")
			}
			if err := objectBody(decoder); err != nil {
				return err
			}
		}
	}
	token, err := decoder.Token()
	if err != nil || token != json.Delim('}') {
		return fmt.Errorf("invalid config JSON")
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
func validateToken(value string) error {
	if value != "" && (strings.TrimSpace(value) != value || strings.ContainsAny(value, "\r\n\x00\t ")) {
		return fmt.Errorf("invalid server-token")
	}
	return nil
}
func validatePath(value string) error {
	if strings.TrimSpace(value) == "" || strings.ContainsRune(value, 0) {
		return fmt.Errorf("database path must not be empty or contain NUL")
	}
	return nil
}
func (s LocalSettings) Validate() error {
	if err := networkprefs.ValidateHost(s.Host); err != nil {
		return err
	}
	if s.Host == "" || s.Port < 0 || s.Port > 65535 {
		return fmt.Errorf("invalid local host/port")
	}
	for _, path := range []string{s.CollectorDBPath, s.ServerDBPath} {
		if err := validatePath(path); err != nil {
			return err
		}
	}
	if s.AppDBPath != "" {
		return validatePath(s.AppDBPath)
	}
	return nil
}
func (s LocalSettings) ApplicationPath() string {
	if s.AppDBPath != "" {
		return s.AppDBPath
	}
	path, err := dbpath.Canonical(s.ServerDBPath)
	if err != nil {
		path = s.ServerDBPath
	}
	return filepath.Join(filepath.Dir(path), "app.sqlite")
}
func (s SyncSettings) ValidateDestination() error {
	if s.ServerURL == "" || s.ServerToken == "" {
		return fmt.Errorf("sync requires distributed.server-url and distributed.server-token")
	}
	if err := ValidateURL(s.ServerURL); err != nil {
		return err
	}
	if err := validateToken(s.ServerToken); err != nil {
		return err
	}
	return nil
}
func (s SyncSettings) Validate() error {
	if err := s.ValidateDestination(); err != nil {
		return err
	}
	return validatePath(s.CollectorDBPath)
}
func (s BrowseSettings) Validate() error {
	if s.ServerURL == "" {
		return fmt.Errorf("browse requires distributed.server-url")
	}
	return ValidateURL(s.ServerURL)
}
func resolveString(fallback string, saved *string, key string, override *string, environment bool) string {
	if override != nil {
		return *override
	}
	if environment {
		if value, ok := os.LookupEnv(key); ok {
			return value
		}
	}
	if saved != nil {
		return *saved
	}
	return fallback
}
func resolvePath(fallback string, saved *string, key string, override *string, base string, environment bool) string {
	if override != nil {
		return *override
	}
	if environment {
		if value := os.Getenv(key); value != "" {
			return value
		}
	}
	if saved != nil {
		if *saved == "" {
			return ""
		}
		if filepath.IsAbs(*saved) {
			return *saved
		}
		return filepath.Join(base, *saved)
	}
	return fallback
}
func (v Values) local(base string, environment bool, o Overrides) (LocalSettings, error) {
	s := Defaults()
	s.CollectorDBPath = resolvePath(s.CollectorDBPath, v.Collector.DBPath, "TOKENINSIGHTS_COLLECTOR_DB_PATH", o.CollectorDBPath, base, environment)
	s.ServerDBPath = resolvePath(s.ServerDBPath, v.InProcess.ServerDBPath, "TOKENINSIGHTS_SERVER_DB_PATH", o.ServerDBPath, base, environment)
	s.AppDBPath = resolvePath("", v.InProcess.AppDBPath, "TOKENINSIGHTS_APP_DB_PATH", o.AppDBPath, base, environment)
	s.Host = resolveString(s.Host, v.InProcess.Host, "TOKENINSIGHTS_HOST", o.Host, environment)
	if o.Port != nil {
		s.Port = *o.Port
	} else if environment && os.Getenv("TOKENINSIGHTS_PORT") != "" {
		var err error
		s.Port, err = strconv.Atoi(os.Getenv("TOKENINSIGHTS_PORT"))
		if err != nil {
			return s, fmt.Errorf("invalid TOKENINSIGHTS_PORT")
		}
	} else if v.InProcess.Port != nil {
		s.Port = *v.InProcess.Port
	}
	return s, s.Validate()
}
func (v Values) remote(base string, environment bool, o Overrides) SyncSettings {
	return SyncSettings{
		CollectorDBPath: resolvePath(Defaults().CollectorDBPath, v.Collector.DBPath, "TOKENINSIGHTS_COLLECTOR_DB_PATH", o.CollectorDBPath, base, environment),
		ServerURL:       resolveString("", v.Distributed.ServerURL, "TOKENINSIGHTS_SERVER_URL", o.ServerURL, environment),
		ServerToken:     resolveString("", v.Distributed.ServerToken, "TOKENINSIGHTS_ACCESS_TOKEN", o.ServerToken, environment)}
}
func ResolveLocal(path string, environment bool, o Overrides) (LocalSettings, error) {
	v, err := Read(path)
	if err != nil {
		return LocalSettings{}, err
	}
	return v.local(filepath.Dir(path), environment, o)
}

// Sync completeness is checked after flags: status and dry-run do not submit.
func ResolveSync(path string, environment bool, o Overrides) (SyncSettings, error) {
	v, err := Read(path)
	if err != nil {
		return SyncSettings{}, err
	}
	settings := v.remote(filepath.Dir(path), environment, o)
	return settings, validatePath(settings.CollectorDBPath)
}
func ResolveBrowse(path string, environment bool, o Overrides) (BrowseSettings, error) {
	v, err := Read(path)
	if err != nil {
		return BrowseSettings{}, err
	}
	return BrowseSettings{ServerURL: resolveString("", v.Distributed.ServerURL, "TOKENINSIGHTS_SERVER_URL", o.ServerURL, environment)}, nil
}
func (v Values) Get(key, base string) (string, error) {
	local, err := v.local(base, false, Overrides{})
	remote := v.remote(base, false, Overrides{})
	switch key {
	case "collector.db-path":
		return remote.CollectorDBPath, nil
	case "in-process.app-db-path":
		return local.ApplicationPath(), err
	case "in-process.server-db-path":
		return local.ServerDBPath, err
	case "in-process.host":
		return local.Host, err
	case "in-process.port":
		return strconv.Itoa(local.Port), err
	case "distributed.server-url":
		return remote.ServerURL, nil
	case "distributed.server-token":
		if remote.ServerToken == "" {
			return "unset", nil
		}
		return "configured", nil
	default:
		return "", fmt.Errorf("unknown config key %q", key)
	}
}
func (v *Values) Set(key, value string, remove bool) error {
	var stringValue *string
	if !remove {
		stringValue = &value
	}
	switch key {
	case "collector.db-path", "in-process.server-db-path", "in-process.app-db-path":
		if !remove {
			if err := validatePath(value); err != nil {
				return err
			}
			absolute, err := filepath.Abs(value)
			if err != nil {
				return err
			}
			value = absolute
		}
		switch key {
		case "collector.db-path":
			v.Collector.DBPath = stringValue
		case "in-process.server-db-path":
			v.InProcess.ServerDBPath = stringValue
		default:
			v.InProcess.AppDBPath = stringValue
		}
	case "in-process.host":
		if !remove {
			if value == "" {
				return fmt.Errorf("empty host")
			}
			if err := networkprefs.ValidateHost(value); err != nil {
				return err
			}
		}
		v.InProcess.Host = stringValue
	case "in-process.port":
		v.InProcess.Port = nil
		if !remove {
			port, err := strconv.Atoi(value)
			if err != nil || port < 0 || port > 65535 {
				return fmt.Errorf("port must be between 0 and 65535")
			}
			v.InProcess.Port = &port
		}
	case "distributed.server-url":
		if err := ValidateURL(value); err != nil {
			return err
		}
		v.Distributed.ServerURL = stringValue
	case "distributed.server-token":
		if !remove {
			if value == "" {
				return fmt.Errorf("empty server-token")
			}
			if err := validateToken(value); err != nil {
				return err
			}
		}
		v.Distributed.ServerToken = stringValue
	default:
		return fmt.Errorf("unknown config key %q", key)
	}
	return nil
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
