package runtime

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/MB3R-Lab/Bering/internal/config"
	"github.com/MB3R-Lab/Bering/internal/connectors/ebpf"
	"github.com/MB3R-Lab/Bering/internal/schema"
	"github.com/MB3R-Lab/Bering/internal/snapshot"
)

func TestEBPFSourceCapturesWireInputAndBuildsModel(t *testing.T) {
	dir := t.TempDir()
	cfg := config.DefaultServeConfig()
	cfg.Runtime.Reconciliation.Enabled = false
	cfg.Sink.Directory = filepath.Join(dir, "snapshots")
	cfg.Sink.LatestPath = filepath.Join(dir, "latest.json")
	cfg.Sink.SignalQualityPath = filepath.Join(dir, "quality.json")
	source := config.SourceConfig{ID: "obi-main", Type: "ebpf-obi", Domain: "deployment", CaptureDirectory: filepath.Join(dir, "capture")}
	cfg.Sources = []config.SourceConfig{source}
	service, err := NewService(cfg, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"resourceSpans":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"edge-api"}}]},"scopeSpans":[{"scope":{"name":"test.collector"},"spans":[{"traceId":"00000000000000000000000000000001","spanId":"0000000000000001","kind":2,"name":"GET /call","attributes":[{"key":"http.request.method","value":{"stringValue":"GET"}},{"key":"http.route","value":{"stringValue":"/call"}}]}]}]}]}`)
	req := httptest.NewRequest(http.MethodPost, "/sources/obi-main/v1/traces", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	service.httpServer.Handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("response %d: %s", recorder.Code, recorder.Body.String())
	}
	files, err := filepath.Glob(filepath.Join(source.CaptureDirectory, "*.json"))
	if err != nil || len(files) != 1 {
		t.Fatalf("capture files=%v err=%v", files, err)
	}
	raw, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(raw, body) {
		t.Fatal("wire capture changed")
	}
	loaded, err := ebpf.Load(source.CaptureDirectory, ebpf.Options{Domain: "deployment"})
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Spans) != 1 || loaded.Spans[0].SourceDomain != "ebpf:deployment" || loaded.Spans[0].SpanID != "0000000000000001" {
		t.Fatalf("replay=%+v", loaded)
	}
	if err := service.engine.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	raw, err = os.ReadFile(cfg.Sink.LatestPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := schema.ValidateSnapshotJSON(raw); err != nil {
		t.Fatal(err)
	}
	artifact, err := snapshot.ParseJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(artifact.Model.Services) != 1 || artifact.Model.Services[0].ID != "edge-api" {
		t.Fatalf("model=%+v", artifact.Model)
	}
	if len(artifact.Sources) != 1 || artifact.Sources[0].Type != "ebpf" {
		t.Fatalf("sources=%+v", artifact.Sources)
	}
	if artifact.Sources[0].Observations != len(loaded.Spans) {
		t.Fatalf("live source count %d differs from raw replay %d", artifact.Sources[0].Observations, len(loaded.Spans))
	}
}

func TestInvalidEBPFSourceConfigurationRejected(t *testing.T) {
	for _, source := range []config.SourceConfig{
		{ID: "../source", Type: "ebpf-obi"}, {ID: "source", Type: "other"}, {ID: "source", Type: "ebpf-obi", Domain: "bad/domain"},
	} {
		cfg := config.DefaultServeConfig()
		cfg.Runtime.Reconciliation.Enabled = false
		cfg.Sources = []config.SourceConfig{source}
		if _, err := NewService(cfg, nil, nil); err == nil {
			t.Fatalf("accepted %+v", source)
		}
	}
}
