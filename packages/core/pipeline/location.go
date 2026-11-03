package pipeline

import (
	"context"
	"net/url"
	"os"
	"os/exec"
	pathpkg "path"
	"path/filepath"
	"strings"
	"sync"
)

const locationLabelLimit = 48

// Location contains hashed identities and display labels. Directory labels
// may contain a sanitized path; remote URLs are not persisted.
type Location struct {
	DirectoryKey, DirectoryName   string
	RepositoryKey, RepositoryName string
	RepositorySource              string
}

type gitLocation struct {
	root, commonDir, remote string
}

type locationResolver struct {
	mu  sync.Mutex
	git map[string]*gitLocationCall
}

type gitLocationCall struct {
	done     chan struct{}
	location gitLocation
}

func resolveFactLocation(ctx context.Context, options SyncOptions, directory, recordedRemote, projectID string) (*Location, bool) {
	path, displayPath := recordedDirectoryPath(directory)
	location := &Location{}
	if path != "" {
		location.DirectoryKey = stableHash("directory:" + path)
		home := ""
		if options.Sources != nil {
			home = options.Sources.Home
		}
		location.DirectoryName = sanitizedLocationPathWithHome(displayPath, home)
	}
	resolver := options.locationResolver
	if resolver == nil {
		resolver = &locationResolver{}
	}
	git := resolver.inspect(ctx, path)
	recordedIdentity, recordedName := remoteIdentity(recordedRemote)
	gitIdentity, gitName := remoteIdentity(git.remote, git.root)
	conflict := recordedIdentity != "" && gitIdentity != "" && recordedIdentity != gitIdentity
	if recordedIdentity != "" {
		location.RepositoryKey = stableHash("remote:" + recordedIdentity)
		location.RepositoryName = locationLabel(recordedName)
		location.RepositorySource = "harness"
	} else if gitIdentity != "" {
		location.RepositoryKey = stableHash("remote:" + gitIdentity)
		location.RepositoryName = locationLabel(gitName)
		location.RepositorySource = "git-remote"
	} else if git.commonDir != "" {
		location.RepositoryKey = stableHash("git-common-dir:" + git.commonDir)
		location.RepositoryName = locationLabel(filepath.Base(filepath.Dir(git.commonDir)))
		location.RepositorySource = "git-common-dir"
	} else if projectID != "" {
		location.RepositoryKey = stableHash("opencode-project:" + projectID)
		location.RepositoryName = locationLabel(filepath.Base(path))
		location.RepositorySource = "opencode-project"
	}
	if location.DirectoryKey == "" && location.RepositoryKey == "" {
		return nil, conflict
	}
	return location, conflict
}

func (resolver *locationResolver) inspect(ctx context.Context, path string) gitLocation {
	if path == "" {
		return gitLocation{}
	}
	resolver.mu.Lock()
	if cached, ok := resolver.git[path]; ok {
		resolver.mu.Unlock()
		select {
		case <-cached.done:
			return cached.location
		case <-ctx.Done():
			return gitLocation{}
		}
	}
	if resolver.git == nil {
		resolver.git = make(map[string]*gitLocationCall)
	}
	call := &gitLocationCall{done: make(chan struct{})}
	resolver.git[path] = call
	resolver.mu.Unlock()
	result := gitLocation{}
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		paths := strings.SplitN(gitValue(ctx, path, "rev-parse", "--show-toplevel", "--path-format=absolute", "--git-common-dir"), "\n", 2)
		if len(paths) == 2 && paths[0] != "" {
			result.root, result.commonDir = paths[0], paths[1]
			remotes := map[string]string{}
			for _, line := range strings.Split(gitValue(ctx, path, "config", "--get-regexp", `^remote\..*\.url$`), "\n") {
				key, value, ok := strings.Cut(line, " ")
				if !ok || value == "" {
					continue
				}
				name := strings.TrimSuffix(strings.TrimPrefix(key, "remote."), ".url")
				remotes[name] = value
			}
			result.remote = remotes["origin"]
			if result.remote == "" && len(remotes) == 1 {
				for _, remote := range remotes {
					result.remote = remote
				}
			}
		}
	}
	resolver.mu.Lock()
	call.location = result
	if ctx.Err() != nil {
		delete(resolver.git, path)
	}
	close(call.done)
	resolver.mu.Unlock()
	return result
}

