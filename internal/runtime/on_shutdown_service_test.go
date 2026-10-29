package runtime

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/MB3R-Lab/Bering/internal/config"
	"github.com/MB3R-Lab/Bering/internal/connectors/traces"
	"github.com/MB3R-Lab/Bering/internal/schema"
	"github.com/MB3R-Lab/Bering/internal/snapshot"
)

func TestOnShutdownServiceRetainsBothOTLPSourcesUntilShutdown(t *testing.T) {
	dir := t.TempDir()
	cfg := config.DefaultServeConfig()
	cfg.Server.ListenAddress = "127.0.0.1:0"
	cfg.Runtime.PublicationMode = config.PublicationModeOnShutdown
	cfg.Runtime.Reconciliation.Enabled = false
	cfg.Runtime.WindowSize = 0
	cfg.Runtime.FlushInterval = 0
	cfg.Sink.Directory = filepath.Join(dir, "snapshots")
	cfg.Sink.LatestPath = filepath.Join(dir, "latest.json")
	cfg.Sources = []config.SourceConfig{{ID: "obi", Type: "ebpf-obi", Domain: "deployment-a", CaptureDirectory: filepath.Join(dir, "capture")}}
	service, err := NewService(cfg, nil, slog.New(slog.NewTextHandler(ioDiscard{}, nil)))
	if err != nil {
		t.Fatal(err)
	}
	if service.httpServer.ReadTimeout != 0 || service.httpServer.WriteTimeout != 0 {
		t.Fatal("on_shutdown still imposes request body/response timeouts")
	}
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- service.Run(ctx) }()
	addr := waitForAddr(t, service, 2*time.Second)
	post := func(path, trace, span string) {
		t.Helper()
		body := []byte(`{"resourceSpans":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"frontend"}}]},"scopeSpans":[{"scope":{"name":"collector"},"spans":[{"traceId":"` + trace + `","spanId":"` + span + `","kind":2,"name":"GET /items","attributes":[{"key":"http.request.method","value":{"stringValue":"GET"}},{"key":"url.path","value":{"stringValue":"/items"}}]}]}]}]}`)
		response, err := http.Post("http://"+addr+path, "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			raw, _ := io.ReadAll(response.Body)
			t.Fatalf("status=%d body=%s", response.StatusCode, raw)
		}
	}
	post("/sources/obi/v1/traces", "00000000000000000000000000000001", "0000000000000001")
	post("/v1/traces", "00000000000000000000000000000002", "0000000000000002")
	if _, err := os.Stat(cfg.Sink.LatestPath); !os.IsNotExist(err) {
		t.Fatalf("premature publication: %v", err)
	}
	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("service did not finish shutdown")
	}
	raw, err := os.ReadFile(cfg.Sink.LatestPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := schema.ValidateSnapshotJSON(raw); err != nil {
		t.Fatal(err)
	}
	env, err := snapshot.ParseJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	if env.Ingest.Spans != 2 || env.Ingest.DroppedSpans != 0 || len(env.Sources) != 2 {
		t.Fatalf("mixed-source accounting: ingest=%+v sources=%+v", env.Ingest, env.Sources)
	}
	files, err := filepath.Glob(filepath.Join(cfg.Sink.Directory, "*.json"))
	if err != nil || len(files) != 2 {
		t.Fatalf("expected one snapshot and one sidecar, files=%v err=%v", files, err)
	}
}

func TestWindowedHTTPTimeoutsRemainDefault(t *testing.T) {
	server := newHTTPServerForMode(http.NewServeMux(), config.PublicationModeWindowed)
	if server.ReadTimeout != serverReadTimeout || server.WriteTimeout != serverWriteTimeout {
		t.Fatal("windowed HTTP limits changed")
	}
}

func TestOnShutdownHTTPOverflowCannotPublishAcceptedPrefix(t *testing.T) {
	dir := t.TempDir()
	cfg := config.DefaultServeConfig()
	cfg.Runtime.PublicationMode = config.PublicationModeOnShutdown
	cfg.Runtime.Reconciliation.Enabled = false
	cfg.Runtime.MaxInMemorySpans = 1
	cfg.Sink.Directory = filepath.Join(dir, "snapshots")
	cfg.Sink.LatestPath = filepath.Join(dir, "latest.json")
	service, err := NewService(cfg, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	post := func(trace, span string) int {
		body := []byte(`{"resourceSpans":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"frontend"}}]},"scopeSpans":[{"spans":[{"traceId":"` + trace + `","spanId":"` + span + `","kind":2,"name":"GET /items","attributes":[{"key":"http.request.method","value":{"stringValue":"GET"}},{"key":"url.path","value":{"stringValue":"/items"}}]}]}]}]}`)
		req := httptest.NewRequest(http.MethodPost, "/v1/traces", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		out := httptest.NewRecorder()
		service.httpServer.Handler.ServeHTTP(out, req)
		return out.Code
	}
	if got := post("00000000000000000000000000000001", "0000000000000001"); got != http.StatusOK {
		t.Fatalf("first status: %d", got)
	}
	if got := post("00000000000000000000000000000002", "0000000000000002"); got != http.StatusInternalServerError {
		t.Fatalf("overflow status: %d", got)
	}
	if err := service.engine.Close(context.Background()); err == nil {
		t.Fatal("Close accepted overflowed batch")
	}
	if _, err := os.Stat(cfg.Sink.LatestPath); !os.IsNotExist(err) {
		t.Fatalf("published accepted prefix: %v", err)
	}
}

type closeContextSink struct {
	calls       int
	hadDeadline bool
	canceled    bool
}

func (s *closeContextSink) Write(ctx context.Context, _ snapshot.Envelope) error {
	s.calls++
	_, s.hadDeadline = ctx.Deadline()
	s.canceled = ctx.Err() != nil
	return nil
}

func TestOnShutdownServiceFinalPublicationHasNoDeadline(t *testing.T) {
	cfg := config.DefaultServeConfig()
	cfg.Server.ListenAddress = "127.0.0.1:0"
	cfg.Runtime.PublicationMode = config.PublicationModeOnShutdown
	cfg.Runtime.Reconciliation.Enabled = false
	service, err := NewService(cfg, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	sink := &closeContextSink{}
	service.engine.cfg.Sink = sink
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- service.Run(ctx) }()
	_ = waitForAddr(t, service, 2*time.Second)
	span := testSpan("frontend", "trace", "server", "server", time.Now().UTC())
	if err := service.engine.Ingest(context.Background(), []traces.Span{span}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("service did not finish shutdown")
	}
	if sink.calls != 1 || sink.hadDeadline || sink.canceled {
		t.Fatalf("publication used a canceled or bounded context: %+v", sink)
	}
}
