package pipeline

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const opencodeSQLiteSourceKind = "opencode-sqlite"

type opencodeSQLiteAdapter struct{}

type openCodeReader interface {
	QueryContext(context.Context, string, ...interface{}) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...interface{}) *sql.Row
}

func (a opencodeSQLiteAdapter) Harness() Harness {
	return HarnessOpenCode
}

func (a opencodeSQLiteAdapter) Discover(ctx context.Context, options DiscoverOptions) ([]Source, error) {
	roots, err := a.discoveryRoots(options)
	if err != nil {
		return nil, err
	}

	var sources []Source
	for _, root := range roots {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		info, err := os.Stat(root)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		if !info.IsDir() {
			if isOpenCodeSQLiteDB(root) {
				source := a.source(root, filepath.Dir(root))
				if options.Sources != nil {
					source = a.source(options.Sources.identityPath(root), filepath.Dir(options.Sources.identityPath(root)))
					source.Path = root
				}
				sources = append(sources, source)
			}
			continue
		}
		entries, err := os.ReadDir(root)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if entry.IsDir() {
				continue
			}
			path := filepath.Join(root, entry.Name())
			if isOpenCodeSQLiteDB(path) {
				source := a.source(path, root)
				if options.Sources != nil {
					source = a.source(options.Sources.identityPath(path), options.Sources.identityPath(root))
					source.Path = path
				}
				sources = append(sources, source)
			}
		}
	}
	sort.Slice(sources, func(i int, j int) bool {
		return sources[i].Path < sources[j].Path
	})
	return sources, nil
}

func (a opencodeSQLiteAdapter) discoveryRoots(options DiscoverOptions) ([]string, error) {
	if options.Sources != nil {
		return options.Sources.discoveryRoots(HarnessOpenCode, options.HarnessSubdirOnly)
	}
	sourceDir := strings.TrimSpace(options.SourceDir)
	if sourceDir != "" {
		harnessDir := filepath.Join(sourceDir, string(HarnessOpenCode))
		if info, err := os.Stat(harnessDir); err == nil && info.IsDir() {
			return []string{harnessDir}, nil
		} else if options.HarnessSubdirOnly {
			if err != nil && !os.IsNotExist(err) {
				return nil, err
			}
			return nil, nil
		}
		return []string{sourceDir}, nil
	}

	dataHome := strings.TrimSpace(os.Getenv("XDG_DATA_HOME"))
	if dataHome != "" {
		return []string{filepath.Join(dataHome, "opencode")}, nil
	}
	home := strings.TrimSpace(os.Getenv("HOME"))
	if home == "" {
		return nil, nil
	}
	return []string{filepath.Join(home, ".local/share/opencode")}, nil
}

func (a opencodeSQLiteAdapter) source(path string, root string) Source {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		rel = filepath.Base(path)
	}
	return Source{
		Harness:     HarnessOpenCode,
		ID:          stableHash("opencode-sqlite-db:" + filepath.ToSlash(rel) + ":" + stableHash(path)),
		Kind:        opencodeSQLiteSourceKind,
		Path:        path,
		RawSourceID: stableHash("opencode-sqlite-root:" + stableHash(root)),
	}
}

type openCodeSessionLocation struct {
	directory string
	projectID string
}

func attachOpenCodeLocations(ctx context.Context, database openCodeReader, options SyncOptions, facts []sessionLocation) error {
	if len(facts) == 0 {
		return nil
	}
	if err := requireSQLiteColumns(ctx, database, "session", []string{"id", "directory", "project_id"}); err != nil {
		return nil
	}
	gitProjects := map[string]bool{}
	if err := requireSQLiteColumns(ctx, database, "project", []string{"id", "vcs"}); err == nil {
		rows, err := database.QueryContext(ctx, "SELECT id FROM project WHERE vcs = 'git'")
		if err != nil {
			return err
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				_ = rows.Close()
				return err
			}
			gitProjects[id] = true
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return err
		}
		_ = rows.Close()
	}
	rows, err := database.QueryContext(ctx, "SELECT id, directory, project_id FROM session")
	if err != nil {
		return err
	}
	sessions := map[string]openCodeSessionLocation{}
	for rows.Next() {
		var id string
		var directory, projectID sql.NullString
		if err := rows.Scan(&id, &directory, &projectID); err != nil {
			_ = rows.Close()
			return err
		}
		project := ""
		if gitProjects[projectID.String] {
			project = projectID.String
		}
		sessions[id] = openCodeSessionLocation{directory: directory.String, projectID: project}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	_ = rows.Close()
	for index := range facts {
		if facts[index].SessionID == nil {
			continue
		}
		session := sessions[*facts[index].SessionID]
		facts[index].Location, _ = resolveFactLocation(ctx, options, session.directory, "", session.projectID)
	}
	return nil
}

func isOpenCodeSQLiteDB(path string) bool {
	name := filepath.Base(path)
	if name == "opencode.db" {
		return true
	}
	if !strings.HasPrefix(name, "opencode-") || !strings.HasSuffix(name, ".db") {
		return false
	}
	channel := strings.TrimSuffix(strings.TrimPrefix(name, "opencode-"), ".db")
	if channel == "" {
		return false
	}
	for _, r := range channel {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}

func openReadOnlySQLite(path string) (*sql.DB, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	fileURL := url.URL{Scheme: "file", Path: absPath}
	query := fileURL.Query()
	query.Set("mode", "ro")
	query.Add("_pragma", "query_only(true)")
	query.Add("_pragma", "busy_timeout(5000)")
	fileURL.RawQuery = query.Encode()
	return sql.Open("sqlite", fileURL.String())
}

func sqliteTableExists(ctx context.Context, database openCodeReader, table string) (bool, error) {
	var name string
	err := database.QueryRowContext(ctx, `
		SELECT name
		FROM sqlite_master
		WHERE type = 'table' AND name = ?
	`, table).Scan(&name)
	if err == nil {
		return true, nil
	}
	if err == sql.ErrNoRows {
		return false, nil
	}
	return false, err
}

func requireSQLiteColumns(ctx context.Context, database openCodeReader, table string, columns []string) error {
	rows, err := database.QueryContext(ctx, "PRAGMA table_info("+table+")")
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	found := map[string]bool{}
	for rows.Next() {
		var cid int
		var name string
		var columnType string
		var notNull int
		var defaultValue sql.NullString
		var primaryKey int
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return err
		}
		found[name] = true
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, column := range columns {
		if !found[column] {
			return fmt.Errorf("missing column %s", column)
		}
	}
	return nil
}
