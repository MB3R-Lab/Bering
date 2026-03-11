package runtime

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	collecttracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"

	"github.com/MB3R-Lab/Bering/internal/config"
)

func TestServiceEndToEndOTLPHTTP(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfg := config.DefaultServeConfig()
	cfg.Server.ListenAddress = "127.0.0.1:0"
	cfg.Server.GRPCListenAddress = ""
	cfg.Runtime.WindowSize = config.Duration(150 * time.Millisecond)
	cfg.Runtime.FlushInterval = config.Duration(25 * time.Millisecond)
	cfg.Runtime.MaxInMemorySpans = 100
	cfg.Sink.Directory = filepath.Join(dir, "snapshots")
	cfg.Sink.LatestPath = filepath.Join(dir, "latest.json")

	service, err := NewService(cfg, nil, slog.New(slog.NewTextHandler(ioDiscard{}, nil)))
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- service.Run(ctx) }()
	defer func() {
		cancel()
		select {
		case err := <-errCh:
			if err != nil {
				t.Fatalf("service returned error: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("service did not stop in time")
		}
	}()

	addr := waitForAddr(t, service, 2*time.Second)
	postOTLPSpanHTTP(t, "http://"+addr+"/v1/traces")

	waitForFile(t, cfg.Sink.LatestPath, 3*time.Second)
	latestRaw, err := os.ReadFile(cfg.Sink.LatestPath)
	if err != nil {
		t.Fatalf("read latest snapshot: %v", err)
	}
	if !bytes.Contains(latestRaw, []byte(`"snapshot_id"`)) {
		t.Fatalf("latest snapshot missing snapshot_id: %s", latestRaw)
	}

	checkStatus(t, "http://"+addr+"/healthz", http.StatusOK)
	checkStatus(t, "http://"+addr+"/readyz", http.StatusOK)
	metricsBody := readBody(t, "http://"+addr+"/metrics")
	if !strings.Contains(metricsBody, "spans_ingested_total") {
		t.Fatalf("metrics endpoint missing spans_ingested_total:\n%s", metricsBody)
	}
	if !strings.Contains(metricsBody, "snapshots_emitted_total") {
		t.Fatalf("metrics endpoint missing snapshots_emitted_total:\n%s", metricsBody)
	}
}

func TestServiceEndToEndOTLPGRPC(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfg := config.DefaultServeConfig()
	cfg.Server.ListenAddress = "127.0.0.1:0"
	cfg.Server.GRPCListenAddress = "127.0.0.1:0"
	cfg.Runtime.WindowSize = config.Duration(150 * time.Millisecond)
	cfg.Runtime.FlushInterval = config.Duration(25 * time.Millisecond)
	cfg.Runtime.MaxInMemorySpans = 100
	cfg.Sink.Directory = filepath.Join(dir, "snapshots")
	cfg.Sink.LatestPath = filepath.Join(dir, "latest.json")

	service, err := NewService(cfg, nil, slog.New(slog.NewTextHandler(ioDiscard{}, nil)))
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- service.Run(ctx) }()
	defer func() {
		cancel()
		select {
		case err := <-errCh:
			if err != nil {
				t.Fatalf("service returned error: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("service did not stop in time")
		}
	}()

	httpAddr := waitForAddr(t, service, 2*time.Second)
	grpcAddr := waitForGRPCAddr(t, service, 2*time.Second)
	postOTLPSpanGRPC(t, grpcAddr)

	waitForFile(t, cfg.Sink.LatestPath, 3*time.Second)
	latestRaw, err := os.ReadFile(cfg.Sink.LatestPath)
	if err != nil {
		t.Fatalf("read latest snapshot: %v", err)
	}
	if !bytes.Contains(latestRaw, []byte(`"snapshot_id"`)) {
		t.Fatalf("latest snapshot missing snapshot_id: %s", latestRaw)
	}

	checkStatus(t, "http://"+httpAddr+"/healthz", http.StatusOK)
	checkStatus(t, "http://"+httpAddr+"/readyz", http.StatusOK)
}

func waitForAddr(t *testing.T, service *Service, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if addr := service.Addr(); addr != "" {
			return addr
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("service did not publish an address in time")
	return ""
}

func waitForGRPCAddr(t *testing.T, service *Service, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if addr := service.GRPCAddr(); addr != "" {
			return addr
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("service did not publish a gRPC address in time")
	return ""
}

func postOTLPSpanHTTP(t *testing.T, url string) {
	t.Helper()
	raw, err := proto.Marshal(sampleOTLPRequest(time.Now().UTC()))
	if err != nil {
		t.Fatalf("marshal otlp request: %v", err)
	}
	resp, err := http.Post(url, "application/x-protobuf", bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("post otlp span: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected status %d: %s", resp.StatusCode, body)
	}
}

func postOTLPSpanGRPC(t *testing.T, addr string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, addr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		t.Fatalf("dial gRPC service: %v", err)
	}
	defer conn.Close()

	client := collecttracev1.NewTraceServiceClient(conn)
	if _, err := client.Export(ctx, sampleOTLPRequest(time.Now().UTC())); err != nil {
		t.Fatalf("export OTLP/gRPC request: %v", err)
	}
}

func sampleOTLPRequest(start time.Time) *collecttracev1.ExportTraceServiceRequest {
	return &collecttracev1.ExportTraceServiceRequest{
		ResourceSpans: []*tracev1.ResourceSpans{{
			Resource: &resourcev1.Resource{Attributes: []*commonv1.KeyValue{{Key: "service.name", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "frontend"}}}}},
			ScopeSpans: []*tracev1.ScopeSpans{{
				Spans: []*tracev1.Span{{
					TraceId:           []byte{0xaa, 0xbb, 0xcc},
					SpanId:            []byte{0x01, 0x02},
					Name:              "GET /checkout",
					Kind:              tracev1.Span_SPAN_KIND_SERVER,
					StartTimeUnixNano: uint64(start.UnixNano()),
					EndTimeUnixNano:   uint64(start.Add(10 * time.Millisecond).UnixNano()),
					Attributes: []*commonv1.KeyValue{
						{Key: "http.request.method", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "GET"}}},
						{Key: "http.route", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "/checkout"}}},
					},
				}},
			}},
		}},
	}
}

func waitForFile(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("file %s was not created in time", path)
}

func checkStatus(t *testing.T, url string, want int) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("get %s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != want {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected status for %s: got=%d want=%d body=%s", url, resp.StatusCode, want, body)
	}
}

func readBody(t *testing.T, url string) string {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("get %s: %v", url, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read %s: %v", url, err)
	}
	return string(body)
}
