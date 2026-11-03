package pipeline

import (
	"os"
	"path/filepath"
	"strings"
)

func configuredRoot(environment, home string, suffix ...string) string {
	if root := strings.TrimSpace(os.Getenv(environment)); root != "" {
		return root
	}
	return rootChild(home, suffix...)
}

func rootChild(root string, suffix ...string) string {
	if root == "" {
		return ""
	}
	return filepath.Join(append([]string{root}, suffix...)...)
}
