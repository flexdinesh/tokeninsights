package localruntime

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/appstore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collector"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/datastore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/pipeline"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/rawcollectorstore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverownership"
)

// PrepareFixture resets only the two compatible development databases. Keep
// database/lock inodes, WAL sidecars and unrelated files, including old databases.
func PrepareFixture(ctx context.Context, collectorPath, serverPath string) error {
	collectorPath, _, err := serverownership.Identify(collectorPath)
	if err != nil {
		return err
	}
	serverPath, _, err = serverownership.Identify(serverPath)
	if err != nil {
		return err
	}
	root := filepath.Dir(collectorPath)
	if filepath.Base(root) != ".tokeninsights-dev" || filepath.Base(collectorPath) != "collector.sqlite" || filepath.Base(serverPath) != "server.sqlite" || filepath.Dir(serverPath) != root {
		return fmt.Errorf("uncontrolled development databases")
	}
	release, err := db.AcquireWriterLock(ctx, serverPath+".service.op")
	if err != nil {
		return err
	}
	defer release()
	owner, held, err := serverownership.Lifetime(serverPath, true)
	if err != nil {
		return err
	}
	if held {
		return ErrOwned
	}
	defer func() { _ = owner.Close() }()
	// Check both roles before resetting either file. Legacy databases stay intact.
	if err := inspectFixtureRoles(collectorPath, serverPath); err != nil {
		return err
	}
	writer, err := db.AcquireWriterLock(ctx, collectorPath)
	if err != nil {
		return err
	}
	defer writer()
	serverWriter, err := db.AcquireWriterLock(ctx, serverPath)
	if err != nil {
		return err
	}
	defer serverWriter()
	if err := inspectFixtureRoles(collectorPath, serverPath); err != nil {
		return err
	}
	server, err := datastore.Open(ctx, serverPath)
	if err != nil {
		return err
	}
	defer func() { _ = server.Close() }()
	var application *appstore.Store
	appPath := filepath.Join(root, "app.sqlite")
	if _, err := os.Stat(appPath); err == nil {
		identity, err := server.DatabaseIdentity(ctx)
		if err != nil {
			return err
		}
		application, err = appstore.Open(ctx, appPath, identity, datastore.KindPersonal)
		if err != nil {
			return err
		}
		defer func() { _ = application.Close() }()
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := db.ResetAllLocked(ctx, collectorPath); err != nil {
		return err
	}
	capture, err := rawcollectorstore.Open(ctx, collectorPath)
	if err != nil {
		return err
	}
	if err := capture.Close(); err != nil {
		return err
	}
	tx, err := server.SQL().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, table := range []string{"analytics_facts", "analytics_estimates", "analytics_provenance", "processing_outcomes", "processing_dependencies", "processing_scopes", "ingestion_batch_items", "ingestion_items", "ingestion_batches", "raw_evidence", "analytics_generations"} {
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+table); err != nil {
			return err
		}
	}
	now := time.Now().UnixMilli()
	var identity [16]byte
	if _, err := rand.Read(identity[:]); err != nil {
		return err
	}
	databaseID := hex.EncodeToString(identity[:])
	if _, err := tx.ExecContext(ctx, `UPDATE ingestion_instance SET database_id=?,created_at_ms=? WHERE id=1`, databaseID, now); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE ingestion_metadata SET database_id=?,generation=1,target_generation=1,input_revision=0,revision=0,last_ingestion_at_ms=0,created_at_ms=? WHERE dataset_id=?`, databaseID, now, datastore.DatasetID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO analytics_generations(dataset_id,generation,processor_version,state,created_at_ms,activated_at_ms) VALUES(?,1,?,'active',?,?)`, datastore.DatasetID, evidence.ProcessorVersion, now, now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if application != nil {
		if _, err := application.SQL().ExecContext(ctx, "UPDATE application_metadata SET database_id=? WHERE id=1", databaseID); err != nil {
			return err
		}
	}
	for _, name := range []string{"source", "home"} {
		if err := os.RemoveAll(filepath.Join(root, name)); err != nil {
			return err
		}
	}
	return nil
}

func inspectFixtureRoles(collectorPath, serverPath string) error {
	if _, err := os.Stat(collectorPath); err == nil {
		collector, err := db.OpenWritable(collectorPath)
		if err != nil {
			return err
		}
		if err := collector.Close(); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if _, err := os.Stat(serverPath); err == nil {
		if err := datastore.InspectKind(context.Background(), serverPath, datastore.KindPersonal); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

const fixtureVisibilityTimeout = 30 * time.Second

// CaptureFixture populates only the controlled development databases.
func CaptureFixture(ctx context.Context, collectorPath, dataPath, source string) error {
	root := filepath.Dir(collectorPath)
	if filepath.Base(root) != ".tokeninsights-dev" || filepath.Base(collectorPath) != "collector.sqlite" ||
		filepath.Base(dataPath) != "server.sqlite" || filepath.Dir(dataPath) != root || source != filepath.Join(root, "source") {
		return fmt.Errorf("uncontrolled development capture")
	}
	local, err := Open(ctx, collectorPath, dataPath)
	if err != nil {
		return err
	}
	defer func() { _ = local.Close() }()
	_, err = collector.Run(ctx, collector.Options{CollectorDBPath: collectorPath, ServerDBPath: dataPath, Destination: local.Destination,
		SyncOptions: pipeline.SyncOptions{Harnesses: pipeline.SupportedHarnesses, SourceDir: source, Now: time.Now()}})
	if err != nil {
		return err
	}
	visible, cancel := context.WithTimeout(ctx, fixtureVisibilityTimeout)
	defer cancel()
	return local.WaitVisible(visible)
}
