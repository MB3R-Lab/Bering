package app

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/MB3R-Lab/Bering/internal/config"
	"github.com/MB3R-Lab/Bering/internal/connectors/topology"
	"github.com/MB3R-Lab/Bering/internal/connectors/traces"
	"github.com/MB3R-Lab/Bering/internal/discovery"
	"github.com/MB3R-Lab/Bering/internal/model"
	"github.com/MB3R-Lab/Bering/internal/overlay"
	beringruntime "github.com/MB3R-Lab/Bering/internal/runtime"
	"github.com/MB3R-Lab/Bering/internal/schema"
	"github.com/MB3R-Lab/Bering/internal/snapshot"
)

const (
	ExitOK    = 0
	ExitError = 1
)

type Runner struct {
	stdout io.Writer
	stderr io.Writer
	now    func() time.Time
}

func NewRunner(stdout, stderr io.Writer) Runner {
	return Runner{
		stdout: stdout,
		stderr: stderr,
		now:    time.Now,
	}
}

func (r Runner) Run(args []string) int {
	if len(args) == 0 {
		r.printUsage()
		return ExitError
	}

	switch args[0] {
	case "discover":
		return r.runDiscover(args[1:])
	case "validate":
		return r.runValidate(args[1:])
	case "serve":
		return r.runServe(args[1:])
	case "help", "--help", "-h":
		r.printUsage()
		return ExitOK
	default:
		r.printfErr("unknown command: %s\n\n", args[0])
		r.printUsage()
		return ExitError
	}
}

