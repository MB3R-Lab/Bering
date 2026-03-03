package app

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/MB3R-Lab/Bering/internal/connectors/traces"
	"github.com/MB3R-Lab/Bering/internal/discovery"
	"github.com/MB3R-Lab/Bering/internal/model"
	"github.com/MB3R-Lab/Bering/internal/schema"
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

	input := fs.String("input", "", "Path to trace input file or directory")
	out := fs.String("out", "bering-model.json", "Path to output model JSON")
	replicas := fs.String("replicas", "", "Path to replicas override file (yaml or json)")
	discoveredAt := fs.String("discovered-at", "", "RFC3339 timestamp override for metadata.discovered_at")

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

	spans, err := traces.Load(*input)
	if err != nil {
		r.printfErr("load traces: %v\n", err)
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

	mdl, err := discovery.Build(spans, discovery.Options{
		SourceRef:        discovery.BuildSourceRef(*input),
		DiscoveredAt:     discoveredAtValue,
		ReplicasOverride: override,
	})
	if err != nil {
		r.printfErr("discover model: %v\n", err)
		return ExitError
	}

	raw, err := model.MarshalCanonical(mdl)
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

	r.printf("model written: %s\n", *out)
	r.printf("services=%d edges=%d endpoints=%d confidence=%.2f\n", len(mdl.Services), len(mdl.Edges), len(mdl.Endpoints), mdl.Metadata.Confidence)
	return ExitOK
}

func (r Runner) runValidate(args []string) int {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	input := fs.String("input", "", "Path to Bering model JSON")

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
		r.printfErr("read model file: %v\n", err)
		return ExitError
	}

	if err := schema.ValidateJSON(raw); err != nil {
		r.printfErr("contract validation failed: %v\n", err)
		return ExitError
	}
	if _, err := model.ParseJSON(raw); err != nil {
		r.printfErr("semantic validation failed: %v\n", err)
		return ExitError
	}

	r.printf("model is valid: %s\n", *input)
	return ExitOK
}

func (r Runner) printUsage() {
	fmt.Fprintln(r.stdout, "Bering CLI")
	fmt.Fprintln(r.stdout)
	fmt.Fprintln(r.stdout, "Usage:")
	fmt.Fprintln(r.stdout, "  bering discover --input <trace-file|dir> [--out bering-model.json] [--replicas replicas.yaml|json] [--discovered-at RFC3339]")
	fmt.Fprintln(r.stdout, "  bering validate --input <bering-model.json>")
}

func (r Runner) printf(format string, args ...any) {
	fmt.Fprintf(r.stdout, format, args...)
}

func (r Runner) printfErr(format string, args ...any) {
	fmt.Fprintf(r.stderr, format, args...)
}
