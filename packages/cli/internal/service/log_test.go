package service

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestLongLivedLogRotationKeepsBoundedRecentHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "service.log")
	log, err := openLog(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = log.Close() }()
	for _, letter := range []byte("abcde") {
		if _, err := log.Write(bytes.Repeat([]byte{letter}, logLimit)); err != nil {
			t.Fatal(err)
		}
	}
	for i, suffix := range []string{"", ".1", ".2", ".3"} {
		data, err := os.ReadFile(path + suffix)
		if err != nil || len(data) != logLimit || data[0] != byte('e'-i) || data[len(data)-1] != byte('e'-i) {
			t.Fatal("rotated history incorrect", suffix, len(data), err)
		}
	}
	if _, err := os.Stat(path + ".4"); !os.IsNotExist(err) {
		t.Fatal("log retained excess backup", err)
	}
}