func (r Runner) runDiscover(args []string) int {
	fs := flag.NewFlagSet("discover", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	input := fs.String("input", "", "Path to discovery input file or directory")
	out := fs.String("out", "bering-model.json", "Path to output model JSON")
	snapshotOut := fs.String("snapshot-out", "", "Optional path to output snapshot envelope JSON")
	replicas := fs.String("replicas", "", "Path to replicas override file (yaml or json)")
	discoveredAt := fs.String("discovered-at", "", "RFC3339 timestamp override for metadata.discovered_at")
	var overlays stringSliceFlag
	fs.Var(&overlays, "overlay", "Path to a discovery overlay file (yaml or json); may be repeated")

	if err := fs.Parse(args); err != nil {
		r.printfErr("discover flag parse error: %v\n", err)
		return ExitError
	}
	if strings.TrimSpace(*input) == "" {
		r.printfErr("discover requires --input\n")
		return ExitError
	}

	discoveredAtValue := strings.TrimSpace(*discoveredAt)
	if discoveredAtValue == "" {
		discoveredAtValue = r.now().UTC().Format(time.RFC3339)
	}
	if _, err := time.Parse(time.RFC3339, discoveredAtValue); err != nil {
		r.printfErr("invalid --discovered-at value (expected RFC3339): %v\n", err)
		return ExitError
	}

	loadedOverlays, err := overlay.LoadFiles(overlays.Values())
	if err != nil {
		r.printfErr("load overlays: %v\n", err)
		return ExitError
	}

	override := map[string]int{}
	if strings.TrimSpace(*replicas) != "" {
		override, err = traces.LoadReplicasOverride(*replicas)
		if err != nil {
			r.printfErr("load replicas override: %v\n", err)
			return ExitError
		}
	}

	result, err := loadDiscoveryResult(*input, discoveredAtValue, override, loadedOverlays)
	if err != nil {
		r.printfErr("discover model: %v\n", err)
		return ExitError
	}

	raw, err := model.MarshalCanonical(result.Model)
	if err != nil {
		r.printfErr("serialize model: %v\n", err)
		return ExitError
	}
	if err := schema.ValidateJSON(raw); err != nil {
		r.printfErr("post-discovery model validation failed: %v\n", err)
		return ExitError
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		r.printfErr("create output directory: %v\n", err)
		return ExitError
	}
	if err := os.WriteFile(*out, raw, 0o644); err != nil {
		r.printfErr("write output model: %v\n", err)
		return ExitError
	}

	if strings.TrimSpace(*snapshotOut) != "" {
		env, err := buildBatchSnapshot(result, discoveredAtValue)
		if err != nil {
			r.printfErr("build snapshot output: %v\n", err)
			return ExitError
		}
		rawSnapshot, err := snapshot.MarshalCanonical(env)
		if err != nil {
			r.printfErr("serialize snapshot output: %v\n", err)
			return ExitError
		}
		if err := schema.ValidateSnapshotJSON(rawSnapshot); err != nil {
			r.printfErr("post-discovery snapshot validation failed: %v\n", err)
			return ExitError
		}
		if err := os.MkdirAll(filepath.Dir(*snapshotOut), 0o755); err != nil {
			r.printfErr("create snapshot output directory: %v\n", err)
			return ExitError
		}
		if err := os.WriteFile(*snapshotOut, rawSnapshot, 0o644); err != nil {
			r.printfErr("write snapshot output: %v\n", err)
			return ExitError
		}
		r.printf("snapshot written: %s\n", *snapshotOut)
	}

	r.printf("model written: %s\n", *out)
	r.printf("services=%d edges=%d endpoints=%d confidence=%.2f\n", len(result.Model.Services), len(result.Model.Edges), len(result.Model.Endpoints), result.Model.Metadata.Confidence)
	return ExitOK
}

func (r Runner) runValidate(args []string) int {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	input := fs.String("input", "", "Path to Bering model or snapshot JSON")

	if err := fs.Parse(args); err != nil {
		r.printfErr("validate flag parse error: %v\n", err)
		return ExitError
	}
	if strings.TrimSpace(*input) == "" {
		r.printfErr("validate requires --input\n")
		return ExitError
	}

	raw, err := os.ReadFile(*input)
	if err != nil {
		r.printfErr("read artifact file: %v\n", err)
		return ExitError
	}

	if err := schema.ValidateArtifactJSON(raw); err != nil {
		r.printfErr("contract validation failed: %v\n", err)
		return ExitError
	}
	ref, err := schema.ExtractSchemaRef(raw)
	if err != nil {
		r.printfErr("extract schema ref: %v\n", err)
		return ExitError
	}
	switch ref.Name {
	case schema.ExpectedSchemaName:
		if _, err := model.ParseJSON(raw); err != nil {
			r.printfErr("semantic validation failed: %v\n", err)
			return ExitError
		}
	case schema.ExpectedSnapshotSchemaName:
		if _, err := snapshot.ParseJSON(raw); err != nil {
			r.printfErr("semantic validation failed: %v\n", err)
			return ExitError
		}
	default:
		r.printfErr("unsupported artifact schema: %s\n", ref.Name)
		return ExitError
	}

	r.printf("artifact is valid: %s\n", *input)
	return ExitOK
}

func (r Runner) runServe(args []string) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	configPath := fs.String("config", "", "Path to serve config YAML/JSON")
	listen := fs.String("listen", "", "Override config server listen address")
	grpcListen := fs.String("grpc-listen", "", "Override config server OTLP/gRPC listen address")
	flushInterval := fs.String("flush-interval", "", "Override config runtime flush interval (e.g. 5s)")
	windowSize := fs.String("window-size", "", "Override config runtime window size (e.g. 30s)")
	maxInMemory := fs.Int("max-in-memory-spans", -1, "Override config runtime max in-memory spans")
	latePolicy := fs.String("late-span-policy", "", "Override config runtime late span policy (drop|current_window)")
	sinkDir := fs.String("sink-dir", "", "Override config sink directory")
	latestPath := fs.String("latest-path", "", "Override config stable latest snapshot path")
	logFormat := fs.String("log-format", "", "Override log format (text|json)")
	var overlays stringSliceFlag
	fs.Var(&overlays, "overlay", "Override overlay file list; may be repeated")

	if err := fs.Parse(args); err != nil {
		r.printfErr("serve flag parse error: %v\n", err)
		return ExitError
	}

	cfg, err := config.LoadServeConfig(*configPath)
	if err != nil {
		r.printfErr("load config: %v\n", err)
		return ExitError
	}
	if strings.TrimSpace(*listen) != "" {
		cfg.Server.ListenAddress = strings.TrimSpace(*listen)
	}
	if strings.TrimSpace(*grpcListen) != "" {
		cfg.Server.GRPCListenAddress = strings.TrimSpace(*grpcListen)
	}
	if strings.TrimSpace(*flushInterval) != "" {
		parsed, err := time.ParseDuration(strings.TrimSpace(*flushInterval))
		if err != nil {
			r.printfErr("parse --flush-interval: %v\n", err)
			return ExitError
		}
		cfg.Runtime.FlushInterval = config.Duration(parsed)
	}
	if strings.TrimSpace(*windowSize) != "" {
		parsed, err := time.ParseDuration(strings.TrimSpace(*windowSize))
		if err != nil {
			r.printfErr("parse --window-size: %v\n", err)
			return ExitError
		}
		cfg.Runtime.WindowSize = config.Duration(parsed)
	}
	if *maxInMemory > 0 {
		cfg.Runtime.MaxInMemorySpans = *maxInMemory
	}
	if strings.TrimSpace(*latePolicy) != "" {
		cfg.Runtime.LateSpanPolicy = strings.TrimSpace(*latePolicy)
	}
	if strings.TrimSpace(*sinkDir) != "" {
		cfg.Sink.Directory = strings.TrimSpace(*sinkDir)
	}
	if strings.TrimSpace(*latestPath) != "" {
		cfg.Sink.LatestPath = strings.TrimSpace(*latestPath)
	}
	if len(overlays.items) > 0 {
		cfg.Overlays = overlays.Values()
	}
	if err := cfg.Validate(); err != nil {
		r.printfErr("invalid config: %v\n", err)
		return ExitError
	}

	loadedOverlays, err := overlay.LoadFiles(cfg.Overlays)
	if err != nil {
		r.printfErr("load overlays: %v\n", err)
		return ExitError
	}

	logger := newLogger(r.stderr, cfg.Logging.Structured, strings.TrimSpace(*logFormat))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	service, err := beringruntime.NewService(cfg, loadedOverlays, logger)
	if err != nil {
		r.printfErr("create service: %v\n", err)
		return ExitError
	}
	if err := service.Run(ctx); err != nil {
		r.printfErr("serve failed: %v\n", err)
		return ExitError
	}
	return ExitOK
}

