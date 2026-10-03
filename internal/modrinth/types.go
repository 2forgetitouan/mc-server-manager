package modrinth

// Project represents a Modrinth project (mod, modpack, etc.).
type Project struct {
	ID           string   `json:"id"`
	Slug         string   `json:"slug"`
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	ServerSide   string   `json:"server_side"` // required, optional, unsupported
	ClientSide   string   `json:"client_side"` // required, optional, unsupported
	GameVersions []string `json:"game_versions"`
	Loaders      []string `json:"loaders"`
}

// Version represents a specific release of a Modrinth project.
type Version struct {
	ID            string        `json:"id"`
	ProjectID     string        `json:"project_id"`
	Name          string        `json:"name"`
	VersionNumber string        `json:"version_number"`
	GameVersions  []string      `json:"game_versions"`
	Loaders       []string      `json:"loaders"`
	VersionType   string        `json:"version_type"` // release, beta, alpha
	DatePublished string        `json:"date_published"`
	Files         []VersionFile `json:"files"`
	Dependencies  []Dependency  `json:"dependencies"`
}

// VersionFile represents a downloadable file attached to a version.
type VersionFile struct {
	Hashes   FileHashes `json:"hashes"`
	URL      string     `json:"url"`
	Filename string     `json:"filename"`
	Primary  bool       `json:"primary"`
	Size     int64      `json:"size"`
}

// FileHashes holds the hash values for a version file.
type FileHashes struct {
	SHA512 string `json:"sha512"`
	SHA1   string `json:"sha1"`
}

// Dependency represents a dependency relationship between versions.
type Dependency struct {
	VersionID *string `json:"version_id"`
	ProjectID *string `json:"project_id"`
	FileName  *string `json:"file_name"`
	Type      string  `json:"dependency_type"` // required, optional, incompatible, embedded
}

// SearchResult holds a paginated list of search hits from the Modrinth API.
type SearchResult struct {
	Hits      []SearchHit `json:"hits"`
	Offset    int         `json:"offset"`
	Limit     int         `json:"limit"`
	TotalHits int         `json:"total_hits"`
}

// SearchHit represents a single result in a Modrinth search response.
type SearchHit struct {
	ProjectID    string   `json:"project_id"`
	Slug         string   `json:"slug"`
	Title        string   `json:"title"`
	ServerSide   string   `json:"server_side"`
	ClientSide   string   `json:"client_side"`
	GameVersions []string `json:"versions"`
}

// HashLookupResult maps file hashes to their corresponding versions.
type HashLookupResult map[string]Version

// UpdateInfo represents a mod that has an update available.
type UpdateInfo struct {
	ProjectSlug   string
	ProjectTitle  string
	CurrentFile   string
	CurrentHash   string
	LatestVersion Version
	LatestFile    VersionFile
	Dependencies  []Dependency
}
