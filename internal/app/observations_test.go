package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/MB3R-Lab/Bering/internal/evidence"
	"github.com/MB3R-Lab/Bering/internal/model"
	"github.com/MB3R-Lab/Bering/internal/quality"
	"github.com/MB3R-Lab/Bering/internal/snapshot"
)

func TestDiscoverObservationsSharedDatabase(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "synthetic-otlp.json")
	modelPath := filepath.Join(dir, "model.json")
	snapshotPath := filepath.Join(dir, "snapshot.json")
	if err := os.WriteFile(input, []byte(`{
  "resourceSpans": [
    {"resource":{"attributes":[
      {"key":"service.name","value":{"stringValue":"api-a"}},
      {"key":"service.instance.id","value":{"stringValue":"api-a-1"}},
      {"key":"host.id","value":{"stringValue":"host-a"}},
      {"key":"bering.network.scope","value":{"stringValue":"network-a"}}
    ]},"scopeSpans":[{"scope":{"name":"db.client","version":"1.0"},"spans":[{
      "traceId":"00000000000000000000000000000001","spanId":"0000000000000001",
      "name":"SELECT items","kind":3,"startTimeUnixNano":"1773220800000000000","endTimeUnixNano":"1773220800001000000",
      "attributes":[{"key":"db.system.name","value":{"stringValue":"postgresql"}},
        {"key":"server.address","value":{"stringValue":"db.internal"}},
        {"key":"server.port","value":{"intValue":"5432"}},
        {"key":"db.namespace","value":{"stringValue":"appdata"}}]
    }]}]},
    {"resource":{"attributes":[
      {"key":"service.name","value":{"stringValue":"api-b"}},
      {"key":"service.instance.id","value":{"stringValue":"api-b-1"}},
      {"key":"host.id","value":{"stringValue":"host-b"}},
      {"key":"bering.network.scope","value":{"stringValue":"network-a"}}
    ]},"scopeSpans":[{"scope":{"name":"db.client","version":"1.0"},"spans":[{
      "traceId":"00000000000000000000000000000002","spanId":"0000000000000002",
      "name":"SELECT items","kind":3,"startTimeUnixNano":"1773220801000000000","endTimeUnixNano":"1773220801001000000",
      "attributes":[{"key":"db.system.name","value":{"stringValue":"postgresql"}},
        {"key":"server.address","value":{"stringValue":"db.internal"}},
        {"key":"server.port","value":{"intValue":"5432"}}]
    }]}]}
  ]
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	runner := NewRunner(&stdout, &stderr)
	if code := runner.Run([]string{"discover", "--input", input, "--out", modelPath, "--snapshot-out", snapshotPath, "--discovered-at", "2026-03-11T12:00:00Z"}); code != ExitOK {
		t.Fatalf("discover exit=%d stderr=%s", code, stderr.String())
	}
	for _, path := range []string{modelPath, snapshotPath} {
		stdout.Reset()
		stderr.Reset()
		if code := runner.Run([]string{"validate", "--input", path}); code != ExitOK {
			t.Fatalf("validate %s exit=%d stderr=%s", path, code, stderr.String())
		}
	}

	modelRaw, err := os.ReadFile(modelPath)
	if err != nil {
		t.Fatal(err)
	}
	mdl, err := model.ParseJSON(modelRaw)
	if err != nil {
		t.Fatal(err)
	}
	if mdl.Metadata.Schema.Version != "1.3.0" {
		t.Fatalf("model schema changed: %s", mdl.Metadata.Schema.Version)
	}
	var dbID string
	for _, service := range mdl.Services {
		if service.Metadata != nil && service.Metadata.Labels["bering.node.kind"] == "database" {
			if dbID != "" {
				t.Fatal("expected one shared database node")
			}
			dbID = service.ID
			if service.Metadata.Labels["bering.network.scope"] != "declared:network-a" || len(service.Metadata.Placements) != 0 {
				t.Fatalf("database scope/placement mismatch: %+v", service.Metadata)
			}
		}
	}
	if len(mdl.Services) != 3 || dbID == "" || len(mdl.Edges) != 2 {
		t.Fatalf("topology mismatch: services=%d edges=%d db=%q", len(mdl.Services), len(mdl.Edges), dbID)
	}
	callers := map[string]bool{}
	for _, edge := range mdl.Edges {
		if edge.To != dbID || edge.Metadata == nil || edge.Metadata.Labels["bering.relation.kind"] != "client_dependency" {
			t.Fatalf("unexpected database edge: %+v", edge)
		}
		callers[edge.From] = true
	}
	if !callers["api-a"] || !callers["api-b"] {
		t.Fatalf("missing caller edges: %+v", callers)
	}

	snapshotRaw, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatal(err)
	}
	env, err := snapshot.ParseJSON(snapshotRaw)
	if err != nil {
		t.Fatal(err)
	}
	if env.Metadata.Schema.Version != "1.3.0" || env.Model.Metadata.Schema.Version != "1.3.0" {
		t.Fatalf("snapshot schema changed: envelope=%s model=%s", env.Metadata.Schema.Version, env.Model.Metadata.Schema.Version)
	}
	instanceIDs := map[string]string{}
	for _, service := range env.Discovery.Services {
		if service.Metadata == nil {
			continue
		}
		record, err := evidence.Decode(service.Metadata.Attributes[evidence.AttributeKey])
		if err != nil {
			t.Fatalf("service %s evidence: %v", service.ID, err)
		}
		if service.ID == dbID {
			if record.Database == nil || record.Database.IdentityKind != "logical_endpoint" || record.Database.PhysicalInstanceID != "" || record.Database.Port == nil || *record.Database.Port != 5432 {
				t.Fatalf("database evidence mismatch: %+v", record.Database)
			}
			continue
		}
		if len(record.Instances) != 1 || record.Instances[0].IdentityStatus != "observed" || record.Instances[0].Sources[0].Ref != input || record.Instances[0].Sources[0].ScopeName != "db.client" {
			t.Fatalf("instance evidence mismatch for %s: %+v", service.ID, record)
		}
		if record.Replicas == nil || record.Replicas.Configured != nil || record.Replicas.ObservedIdentities != 1 || record.Replicas.Basis != "compatibility_default_unknown" {
			t.Fatalf("replica evidence mismatch for %s: %+v", service.ID, record.Replicas)
		}
		instanceIDs[service.ID] = record.Instances[0].ID
	}
	for _, edge := range env.Discovery.Edges {
		if edge.To != dbID {
			continue
		}
		record, err := evidence.Decode(edge.Metadata.Attributes[evidence.AttributeKey])
		if err != nil {
			t.Fatal(err)
		}
		if record.Relation == nil || record.Relation.Kind != "client_dependency" || record.Relation.Basis != "database_client_attributes" || len(record.Relation.FromInstances) != 1 || record.Relation.FromInstances[0] != instanceIDs[edge.From] || len(record.Relation.ToInstances) != 0 {
			t.Fatalf("edge relation mismatch: %+v", record.Relation)
		}
	}
	for _, path := range []string{quality.SidecarPath(modelPath), quality.SidecarPath(snapshotPath)} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var report quality.Report
		if err := json.Unmarshal(raw, &report); err != nil {
			t.Fatal(err)
		}
		if report.SchemaVersion != quality.SchemaVersion || report.Counts.Services != 3 || report.Counts.Edges != 2 || len(report.ObservationEvidence) < 5 {
			t.Fatalf("quality evidence mismatch for %s: %+v", path, report)
		}
	}
}