func (r Runner) printUsage() {
	fmt.Fprintln(r.stdout, "Bering CLI")
	fmt.Fprintln(r.stdout)
	fmt.Fprintln(r.stdout, "Usage:")
	fmt.Fprintln(r.stdout, "  bering discover --input <trace-file|topology-file|dir> [--out bering-model.json] [--snapshot-out bering-snapshot.json] [--replicas replicas.yaml|json] [--overlay overlay.yaml] [--discovered-at RFC3339]")
	fmt.Fprintln(r.stdout, "  bering validate --input <bering-model.json|bering-snapshot.json>")
	fmt.Fprintln(r.stdout, "  bering serve --config configs/serve.sample.yaml [--listen :8080] [--grpc-listen :4317] [--window-size 30s] [--flush-interval 5s]")
}

func (r Runner) printf(format string, args ...any) {
	fmt.Fprintf(r.stdout, format, args...)
}

func (r Runner) printfErr(format string, args ...any) {
	fmt.Fprintf(r.stderr, format, args...)
}

type stringSliceFlag struct {
	items []string
}

func (f *stringSliceFlag) String() string {
	return strings.Join(f.items, ",")
}

func (f *stringSliceFlag) Set(value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fmt.Errorf("value cannot be empty")
	}
	f.items = append(f.items, trimmed)
	return nil
}

func (f *stringSliceFlag) Values() []string {
	return append([]string(nil), f.items...)
}

func newLogger(w io.Writer, configStructured bool, formatOverride string) *slog.Logger {
	structured := configStructured
	switch strings.ToLower(strings.TrimSpace(formatOverride)) {
	case "json":
		structured = true
	case "text":
		structured = false
	}
	if structured {
		return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: slog.LevelInfo}))
	}
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: slog.LevelInfo}))
}

func buildBatchSnapshot(result discovery.Result, discoveredAt string) (snapshot.Envelope, error) {
	topologyVersion, err := snapshot.TopologyDigest(result.Model)
	if err != nil {
		return snapshot.Envelope{}, err
	}
	env := snapshot.Envelope{
		SnapshotID:      snapshot.BuildSnapshotID(discoveredAt, discoveredAt, topologyVersion),
		TopologyVersion: topologyVersion,
		WindowStart:     discoveredAt,
		WindowEnd:       discoveredAt,
		Ingest: snapshot.IngestSummary{
			Spans:  result.SpanCount,
			Traces: result.TraceCount,
		},
		Counts: snapshot.Counts{
			Services:  len(result.Model.Services),
			Edges:     len(result.Model.Edges),
			Endpoints: len(result.Model.Endpoints),
		},
		Coverage:  result.Coverage,
		Sources:   result.Sources,
		Discovery: result.Discovery,
		Model:     result.Model,
		Metadata: snapshot.Metadata{
			SourceType: discovery.SourceTypeBering,
			SourceRef:  result.Model.Metadata.SourceRef,
			EmittedAt:  discoveredAt,
			Confidence: result.Coverage.Confidence,
			Schema: model.SchemaRef{
				Name:    schema.ExpectedSnapshotSchemaName,
				Version: schema.ExpectedSnapshotSchemaVersion,
				URI:     schema.ExpectedSnapshotSchemaURI,
				Digest:  schema.ExpectedSnapshotSchemaDigest,
			},
		},
	}
	env.Diff = snapshot.ComputeDiff(nil, env)
	env.SortDeterministic()
	return env, nil
}

func loadDiscoveryResult(input, discoveredAt string, override map[string]int, loadedOverlays []overlay.File) (discovery.Result, error) {
	sourceRef := discovery.BuildSourceRef(input)
	opts := discovery.Options{
		SourceRef:        sourceRef,
		DiscoveredAt:     discoveredAt,
		ReplicasOverride: override,
		Overlays:         loadedOverlays,
	}

	spans, traceErr := traces.Load(input)
	if traceErr == nil {
		return discovery.Discover(spans, opts)
	}

	doc, topologyErr := topology.LoadFile(input)
	if topologyErr == nil {
		return discovery.DiscoverTopology(doc, opts)
	}

	return discovery.Result{}, fmt.Errorf("load input as traces: %w; as topology_api: %v", traceErr, topologyErr)
}
