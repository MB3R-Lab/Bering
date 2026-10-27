package evidence

import (
	"reflect"
	"testing"
)

func TestEncodeCanonicalizesOrderingAndDuplicates(t *testing.T) {
	t.Parallel()

	configured := 3
	port := 5432
	record := Record{
		Replicas: &ReplicaCount{ModelValue: 2, Basis: " observed identities ", ObservedIdentities: 2, Configured: &configured},
		Instances: []Instance{
			{ID: " worker-b ", IdentityStatus: "observed", Attributes: map[string]string{"zone": "b"}, Sources: []Source{{Ref: "z"}, {Ref: "a"}, {Ref: "a"}}},
			{ID: "worker-a", IdentityStatus: " observed ", Attributes: map[string]string{"zone": "a"}},
			{ID: "worker-a", IdentityStatus: " observed ", Attributes: map[string]string{"zone": "a"}},
		},
		Relation: &Relation{
			Kind: " ancestry ", Basis: " exact parent ",
			FromInstances: []string{"worker-b", "worker-a", "worker-b"},
			ToInstances:   []string{"database-a", "database-a"},
			Sources:       []Source{{ScopeName: "instrumentation", ScopeVersion: "1"}},
		},
		Database: &Database{
			System: " sql ", Address: " database.internal ", Port: &port,
			Namespaces: []string{"inventory", "inventory"}, IdentityKind: "logical_endpoint",
			Sources: []Source{{Ref: "catalog"}},
		},
		Warnings: []string{" second ", "first", "first"},
	}

	first, err := Encode(record)
	if err != nil {
		t.Fatalf("encode evidence: %v", err)
	}
	second, err := Encode(record)
	if err != nil {
		t.Fatalf("encode evidence again: %v", err)
	}
	if first != second {
		t.Fatalf("encoding is not deterministic:\n%s\n%s", first, second)
	}

	decoded, err := Decode(first)
	if err != nil {
		t.Fatalf("decode evidence: %v", err)
	}
	if decoded.SchemaVersion != SchemaVersion {
		t.Fatalf("schema version mismatch: %q", decoded.SchemaVersion)
	}
	if got, want := []string{decoded.Instances[0].ID, decoded.Instances[1].ID}, []string{"worker-a", "worker-b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("instance order/dedup mismatch: got=%v want=%v", got, want)
	}
	if got, want := decoded.Relation.FromInstances, []string{"worker-a", "worker-b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("relation instances mismatch: got=%v want=%v", got, want)
	}
	if got, want := decoded.Warnings, []string{"first", "second"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("warning order/dedup mismatch: got=%v want=%v", got, want)
	}
	if got := decoded.Instances[1].Sources; len(got) != 2 || got[0].Ref != "a" || got[1].Ref != "z" {
		t.Fatalf("source order/dedup mismatch: %+v", got)
	}
	if decoded.Database == nil || decoded.Database.System != "sql" || decoded.Database.Port == nil || *decoded.Database.Port != 5432 {
		t.Fatalf("database evidence did not round-trip: %+v", decoded.Database)
	}
}

