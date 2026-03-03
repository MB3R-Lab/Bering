package schema

const (
	ExpectedSchemaName    = "io.mb3r.bering.model"
	ExpectedSchemaVersion = "1.0.0"
	ExpectedSchemaURI     = "https://schemas.mb3r.dev/bering/model/v1.0.0/model.schema.json"
	ExpectedSchemaDigest  = "sha256:7dc733936a9d3f94ab92f46a30d4c8d0f5c05d60670c4247786c59a3fe7630f7"
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
