// Package ebpf loads OpenTelemetry spans from a declared eBPF OTLP source.
package ebpf

import (
	"bufio"
	"compress/gzip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/MB3R-Lab/Bering/internal/connectors/otlp"
	"github.com/MB3R-Lab/Bering/internal/connectors/traces"
	"github.com/MB3R-Lab/Bering/internal/snapshot"
)

const (
	maxJSONLRecordBytes = 32 << 20
	maxDocumentBytes    = 256 << 20
)

var (
	domainPattern   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
	sourceIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
)

type Options struct {
	Type     string
	SourceID string
	Domain   string
}

type Result struct {
	Spans   []traces.Span
	Sources []snapshot.SourceSummary
}

func NormalizedOptions(opts Options) (Options, error) {
	opts.Type = strings.TrimSpace(opts.Type)
	if opts.Type == "" {
		opts.Type = "ebpf-obi"
	}
	if opts.Type != "ebpf-obi" && opts.Type != "ebpf-otlp" {
		return Options{}, fmt.Errorf("invalid eBPF source type %q", opts.Type)
	}
	opts.SourceID = strings.TrimSpace(opts.SourceID)
	if opts.Type == "ebpf-otlp" {
		if opts.SourceID == "" {
			opts.SourceID = "default"
		}
		if !sourceIDPattern.MatchString(opts.SourceID) {
			return Options{}, fmt.Errorf("invalid eBPF source id %q", opts.SourceID)
		}
	}
	opts.Domain = strings.TrimSpace(opts.Domain)
	if opts.Domain == "" {
		opts.Domain = "default"
	}
	if !domainPattern.MatchString(opts.Domain) {
		return Options{}, fmt.Errorf("invalid eBPF collection domain %q", opts.Domain)
	}
	return opts, nil
}

func sourceDomain(opts Options) string {
	if opts.Type == "ebpf-obi" {
		return "ebpf:" + opts.Domain
	}
	// Length-prefix the logical source ID so IDs and domains containing the same
	// delimiter remain injective. The prefix is disjoint from legacy ebpf-obi.
	return fmt.Sprintf("ebpf-otlp:%d:%s:%s", len(opts.SourceID), opts.SourceID, opts.Domain)
}

// Normalize annotates only declared source provenance. It does not modify IDs,
// scope, resource attributes, service identity, host or process attributes.
func Normalize(spans []traces.Span, opts Options, sourceRef string) (Result, error) {
	opts, err := NormalizedOptions(opts)
	if err != nil {
		return Result{}, err
	}
	sourceRef = strings.TrimSpace(sourceRef)
	if sourceRef == "" {
		return Result{}, fmt.Errorf("eBPF source reference is required")
	}
	out := make([]traces.Span, len(spans))
	copy(out, spans)
	connector := "obi"
	if opts.Type == "ebpf-otlp" {
		connector = "ebpf-otlp"
	}
	for i := range out {
		out[i].SourceRef = sourceRef
		out[i].SourceKind = "ebpf"
		out[i].SourceConnector = connector
		out[i].SourceDomain = sourceDomain(opts)
	}
	return Result{
		Spans: out,
		Sources: []snapshot.SourceSummary{{
			Type: "ebpf", Connector: connector, Ref: sourceRef,
			Observations: len(out),
		}},
	}, nil
}

// Load accepts bare OTLP JSON, newline-delimited OTLP export requests, and
// binary OTLP protobuf. Directories are traversed in lexical path order.
func Load(path string, opts Options) (Result, error) {
	if _, err := NormalizedOptions(opts); err != nil {
		return Result{}, err
	}
	paths, err := inputFiles(path)
	if err != nil {
		return Result{}, err
	}
	var result Result
	for _, file := range paths {
		spans, err := loadFile(file)
		if err != nil {
			return Result{}, fmt.Errorf("load eBPF source %q: %w", file, err)
		}
		part, err := Normalize(spans, opts, file)
		if err != nil {
			return Result{}, err
		}
		result.Spans = append(result.Spans, part.Spans...)
		result.Sources = append(result.Sources, part.Sources...)
	}
	return result, nil
}

func inputFiles(path string) ([]string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("eBPF input path is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve eBPF input path: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("stat eBPF input: %w", err)
	}
	if !info.IsDir() {
		if !supportedExtension(abs) {
			return nil, fmt.Errorf("unsupported eBPF source extension %q", filepath.Ext(abs))
		}
		return []string{abs}, nil
	}
	files := []string{}
	err = filepath.WalkDir(abs, func(file string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && supportedExtension(file) {
			files = append(files, file)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk eBPF input: %w", err)
	}
	sort.Strings(files)
	if len(files) == 0 {
		return nil, fmt.Errorf("no OTLP .json, .jsonl, .ndjson, .pb or gzip variants under %q", abs)
	}
	return files, nil
}

func supportedExtension(path string) bool {
	_, ok := payloadFormat(path)
	return ok
}

func payloadFormat(path string) (string, bool) {
	lower := strings.ToLower(path)
	if strings.HasSuffix(lower, ".gz") {
		lower = strings.TrimSuffix(lower, ".gz")
	}
	switch filepath.Ext(lower) {
	case ".json", ".jsonl", ".ndjson", ".pb":
		return filepath.Ext(lower), true
	default:
		return "", false
	}
}

func loadFile(path string) ([]traces.Span, error) {
	return loadFileLimited(path, maxDocumentBytes, maxJSONLRecordBytes)
}

func loadFileLimited(path string, documentLimit, recordLimit int) ([]traces.Span, error) {
	format, ok := payloadFormat(path)
	if !ok {
		return nil, fmt.Errorf("unsupported eBPF source extension %q", filepath.Ext(path))
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var reader io.Reader = file
	if strings.EqualFold(filepath.Ext(path), ".gz") {
		compressed, err := gzip.NewReader(file)
		if err != nil {
			return nil, fmt.Errorf("open gzip stream: %w", err)
		}
		defer compressed.Close()
		reader = compressed
	}
	switch format {
	case ".jsonl", ".ndjson":
		return loadJSONL(reader, recordLimit)
	case ".json", ".pb":
		body, err := io.ReadAll(io.LimitReader(reader, int64(documentLimit)+1))
		if err != nil {
			return nil, fmt.Errorf("read OTLP document: %w", err)
		}
		if len(body) > documentLimit {
			return nil, fmt.Errorf("decompressed OTLP document exceeds %d bytes", documentLimit)
		}
		contentType := "application/x-protobuf"
		if format == ".json" {
			contentType = "application/json"
		}
		return otlp.DecodePayload(contentType, body)
	default:
		panic("payloadFormat returned unsupported extension")
	}
}

func loadJSONL(reader io.Reader, recordLimit int) ([]traces.Span, error) {
	scanner := bufio.NewScanner(reader)
	initial := 64 * 1024
	if recordLimit < initial {
		initial = recordLimit
	}
	scanner.Buffer(make([]byte, initial), recordLimit)
	spans := []traces.Span{}
	line := 0
	for scanner.Scan() {
		line++
		body := strings.TrimSpace(scanner.Text())
		if body == "" {
			continue
		}
		part, err := otlp.DecodePayload("application/json", []byte(body))
		if err != nil {
			return nil, fmt.Errorf("JSONL record %d: %w", line, err)
		}
		spans = append(spans, part...)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("JSONL record %d (limit %d bytes): %w", line+1, recordLimit, err)
	}
	return spans, nil
}
