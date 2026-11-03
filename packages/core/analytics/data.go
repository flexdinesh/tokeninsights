package analytics

const ChartLimit = 12
const SessionOptionLimit = 100

type Query struct {
	Quality        string
	Selection      Selection
	Tab            string
	LocationGroup  RepoGroup
	RepositoryKeys []string
	DirectoryKeys  []string
	Sort           string
	Direction      string
	Page           int
	PageSize       int
}

// Row is a numeric analytics result, independent of API transport.
type Row struct {
	Key                 string
	Name                string
	Harness             string
	Provider            string
	Model               string
	Date                int64
	Sessions            int64
	Input               int64
	Output              int64
	Reasoning           int64
	CacheRead           int64
	CacheWrite          int64
	Total               int64
	Context             int64
	AverageContext      int64
	MedianContext       int64
	MaxContext          int64
	LocationKey         string
	LocationName        string
	DirectoryNames      []string
	HasUnknownDirectory bool
	RepositoryKey       string
	RepositoryName      string
}

type Dashboard struct {
	FactCount     int64
	Generation    int64
	InputRevision int64
	Pending       int64
	Unresolved    int64
	Quality       string
	Revision      int64
	DatabaseID    string
	DatasetID     string
	Rows          []Row
	Chart         []Row
	RowCount      int
	Page          int
	PageSize      int
	Summary       ViewerSummaryRow
	LastSynced    int64
	Range         string
}
