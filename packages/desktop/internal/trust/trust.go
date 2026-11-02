// Package trust remembers dashboard origins the user approved.
package trust

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
)

const (
	dirMode  fs.FileMode = 0o700
	fileMode fs.FileMode = 0o600
)

// Store persists approved origins in one private JSON file.
// It is not safe for concurrent use; the viewer calls it from the main thread.
type Store struct{ path string }

type document struct {
	Origins []string `json:"origins"`
}

// New returns a store backed by path. Nothing is read until first use.
func New(path string) Store { return Store{path: path} }

// Trusted reports whether origin was approved. A missing file has no origins.
func (s Store) Trusted(origin string) (bool, error) {
	doc, err := s.load()
	if err != nil {
		return false, err
	}
	return slices.Contains(doc.Origins, origin), nil
}

// Trust records origin. An unreadable or corrupt file is reported, never overwritten.
func (s Store) Trust(origin string) error {
	doc, err := s.load()
	if err != nil {
		return err
	}
	if slices.Contains(doc.Origins, origin) {
		return nil
	}
	doc.Origins = append(doc.Origins, origin)
	slices.Sort(doc.Origins)
	return s.save(doc)
}

func (s Store) load() (document, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return document{}, nil
	}
	if err != nil {
		return document{}, fmt.Errorf("read trusted origins: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var doc document
	if err := decoder.Decode(&doc); err != nil {
		return document{}, fmt.Errorf("read trusted origins %s: %w", s.path, err)
	}
	if decoder.More() {
		return document{}, fmt.Errorf("read trusted origins %s: trailing data", s.path)
	}
	return doc, nil
}

func (s Store) save(doc document) (err error) {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return fmt.Errorf("save trusted origins: %w", err)
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".trusted-origins-*")
	if err != nil {
		return fmt.Errorf("save trusted origins: %w", err)
	}
	defer func() {
		if err != nil {
			_ = os.Remove(tmp.Name())
		}
	}()
	if err := tmp.Chmod(fileMode); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("save trusted origins: %w", err)
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("save trusted origins: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("save trusted origins: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("save trusted origins: %w", err)
	}
	if err := os.Rename(tmp.Name(), s.path); err != nil {
		return fmt.Errorf("save trusted origins: %w", err)
	}
	return nil
}
