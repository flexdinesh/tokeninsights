package server

import (
	"os"
	"os/exec"
	"runtime"
)

func browserCommand(platform string, getenv func(string) string, url string) []string {
	for _, key := range []string{"SSH_CONNECTION", "SSH_CLIENT", "SSH_TTY"} {
		if getenv(key) != "" {
			return nil
		}
	}
	switch platform {
	case "darwin":
		return []string{"open", url}
	case "windows":
		return []string{"rundll32", "url.dll,FileProtocolHandler", url}
	case "linux":
		if getenv("DISPLAY") != "" || getenv("WAYLAND_DISPLAY") != "" {
			return []string{"xdg-open", url}
		}
	}
	return nil
}

func openBrowser(url string) error {
	command := browserCommand(runtime.GOOS, os.Getenv, url)
	if len(command) == 0 {
		return nil
	}
	opener := exec.Command(command[0], command[1:]...)
	if err := opener.Start(); err != nil {
		return err
	}
	// Some launchers wait for the browser to close. Reap them without blocking serving.
	go func() { _ = opener.Wait() }()
	return nil
}
