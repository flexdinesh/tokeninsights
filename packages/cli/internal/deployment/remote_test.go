package deployment_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/accounts"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/adapters/postgres"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/adapters/sqlite"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/datastore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/persistence/postgres/testdb"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/storagecontract"
)

// Only hosted composition varies its backend. Local ownership tests keep SQLite.
func runRemoteBackends(t *testing.T, scenario func(*testing.T, *remoteServer)) {
	t.Helper()
	for _, backend := range []string{"sqlite", "postgres"} {
		t.Run(backend, func(t *testing.T) { scenario(t, newRemote(t, backend)) })
	}
}

type remoteServer struct {
	root, backend, path, dsn, socket, target string
	user                                     accounts.User
	token                                    accounts.Token
	stop                                     func()
}

func newRemote(t *testing.T, backend string) *remoteServer {
	t.Helper()
	// Keep operator socket paths below platform limits, even for long test names.
	root, err := os.MkdirTemp("", "ti-server-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	r := &remoteServer{root: root, backend: backend}
	r.path = filepath.Join(r.root, "server.sqlite")
	r.socket = filepath.Join(r.root, "admin.sock")
	if backend == "postgres" {
		r.path = ""
		r.dsn = testdb.New(t)
	}
	r.start(t)
	r.admin(t, accounts.AdminRequest{Operation: "create-user", DisplayName: "fixture"}, &r.user)
	r.admin(t, accounts.AdminRequest{Operation: "create-token", UserID: r.user.UserID, Permissions: []string{accounts.Read, accounts.Ingest}}, &r.token)
	remoteTokens.Store(r.target, r.token.Secret)
	return r
}

func (r *remoteServer) admin(t *testing.T, request accounts.AdminRequest, result interface{}) {
	t.Helper()
	var output bytes.Buffer
	if err := accounts.AdminCall(t.Context(), r.socket, request, &output); err != nil {
		t.Fatal(err)
	}
	if result != nil {
		if err := json.Unmarshal(output.Bytes(), result); err != nil {
			t.Fatal(err)
		}
	}
}

func (r *remoteServer) start(t *testing.T) {
	t.Helper()
	listen := "0.0.0.0:0"
	if r.target != "" {
		listen = strings.TrimPrefix(r.target, "http://")
	}
	command := exec.Command(serverBinary)
	command.Env = append(isolatedEnvironment(r.root),
		"TOKENINSIGHTS_PUBLIC_URL=https://usage.example", "TOKENINSIGHTS_LISTEN="+listen,
		"TOKENINSIGHTS_STORAGE_BACKEND="+r.backend, "TOKENINSIGHTS_ADMIN_SOCKET="+r.socket)
	if r.backend == "sqlite" {
		command.Env = append(command.Env, "TOKENINSIGHTS_SERVER_DB_PATH="+r.path)
	} else {
		secret := filepath.Join(r.root, "postgres-dsn")
		if err := os.WriteFile(secret, []byte(r.dsn), 0o600); err != nil {
			t.Fatal(err)
		}
		command.Env = append(command.Env, "TOKENINSIGHTS_POSTGRES_DSN_FILE="+secret)
	}
	output, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	log, err := os.CreateTemp(r.root, "server-*.log")
	if err != nil {
		t.Fatal(err)
	}
	command.Stderr = log
	if err := command.Start(); err != nil {
		_ = log.Close()
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	var once sync.Once
	var target string
	r.stop = func() {
		once.Do(func() {
			_ = command.Process.Signal(syscall.SIGTERM)
			select {
			case err := <-done:
				if err != nil {
					t.Error(err)
				}
			case <-time.After(20 * time.Second):
				_ = command.Process.Kill()
				<-done
				t.Error("server shutdown timeout")
			}
			_ = log.Close()
			remoteTokens.Delete(target)
		})
	}
	t.Cleanup(r.stop)
	ready := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(output)
		if scanner.Scan() {
			ready <- scanner.Text()
		}
	}()
	select {
	case target = <-ready:
	case <-time.After(10 * time.Second):
		body, _ := os.ReadFile(log.Name())
		t.Fatalf("server readiness: %s", body)
	}
	if !strings.HasPrefix(target, "http://127.0.0.1:") {
		t.Fatal("unusable wildcard URL", target)
	}
	for _, name := range []string{"config", "state"} {
		if _, err := os.Stat(filepath.Join(r.root, name)); !os.IsNotExist(err) {
			t.Fatal("remote created client state", name, err)
		}
	}
	r.target = target
	if r.token.Secret != "" {
		remoteTokens.Store(target, r.token.Secret)
	}
}

// Seed durable work with the real adapter while the binary is stopped. No worker
// runs here: restart coverage must not depend on winning a race with processing.
func (r *remoteServer) offlineDataset(t *testing.T, seed func(storagecontract.Dataset)) {
	t.Helper()
	r.stop()
	var store *datastore.Store
	var closeStore func() error
	if r.backend == "postgres" {
		s, err := postgres.Open(t.Context(), r.dsn)
		if err != nil {
			t.Fatal(err)
		}
		store, closeStore = s.Tokens, s.Owner.Close
	} else {
		s, err := sqlite.Open(t.Context(), r.path, datastore.Options{Kind: datastore.KindHosted})
		if err != nil {
			t.Fatal(err)
		}
		store, closeStore = s, s.Close
	}
	defer func() {
		if err := closeStore(); err != nil {
			t.Error(err)
		}
	}()
	d := store.ForDataset(r.user.DatasetID)
	seed(storagecontract.Dataset{Receiver: d, Processing: d, Reprocess: d.Reprocess})
}
