package schema

const (
	ExpectedSchemaName    = "io.mb3r.bering.model"
	ExpectedSchemaVersion = "1.0.0"
	ExpectedSchemaURI     = "https://mb3r-lab.github.io/Bering/schema/model/v1.0.0/model.schema.json"
	ExpectedSchemaDigest  = "sha256:272277c093f37580adcd2dded225bd37c86539d642d7910baad7e4228227d1a7"

	ExpectedSnapshotSchemaName    = "io.mb3r.bering.snapshot"
	ExpectedSnapshotSchemaVersion = "1.0.0"
	ExpectedSnapshotSchemaURI     = "https://mb3r-lab.github.io/Bering/schema/snapshot/v1.0.0/snapshot.schema.json"
	ExpectedSnapshotSchemaDigest  = "sha256:e698d5d313b48a93fc4fa77036db11b3836636e5a8852406f3a4800e527fb9b1"
)

type SchemaRef struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	URI     string `json:"uri"`
	Digest  string `json:"digest"`
}

func ExpectedRef() SchemaRef {
	return SchemaRef{
		Name:    ExpectedSchemaName,
		Version: ExpectedSchemaVersion,
		URI:     ExpectedSchemaURI,
		Digest:  ExpectedSchemaDigest,
	}
}

func ExpectedSnapshotRef() SchemaRef {
	return SchemaRef{
		Name:    ExpectedSnapshotSchemaName,
		Version: ExpectedSnapshotSchemaVersion,
		URI:     ExpectedSnapshotSchemaURI,
		Digest:  ExpectedSnapshotSchemaDigest,
	}
}
