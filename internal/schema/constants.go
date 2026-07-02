package schema

import (
	"fmt"
)

const (
	ModelContractName    = "io.mb3r.bering.model"
	SnapshotContractName = "io.mb3r.bering.snapshot"

	ContractVersionV1_3_0 = "1.3.0"
	LatestContractVersion = ContractVersionV1_3_0

	ExpectedSchemaName    = ModelContractName
	ExpectedSchemaVersion = LatestContractVersion
	ExpectedSchemaURI     = "https://mb3r-lab.github.io/Bering/schema/model/v1.3.0/model.schema.json"
	ExpectedSchemaDigest  = "sha256:2aa8a3550a25dc626ba6d2f5833569efca2f382b9e5c9c3405be93695d7d48ae"

	ExpectedSnapshotSchemaName    = SnapshotContractName
	ExpectedSnapshotSchemaVersion = LatestContractVersion
	ExpectedSnapshotSchemaURI     = "https://mb3r-lab.github.io/Bering/schema/snapshot/v1.3.0/snapshot.schema.json"
	ExpectedSnapshotSchemaDigest  = "sha256:cb778e5b0866d9ce5cfe7f23b8d98a339603593a0247cccd9cddaf05c7ae4bb1"
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
			Version: ContractVersionV1_3_0,
			URI:     ExpectedSchemaURI,
			Digest:  ExpectedSchemaDigest,
		},
		Artifact:     "model",
		APIPath:      "api/schema/model/v1.3.0/model.schema.json",
		EmbeddedPath: "schema/model/v1.3.0/model.schema.json",
	},
	{
		Ref: SchemaRef{
			Name:    SnapshotContractName,
			Version: ContractVersionV1_3_0,
			URI:     ExpectedSnapshotSchemaURI,
			Digest:  ExpectedSnapshotSchemaDigest,
		},
		Artifact:     "snapshot",
		APIPath:      "api/schema/snapshot/v1.3.0/snapshot.schema.json",
		EmbeddedPath: "schema/snapshot/v1.3.0/snapshot.schema.json",
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
	return []string{ContractVersionV1_3_0}
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
