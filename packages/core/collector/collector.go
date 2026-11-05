// Package collector coordinates host-only collection and durable publication.
package collector

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"github.com/flexdinesh/tokeninsights/packages/core/dbpath"
	"github.com/flexdinesh/tokeninsights/packages/core/evidence"
	"github.com/flexdinesh/tokeninsights/packages/core/pipeline"
)

// Destination is resolved by the personal runtime before source capture.
// Identity binds the durable local journal; Transport performs direct acceptance.
type Destination struct {
	Transport  Delivery
	Identity   string
	DatabaseID string
	DatasetID  string
	Local      bool
}

type Options struct {
	AcceptedReceipt  func(evidence.Receipt) error
	Destination      *Destination
	CollectorDBPath  string
	ServerDBPath     string
	PublishOnly      bool
	SyncOptions      pipeline.SyncOptions
	DeliveryProgress func(DeliveryProgress)
}

// DeliveryProgress reports acknowledged work, never estimated upload progress.
type DeliveryProgress struct {
	Accepted     int64
	Batches      int64
	Pending      int64
	PendingKnown bool
}

type Result struct {
	Accepted        int64
	Collection      pipeline.Summary
	Batches         int64
	Pending         int64
	PendingKnown    bool
	CollectionError error
	DeliveryError   error
}

// StageError exposes a fixed stage/code without echoing credentials or URLs.
type StageError struct {
	Stage, Code string
	BatchID     string
	Cause       error
}

func (e *StageError) Error() string {
	value := e.Stage + ": " + e.Code
	if e.BatchID != "" {
		value += " batch=" + e.BatchID
	}
	return value
}
func (e *StageError) Unwrap() error { return e.Cause }
func failure(stage, code string, cause error) error {
	return &StageError{Stage: stage, Code: code, Cause: cause}
}

// ValidatePaths must precede creation or collection.
func ValidatePaths(collectorPath, serverPath string) error {
	err := dbpath.ValidateDistinct(collectorPath, serverPath)
	var configuration *dbpath.ConfigurationError
	if errors.As(err, &configuration) {
		return failure("configuration", configuration.Code, nil)
	}
	return err
}

func Run(ctx context.Context, options Options) (Result, error) {
	var result Result
	if strings.TrimSpace(options.CollectorDBPath) == "" {
		return result, failure("configuration", "missing_database_path", nil)
	}
	if options.Destination != nil {
		if !options.Destination.Local || options.Destination.DatasetID != evidence.PersonalDatasetID {
			return result, failure("configuration", "local_destination_required", nil)
		}
		if err := ValidatePaths(options.CollectorDBPath, options.ServerDBPath); err != nil {
			return result, err
		}
	}
	if !options.SyncOptions.DryRun && (options.Destination == nil || options.Destination.Transport == nil) {
		return result, failure("configuration", "missing_destination", nil)
	}
	if options.Destination != nil {
		destination := *options.Destination
		var err error
		destination.Identity, err = endpoint(destination.Identity)
		if err != nil {
			return result, err
		}
		if destination.DatabaseID == "" || !evidence.ValidDatasetID(destination.DatasetID) {
			return result, failure("configuration", "invalid_destination", nil)
		}
		options.Destination = &destination
	}
	options.SyncOptions.DBPath = options.CollectorDBPath
	if !options.PublishOnly {
		result.Collection, result.CollectionError = captureRaw(ctx, options)
	}
	if options.SyncOptions.DryRun {
		return result, result.CollectionError
	}
	result.DeliveryError = publishRaw(ctx, options, &result)
	return result, errors.Join(result.CollectionError, result.DeliveryError)
}

func endpoint(value string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed == nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", failure("configuration", "invalid_server_url", nil)
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed.String(), nil
}

func reportDeliveryProgress(options Options, result *Result) {
	if options.DeliveryProgress != nil {
		options.DeliveryProgress(DeliveryProgress{Accepted: result.Accepted, Batches: result.Batches, Pending: result.Pending, PendingKnown: result.PendingKnown})
	}
}