func gitValue(ctx context.Context, path string, args ...string) string {
	command := exec.CommandContext(ctx, "git", append([]string{"-C", path}, args...)...)
	command.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
	output, err := command.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func remoteIdentity(remote string, base ...string) (string, string) {
	remote = strings.TrimSpace(remote)
	if remote == "" {
		return "", ""
	}
	var host, name string
	if filepath.IsAbs(remote) {
		name = strings.TrimSuffix(filepath.Clean(remote), ".git")
		return "local:" + name, filepath.Base(name)
	}
	if len(base) > 0 && base[0] != "" && (strings.HasPrefix(remote, "./") || strings.HasPrefix(remote, "../")) {
		name = strings.TrimSuffix(filepath.Clean(filepath.Join(base[0], remote)), ".git")
		return "local:" + name, filepath.Base(name)
	}
	if parsed, err := url.Parse(remote); err == nil && parsed.Scheme == "file" && filepath.IsAbs(parsed.Path) {
		name = strings.TrimSuffix(filepath.Clean(parsed.Path), ".git")
		return "local:" + name, filepath.Base(name)
	} else if err == nil && parsed.Host != "" {
		host = strings.ToLower(parsed.Hostname())
		if port := parsed.Port(); port != "" && (port != "22" || parsed.Scheme != "ssh") && (port != "443" || parsed.Scheme != "https") && (port != "80" || parsed.Scheme != "http") {
			host += ":" + port
		}
		name = parsed.Path
	} else if before, after, ok := strings.Cut(remote, ":"); ok && !strings.Contains(before, "/") {
		if at := strings.LastIndex(before, "@"); at >= 0 {
			before = before[at+1:]
		}
		host, name = strings.ToLower(before), after
	} else {
		return "", ""
	}
	name = strings.TrimSuffix(strings.Trim(strings.TrimSpace(name), "/"), ".git")
	if host == "" || name == "" {
		return "", ""
	}
	return host + "/" + name, filepath.Base(name)
}

func locationLabel(name string) string {
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == string(filepath.Separator) {
		name = "unknown"
	}
	name = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, name)
	if runes := []rune(name); len(runes) > locationLabelLimit {
		name = string(runes[:locationLabelLimit])
	}
	return name
}

func recordedDirectoryPath(directory string) (string, string) {
	directory = strings.TrimSpace(directory)
	if isWindowsAbsolutePath(directory) {
		path := strings.ReplaceAll(directory, "\\", "/")
		if strings.HasPrefix(path, "//") {
			path = "//" + pathpkg.Clean(strings.TrimPrefix(path, "//"))
		} else {
			path = pathpkg.Clean(path)
		}
		return path, path
	}
	if filepath.IsAbs(directory) {
		path := filepath.Clean(directory)
		return path, filepath.ToSlash(path)
	}
	return "", ""
}

func isWindowsAbsolutePath(value string) bool {
	if len(value) >= 3 && ((value[0] >= 'A' && value[0] <= 'Z') || (value[0] >= 'a' && value[0] <= 'z')) && value[1] == ':' && (value[2] == '/' || value[2] == '\\') {
		return true
	}
	return strings.HasPrefix(value, "\\\\") || strings.HasPrefix(value, "//")
}

func sanitizedLocationPathWithHome(value, home string) string {
	value = strings.ReplaceAll(value, "\\", "/")
	value = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, value)
	if home != "" {
		home = strings.TrimRight(strings.ReplaceAll(home, "\\", "/"), "/")
		if suffix, ok := locationPathSuffix(value, home); ok {
			return "~" + suffix
		}
	}
	for _, prefix := range []string{"/home/", "/Users/", "/var/home/", "/Documents and Settings/"} {
		candidate := value
		if len(candidate) >= 2 && candidate[1] == ':' {
			candidate = candidate[2:]
		}
		if strings.HasPrefix(strings.ToLower(candidate), strings.ToLower(prefix)) {
			rest := candidate[len(prefix):]
			user, suffix, found := strings.Cut(rest, "/")
			if user != "" && user != "." && user != ".." {
				if found {
					return "~/" + suffix
				}
				return "~"
			}
		}
	}
	return value
}

func locationPathSuffix(value, home string) (string, bool) {
	if isWindowsAbsolutePath(value) || isWindowsAbsolutePath(home) {
		if !strings.HasPrefix(strings.ToLower(value), strings.ToLower(home)) {
			return "", false
		}
	} else if !strings.HasPrefix(value, home) {
		return "", false
	}
	suffix := value[len(home):]
	return suffix, suffix == "" || strings.HasPrefix(suffix, "/")
}
