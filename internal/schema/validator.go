package schema

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v5"
)

//go:embed schema/model/v1.0.0/model.schema.json schema/model/v1.1.0/model.schema.json schema/snapshot/v1.0.0/snapshot.schema.json schema/snapshot/v1.1.0/snapshot.schema.json
var embeddedSchemas embed.FS

type compiledSchemaEntry struct {
	once   sync.Once
	schema *jsonschema.Schema
	err    error
}

var compiledSchemaCache = map[string]*compiledSchemaEntry{
	schemaKey(ModelContractName, ContractVersionV1_0_0):    {},
	schemaKey(ModelContractName, ContractVersionV1_1_0):    {},
	schemaKey(SnapshotContractName, ContractVersionV1_0_0): {},
	schemaKey(SnapshotContractName, ContractVersionV1_1_0): {},
}

func EmbeddedSchema() []byte {
	return mustEmbeddedBytes(ExpectedRef())
}

func EmbeddedSnapshotSchema() []byte {
	return mustEmbeddedBytes(ExpectedSnapshotRef())
}

func EmbeddedSchemaDigest() string {
	return mustEmbeddedDigest(ExpectedRef())
}

func EmbeddedSnapshotSchemaDigest() string {
	return mustEmbeddedDigest(ExpectedSnapshotRef())
}

func EmbeddedBytes(ref SchemaRef) ([]byte, error) {
	contract, ok := LookupContract(ref.Name, ref.Version)
	if !ok {
		return nil, fmt.Errorf("unknown embedded schema %s@%s", ref.Name, ref.Version)
	}
	raw, err := embeddedSchemas.ReadFile(contract.EmbeddedPath)
	if err != nil {
		return nil, fmt.Errorf("read embedded schema %s: %w", contract.EmbeddedPath, err)
	}
	out := make([]byte, len(raw))
	copy(out, raw)
	return out, nil
}

func ValidateStrict(ref SchemaRef) error {
	return validateStrictAgainst(ref, ExpectedRef())
}

func ValidateSnapshotStrict(ref SchemaRef) error {
	return validateStrictAgainst(ref, ExpectedSnapshotRef())
}

func ValidateJSON(raw []byte) error {
	return validateDocument(raw, ExpectedRef())
}

func ValidateJSONVersion(raw []byte, version string) error {
	ref, ok := ModelRef(version)
	if !ok {
		return fmt.Errorf("unsupported model schema version: %s", version)
	}
	return validateDocument(raw, ref)
}

func ValidateSnapshotJSON(raw []byte) error {
	return validateSnapshotDocument(raw, ExpectedSnapshotRef())
}

func ValidateSnapshotJSONVersion(raw []byte, version string) error {
	ref, ok := SnapshotRef(version)
	if !ok {
		return fmt.Errorf("unsupported snapshot schema version: %s", version)
	}
	return validateSnapshotDocument(raw, ref)
}

func ValidateArtifactJSON(raw []byte) error {
	doc, err := decodeJSON(raw)
	if err != nil {
		return fmt.Errorf("decode artifact json: %w", err)
	}
	ref, err := extractSchemaRef(doc)
	if err != nil {
		return fmt.Errorf("extract metadata.schema: %w", err)
	}
	switch ref.Name {
	case ModelContractName:
		return validateDocument(raw, ref)
	case SnapshotContractName:
		return validateSnapshotDocument(raw, ref)
	default:
		return fmt.Errorf("unsupported artifact schema name: %s", ref.Name)
	}
}

func validateSnapshotDocument(raw []byte, expected SchemaRef) error {
	doc, err := decodeJSON(raw)
	if err != nil {
		return fmt.Errorf("decode snapshot json: %w", err)
	}
	if err := validateDecodedDocument(doc, expected); err != nil {
		return err
	}
	root, ok := doc.(map[string]any)
	if !ok {
		return fmt.Errorf("snapshot root is not an object")
	}
	rawModel, err := json.Marshal(root["model"])
	if err != nil {
		return fmt.Errorf("encode nested model: %w", err)
	}
	if err := ValidateArtifactJSON(rawModel); err != nil {
		return fmt.Errorf("nested model validation failed: %w", err)
	}
	return nil
}

func validateDocument(raw []byte, expected SchemaRef) error {
	doc, err := decodeJSON(raw)
	if err != nil {
		return fmt.Errorf("decode model json: %w", err)
	}
	return validateDecodedDocument(doc, expected)
}

