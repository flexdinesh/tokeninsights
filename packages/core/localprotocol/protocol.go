// Package localprotocol defines the private local backend contract and filesystem
// identity. It has no storage, runtime, server or generated asset dependencies.
package localprotocol

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/flexdinesh/tokeninsights/packages/core/analytics"
	"github.com/flexdinesh/tokeninsights/packages/core/dbpath"
	"github.com/flexdinesh/tokeninsights/packages/core/networkprefs"
	"github.com/flexdinesh/tokeninsights/packages/core/serverownership"
)

const ProtocolVersion = 1

const MaxQueryRows = 100000

// Private queries stream JSON directly into the decoder, avoiding a duplicate
// response buffer. This separate budget supports complete 100k-row snapshots,
// including responses beyond the public API's 16 MiB per-page budget.
const MaxQueryResponseBytes = 512 << 20
const MaxQueryRequestBytes = 1 << 20

type AllUsageRequest struct {
	Query   analytics.Query
	MaxRows int
}

var ErrOwned = errors.New("database already owned; close the process using this database")
var ErrProcessingFailed = errors.New("processing_failed: saved usage needs attention; retry or run tokeninsights data reprocess")
var ErrProcessingTimeout = fmt.Errorf("processing_timeout: usage is still processing; retry or view saved data: %w", context.DeadlineExceeded)

var ErrSettingsMismatch = errors.New("local backend settings differ; close its viewers before changing local settings")
var ErrIncompatible = errors.New("local backend incompatible; close its viewers and launch matching clients")
var ErrCaptureBusy = errors.New("local backend capture busy with another source directory")

type Settings struct {
	CollectorDBPath, ServerDBPath, AppDBPath, Host string
	Port                                           int
}

// Canonical resolves filesystem aliases without inspecting an active owner's
// store inodes. Collector creation briefly publishes through a hard link.
func (s Settings) Canonical() (Settings, error) {
	if err := networkprefs.ValidateHost(s.Host); err != nil {
		return Settings{}, err
	}
	if s.Host == "" || s.Port < 0 || s.Port > 65535 {
		return Settings{}, errors.New("local backend requires IPv4 host and a valid port")
	}
	paths := []*string{&s.CollectorDBPath, &s.ServerDBPath, &s.AppDBPath}
	for _, path := range paths {
		canonical, err := dbpath.Canonical(*path)
		if err != nil {
			return Settings{}, err
		}
		*path = canonical
	}
	for i, first := range paths {
		for _, second := range paths[i+1:] {
			if err := dbpath.ValidateDistinct(*first, *second); err != nil {
				return Settings{}, err
			}
		}
	}
	return s, nil
}

// ValidateStorageIdentity runs only before an unowned backend opens storage.
// Persistent hard links reject before mutation; active-owner attachment never
// mistakes a temporary atomic-publication link for user-created storage aliases.
func (s Settings) ValidateStorageIdentity() error {
	for _, path := range []string{s.CollectorDBPath, s.ServerDBPath, s.AppDBPath} {
		if _, _, err := serverownership.Identify(path); err != nil {
			return err
		}
	}
	return nil
}

// SocketPath uses a short, user-owned directory rather than database paths,
// which can exceed the platform Unix socket path limit. Never follow a symlink
// or repair permissions on an existing directory with unknown ownership.
func SocketPath(s Settings) (string, error) {
	_, identity, err := serverownership.Identify(s.ServerDBPath)
	if err != nil {
		return "", err
	}
	dir, err := RuntimeDirectory()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, identity[:24]+".sock"), nil
}

func RuntimeDirectory() (string, error) {
	// Supported platforms share /tmp across GUI and shell environments. TMPDIR
	// can differ between launchers and can exceed Unix socket path limits.
	dir := filepath.Join("/tmp", fmt.Sprintf("ti-%d", os.Getuid()))
	if err := os.Mkdir(dir, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return "", err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return "", err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.IsDir() || info.Mode().Perm() != 0o700 || int(stat.Uid) != os.Getuid() {
		return "", errors.New("local backend runtime directory is not private and owned")
	}
	return dir, nil
}

type Descriptor struct {
	ProtocolVersion int      `json:"protocolVersion"`
	InstanceID      string   `json:"instanceId"`
	URL             string   `json:"url"`
	Settings        Settings `json:"settings"`
}

func (d Descriptor) Compatible(s Settings) error {
	if d.ProtocolVersion != ProtocolVersion || d.InstanceID == "" {
		return ErrIncompatible
	}
	if d.Settings != s {
		return ErrSettingsMismatch
	}
	if _, err := HTTPOrigin(d.URL, false); err != nil {
		return ErrIncompatible
	}
	return nil
}

// HTTPOrigin accepts only an HTTP IPv4 origin with an explicit dialable port.
// Endpoint attachment additionally requires loopback, excluding hosted URLs.
func HTTPOrigin(value string, loopback bool) (string, error) {
	u, err := url.Parse(value)
	if err != nil || u == nil || u.Scheme != "http" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || (u.Path != "" && u.Path != "/") || u.Opaque != "" {
		return "", ErrIncompatible
	}
	host := u.Hostname()
	if host == "localhost" {
		host = "127.0.0.1"
	}
	address, err := netip.ParseAddr(host)
	if err != nil || !address.Is4() || address.IsUnspecified() || loopback && !address.IsLoopback() {
		return "", ErrIncompatible
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil || port < 1 || port > 65535 {
		return "", ErrIncompatible
	}
	u.Host = net.JoinHostPort(host, strconv.Itoa(port))
	u.Path, u.RawPath = "", ""
	return u.String(), nil
}

type CaptureRequest struct {
	FullRefresh bool   `json:"fullRefresh"`
	SourceDir   string `json:"sourceDir"`
}

// CollectionResult contains sanitized completion measurements. Acceptance and
// publication visibility remain distinct; call WaitVisible separately.
type CollectionResult struct {
	AttemptID    string `json:"attemptId"`
	Accepted     int64  `json:"accepted"`
	Batches      int64  `json:"batches"`
	Pending      int64  `json:"pending"`
	PendingKnown bool   `json:"pendingKnown"`
	Quarantined  int    `json:"quarantined"`
	Failed       int    `json:"failed"`
	RawFacts     int    `json:"rawFacts"`
	Skipped      int    `json:"skipped"`
	ErrorCode    string `json:"errorCode,omitempty"`
}