func TestDecodeRejectsMalformedAndUnsupportedVersions(t *testing.T) {
	t.Parallel()

	for name, value := range map[string]string{
		"malformed":           `{"schema_version":`,
		"missing version":     `{}`,
		"unsupported version": `{"schema_version":"2.0.0"}`,
		"unknown field":       `{"schema_version":"1.0.0","unexpected":true}`,
		"duplicate field":     `{"schema_version":"1.0.0","schema_version":"1.0.0"}`,
		"trailing value":      `{"schema_version":"1.0.0"}{}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := Decode(value); err == nil {
				t.Fatalf("Decode(%q) unexpectedly succeeded", value)
			}
		})
	}
}

func TestEncodeRejectsUnsupportedVersion(t *testing.T) {
	t.Parallel()
	if _, err := Encode(Record{SchemaVersion: "0.9.0"}); err == nil {
		t.Fatal("Encode accepted an unsupported evidence version")
	}
}

func TestNormalizeMergesEquivalentInstanceSourcesButPreservesAttributeConflicts(t *testing.T) {
	t.Parallel()

	instances := []Instance{
		{ID: "worker-1", IdentityStatus: "observed", Attributes: map[string]string{"host": "node-a"}, Sources: []Source{{Ref: "collector-b", ScopeName: "runtime", ScopeVersion: "2"}}},
		{ID: "worker-1", IdentityStatus: "observed", Attributes: map[string]string{"host": "node-a"}, Sources: []Source{{Ref: "collector-a", ScopeName: "runtime", ScopeVersion: "1"}}},
		{ID: "worker-1", IdentityStatus: "observed", Attributes: map[string]string{"host": "node-b"}, Sources: []Source{{Ref: "collector-c"}}},
	}
	forward, err := Encode(Record{Instances: instances})
	if err != nil {
		t.Fatalf("encode forward evidence: %v", err)
	}
	for left, right := 0, len(instances)-1; left < right; left, right = left+1, right-1 {
		instances[left], instances[right] = instances[right], instances[left]
	}
	reversed, err := Encode(Record{Instances: instances})
	if err != nil {
		t.Fatalf("encode reversed evidence: %v", err)
	}
	if forward != reversed {
		t.Fatalf("input order changed normalized evidence:\n%s\n%s", forward, reversed)
	}
	record, err := Decode(forward)
	if err != nil {
		t.Fatalf("decode evidence: %v", err)
	}
	if len(record.Instances) != 2 {
		t.Fatalf("equivalent sources were not merged or conflicting attributes were lost: %+v", record.Instances)
	}
	for _, instance := range record.Instances {
		if instance.Attributes["host"] == "node-a" {
			if got := instance.Sources; len(got) != 2 || got[0].Ref != "collector-a" || got[1].Ref != "collector-b" {
				t.Fatalf("merged source provenance mismatch: %+v", got)
			}
			return
		}
	}
	t.Fatalf("merged instance variant not found: %+v", record.Instances)
}

func TestNormalizeCoalescesEquivalentDatabaseAliasesDeterministically(t *testing.T) {
	t.Parallel()

	port := 5432
	aliases := []Database{
		{System: "sql", Address: "db.internal", Port: &port, NetworkScope: "fabric-a", IdentityKind: "logical_endpoint", Namespaces: []string{"orders"}, Sources: []Source{{Ref: "collector-b"}}},
		{System: " sql ", Address: " db.internal ", Port: &port, NetworkScope: " fabric-a ", IdentityKind: " logical_endpoint ", Namespaces: []string{"billing", "orders"}, Sources: []Source{{Ref: "collector-a"}}},
		{System: "sql", Address: "db.internal", Port: &port, NetworkScope: "fabric-b", IdentityKind: "logical_endpoint", Namespaces: []string{"orders"}, Sources: []Source{{Ref: "collector-scope-b"}}},
		{System: "sql", Address: "db-replica.internal", Port: &port, NetworkScope: "fabric-a", IdentityKind: "logical_endpoint", Namespaces: []string{"orders"}, Sources: []Source{{Ref: "collector-c"}}},
	}
	forward, err := Encode(Record{DatabaseAliases: aliases})
	if err != nil {
		t.Fatalf("encode forward aliases: %v", err)
	}
	for left, right := 0, len(aliases)-1; left < right; left, right = left+1, right-1 {
		aliases[left], aliases[right] = aliases[right], aliases[left]
	}
	reversed, err := Encode(Record{DatabaseAliases: aliases})
	if err != nil {
		t.Fatalf("encode reversed aliases: %v", err)
	}
	if forward != reversed {
		t.Fatalf("database alias encoding depends on input order:\n%s\n%s", forward, reversed)
	}
	record, err := Decode(forward)
	if err != nil {
		t.Fatalf("decode aliases: %v", err)
	}
	if len(record.DatabaseAliases) != 3 {
		t.Fatalf("equivalent aliases were not coalesced or distinct endpoint was lost: %+v", record.DatabaseAliases)
	}
	for _, alias := range record.DatabaseAliases {
		if alias.Address == "db.internal" && alias.NetworkScope == "fabric-a" {
			if !reflect.DeepEqual(alias.Namespaces, []string{"billing", "orders"}) || len(alias.Sources) != 2 ||
				alias.Sources[0].Ref != "collector-a" || alias.Sources[1].Ref != "collector-b" {
				t.Fatalf("coalesced alias evidence mismatch: %+v", alias)
			}
			return
		}
	}
	t.Fatalf("coalesced database alias not found: %+v", record.DatabaseAliases)
}