func validateDecodedDocument(doc any, expected SchemaRef) error {
	compiled, err := compiledSchema(expected)
	if err != nil {
		return fmt.Errorf("compile canonical schema: %w", err)
	}
	if err := compiled.Validate(doc); err != nil {
		return fmt.Errorf("jsonschema validation failed: %w", err)
	}
	ref, err := extractSchemaRef(doc)
	if err != nil {
		return fmt.Errorf("extract metadata.schema: %w", err)
	}
	if err := validateStrictAgainst(ref, expected); err != nil {
		return fmt.Errorf("strict metadata.schema validation failed: %w", err)
	}
	return nil
}

func validateStrictAgainst(ref, expected SchemaRef) error {
	if strings.TrimSpace(ref.Name) == "" {
		return fmt.Errorf("metadata.schema.name cannot be empty")
	}
	if strings.TrimSpace(ref.Version) == "" {
		return fmt.Errorf("metadata.schema.version cannot be empty")
	}
	if strings.TrimSpace(ref.URI) == "" {
		return fmt.Errorf("metadata.schema.uri cannot be empty")
	}
	if strings.TrimSpace(ref.Digest) == "" {
		return fmt.Errorf("metadata.schema.digest cannot be empty")
	}
	if ref.Name != expected.Name {
		return fmt.Errorf("schema name mismatch: got %q want %q", ref.Name, expected.Name)
	}
	if ref.Version != expected.Version {
		return fmt.Errorf("schema version mismatch: got %q want %q", ref.Version, expected.Version)
	}
	if ref.URI != expected.URI {
		return fmt.Errorf("schema uri mismatch: got %q want %q", ref.URI, expected.URI)
	}
	if ref.Digest != expected.Digest {
		return fmt.Errorf("schema digest mismatch: got %q want %q", ref.Digest, expected.Digest)
	}
	return nil
}

func compiledSchema(ref SchemaRef) (*jsonschema.Schema, error) {
	entry, ok := compiledSchemaCache[schemaKey(ref.Name, ref.Version)]
	if !ok {
		return nil, fmt.Errorf("unsupported schema %s@%s", ref.Name, ref.Version)
	}
	entry.once.Do(func() {
		raw, err := EmbeddedBytes(ref)
		if err != nil {
			entry.err = err
			return
		}
		compiler := jsonschema.NewCompiler()
		if err := compiler.AddResource(ref.URI, bytes.NewReader(raw)); err != nil {
			entry.err = err
			return
		}
		entry.schema, entry.err = compiler.Compile(ref.URI)
	})
	return entry.schema, entry.err
}

func decodeJSON(raw []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()

	var doc any
	if err := dec.Decode(&doc); err != nil {
		return nil, err
	}
	if dec.More() {
		return nil, fmt.Errorf("unexpected trailing tokens")
	}
	return doc, nil
}

func ExtractSchemaRef(raw []byte) (SchemaRef, error) {
	doc, err := decodeJSON(raw)
	if err != nil {
		return SchemaRef{}, err
	}
	return extractSchemaRef(doc)
}

func extractSchemaRef(doc any) (SchemaRef, error) {
	root, ok := doc.(map[string]any)
	if !ok {
		return SchemaRef{}, fmt.Errorf("root is not an object")
	}

	metadata, ok := root["metadata"].(map[string]any)
	if !ok {
		return SchemaRef{}, fmt.Errorf("metadata object missing")
	}

	rawSchema, ok := metadata["schema"].(map[string]any)
	if !ok {
		return SchemaRef{}, fmt.Errorf("metadata.schema object missing")
	}

	ref := SchemaRef{
		Name:    toString(rawSchema["name"]),
		Version: toString(rawSchema["version"]),
		URI:     toString(rawSchema["uri"]),
		Digest:  toString(rawSchema["digest"]),
	}
	return ref, nil
}

func toString(v any) string {
	s, _ := v.(string)
	return s
}

func mustEmbeddedBytes(ref SchemaRef) []byte {
	raw, err := EmbeddedBytes(ref)
	if err != nil {
		panic(err)
	}
	return raw
}

func mustEmbeddedDigest(ref SchemaRef) string {
	raw := mustEmbeddedBytes(ref)
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}
