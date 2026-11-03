package dataengine

// Metadata identifies one dataset and its accepted/published processing revisions.
type Metadata struct {
	DatabaseID, DatasetID, Kind                                         string
	Generation, InputRevision, Revision, LastIngestionAtMs, CreatedAtMs int64
	TargetGeneration                                                    int64
}
