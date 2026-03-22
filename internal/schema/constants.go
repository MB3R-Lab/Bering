package schema

import (
	"fmt"
	"sort"
)

const (
	ModelContractName    = "io.mb3r.bering.model"
	SnapshotContractName = "io.mb3r.bering.snapshot"

	ContractVersionV1_0_0 = "1.0.0"
	ContractVersionV1_1_0 = "1.1.0"
	LatestContractVersion = ContractVersionV1_1_0

	ExpectedSchemaName    = ModelContractName
	ExpectedSchemaVersion = LatestContractVersion
	ExpectedSchemaURI     = "https://mb3r-lab.github.io/Bering/schema/model/v1.1.0/model.schema.json"
	ExpectedSchemaDigest  = "sha256:bc9a60736c9e6bda9599243fd68f293b88f42ade65321d8267369a5c3214779a"

	ExpectedSnapshotSchemaName    = SnapshotContractName
	ExpectedSnapshotSchemaVersion = LatestContractVersion
	ExpectedSnapshotSchemaURI     = "https://mb3r-lab.github.io/Bering/schema/snapshot/v1.1.0/snapshot.schema.json"
	ExpectedSnapshotSchemaDigest  = "sha256:53b127608b2aaa4fabb352b998cd6b2c5ed558764729a09abea56f4f9b40fa01"
)

type SchemaRef struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	URI     string `json:"uri"`
	Digest  string `json:"digest"`
}

type PublishedContract struct {
	Ref          SchemaRef
	Artifact     string
	APIPath      string
	EmbeddedPath string
}

var publishedContracts = []PublishedContract{
	{
		Ref: SchemaRef{
			Name:    ModelContractName,
			Version: ContractVersionV1_0_0,
			URI:     "https://mb3r-lab.github.io/Bering/schema/model/v1.0.0/model.schema.json",
			Digest:  "sha256:272277c093f37580adcd2dded225bd37c86539d642d7910baad7e4228227d1a7",
		},
		Artifact:     "model",
		APIPath:      "api/schema/model/v1.0.0/model.schema.json",
		EmbeddedPath: "schema/model/v1.0.0/model.schema.json",
	},
	{
		Ref: SchemaRef{
			Name:    SnapshotContractName,
			Version: ContractVersionV1_0_0,
			URI:     "https://mb3r-lab.github.io/Bering/schema/snapshot/v1.0.0/snapshot.schema.json",
			Digest:  "sha256:87e4e887ed4a37b72f6136e268b73552eccb92941c4de2c6f3a514dd066ea972",
		},
		Artifact:     "snapshot",
		APIPath:      "api/schema/snapshot/v1.0.0/snapshot.schema.json",
		EmbeddedPath: "schema/snapshot/v1.0.0/snapshot.schema.json",
	},
	{
		Ref: SchemaRef{
			Name:    ModelContractName,
			Version: ContractVersionV1_1_0,
			URI:     ExpectedSchemaURI,
			Digest:  ExpectedSchemaDigest,
		},
		Artifact:     "model",
		APIPath:      "api/schema/model/v1.1.0/model.schema.json",
		EmbeddedPath: "schema/model/v1.1.0/model.schema.json",
	},
	{
		Ref: SchemaRef{
			Name:    SnapshotContractName,
			Version: ContractVersionV1_1_0,
			URI:     ExpectedSnapshotSchemaURI,
			Digest:  ExpectedSnapshotSchemaDigest,
		},
		Artifact:     "snapshot",
		APIPath:      "api/schema/snapshot/v1.1.0/snapshot.schema.json",
		EmbeddedPath: "schema/snapshot/v1.1.0/snapshot.schema.json",
	},
}

var publishedByKey = buildPublishedByKey(publishedContracts)

func ExpectedRef() SchemaRef {
	return mustRef(ModelContractName, LatestContractVersion)
}

func ExpectedSnapshotRef() SchemaRef {
	return mustRef(SnapshotContractName, LatestContractVersion)
}

func SupportedContractVersions() []string {
	versions := []string{ContractVersionV1_0_0, ContractVersionV1_1_0}
	sort.Strings(versions)
	return versions
}

func PublishedContracts() []PublishedContract {
	out := make([]PublishedContract, len(publishedContracts))
	copy(out, publishedContracts)
	return out
}

func LookupContract(name, version string) (PublishedContract, bool) {
	contract, ok := publishedByKey[schemaKey(name, version)]
	return contract, ok
}

func ModelRef(version string) (SchemaRef, bool) {
	contract, ok := LookupContract(ModelContractName, version)
	if !ok {
		return SchemaRef{}, false
	}
	return contract.Ref, true
}

func SnapshotRef(version string) (SchemaRef, bool) {
	contract, ok := LookupContract(SnapshotContractName, version)
	if !ok {
		return SchemaRef{}, false
	}
	return contract.Ref, true
}

func ResolveContractLine(version string) (SchemaRef, SchemaRef, error) {
	modelRef, ok := ModelRef(version)
	if !ok {
		return SchemaRef{}, SchemaRef{}, fmt.Errorf("unsupported model contract version: %s", version)
	}
	snapshotRef, ok := SnapshotRef(version)
	if !ok {
		return SchemaRef{}, SchemaRef{}, fmt.Errorf("unsupported snapshot contract version: %s", version)
	}
	return modelRef, snapshotRef, nil
}

func mustRef(name, version string) SchemaRef {
	ref, ok := lookupRef(name, version)
	if !ok {
		panic(fmt.Sprintf("unknown schema ref %s@%s", name, version))
	}
	return ref
}

func lookupRef(name, version string) (SchemaRef, bool) {
	contract, ok := LookupContract(name, version)
	if !ok {
		return SchemaRef{}, false
	}
	return contract.Ref, true
}

func buildPublishedByKey(items []PublishedContract) map[string]PublishedContract {
	out := make(map[string]PublishedContract, len(items))
	for _, item := range items {
		out[schemaKey(item.Ref.Name, item.Ref.Version)] = item
	}
	return out
}

func schemaKey(name, version string) string {
	return name + "@" + version
}
