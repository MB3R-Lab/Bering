package release

type SchemaDescriptor struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	URI     string `json:"uri"`
}

type ContractDescriptor struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Kind    string `json:"kind"`
	File    string `json:"file"`
	URI     string `json:"uri"`
	Digest  string `json:"digest"`
}

type ContractsManifest struct {
	Schema      SchemaDescriptor     `json:"schema"`
	ProductName string               `json:"product_name"`
	AppVersion  string               `json:"app_version"`
	GeneratedAt string               `json:"generated_at"`
	Contracts   []ContractDescriptor `json:"contracts"`
}

type ChecksumEntry struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Algorithm string `json:"algorithm"`
	Digest    string `json:"digest"`
}

type BinaryArtifact struct {
	ID        string `json:"id"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	Target    string `json:"target"`
	Binary    string `json:"binary"`
	Path      string `json:"path"`
	Archive   string `json:"archive"`
	Format    string `json:"format"`
	ArchiveID string `json:"archive_id,omitempty"`
	SBOM      string `json:"sbom,omitempty"`
	Checksum  string `json:"checksum,omitempty"`
}

type OCIImageManifest struct {
	Repository     string   `json:"repository"`
	References     []string `json:"references"`
	Tags           []string `json:"tags"`
	Digest         string   `json:"digest,omitempty"`
	Platforms      []string `json:"platforms"`
	LocalLayout    string   `json:"local_layout,omitempty"`
	Published      bool     `json:"published"`
	MediaType      string   `json:"media_type,omitempty"`
	BuildDate      string   `json:"build_date"`
	GitCommit      string   `json:"git_commit"`
	SourceArtifact string   `json:"source_artifact,omitempty"`
}

type ChartMetadata struct {
	Name           string `json:"name"`
	Version        string `json:"version"`
	AppVersion     string `json:"app_version"`
	Package        string `json:"package"`
	PackageDigest  string `json:"package_digest"`
	OCIReference   string `json:"oci_reference,omitempty"`
	Digest         string `json:"digest,omitempty"`
	Published      bool   `json:"published"`
	Repository     string `json:"repository,omitempty"`
	SourceChartDir string `json:"source_chart_dir"`
}

type BreakingSurface struct {
	CLI             bool     `json:"cli"`
	API             bool     `json:"api"`
	InstallSurface  bool     `json:"install_surface"`
	SchemaContracts []string `json:"schema_contracts"`
}

type ReleaseNotes struct {
	Headline   string          `json:"headline"`
	Summary    string          `json:"summary"`
	Highlights []string        `json:"highlights"`
	Breaking   BreakingSurface `json:"breaking"`
}

type ReleaseChecksums struct {
	Algorithm string          `json:"algorithm"`
	File      string          `json:"file"`
	Digest    string          `json:"digest"`
	Entries   []ChecksumEntry `json:"entries"`
}

type ContractsPack struct {
	Archive  string `json:"archive"`
	Digest   string `json:"digest"`
	Manifest string `json:"manifest"`
}

type ReleaseManifest struct {
	Schema        SchemaDescriptor     `json:"schema"`
	ProductName   string               `json:"product_name"`
	AppVersion    string               `json:"app_version"`
	GitTag        string               `json:"git_tag,omitempty"`
	GitCommit     string               `json:"git_commit"`
	BuildDate     string               `json:"build_date"`
	Binaries      []BinaryArtifact     `json:"binaries"`
	Checksums     ReleaseChecksums     `json:"checksums"`
	OCIImages     []OCIImageManifest   `json:"oci_images"`
	HelmChart     ChartMetadata        `json:"helm_chart"`
	ContractsPack ContractsPack        `json:"contracts_pack"`
	Contracts     []ContractDescriptor `json:"contracts"`
	ReleaseNotes  ReleaseNotes         `json:"release_notes"`
}
