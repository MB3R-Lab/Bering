package runtime

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/MB3R-Lab/Bering/internal/atomicfile"
	"github.com/MB3R-Lab/Bering/internal/config"
	"github.com/MB3R-Lab/Bering/internal/connectors/ebpf"
	"github.com/MB3R-Lab/Bering/internal/connectors/otlp"
)

var sourceIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
var captureSequence atomic.Uint64

func validSourceID(id string) bool { return sourceIDPattern.MatchString(id) }

func (s *Service) handleEBPF(source config.SourceConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		defer r.Body.Close()
		body, err := io.ReadAll(io.LimitReader(r.Body, s.cfg.Server.MaxRequestBytes+1))
		if err != nil || int64(len(body)) > s.cfg.Server.MaxRequestBytes {
			http.Error(w, "invalid or oversized eBPF export", http.StatusBadRequest)
			return
		}
		// Preserve the actual wire payload. Decoding and model construction stay
		// inside Bering; a capture can be replayed through the same connector.
		r.Body = io.NopCloser(bytes.NewReader(body))
		spans, err := otlp.DecodeHTTPRequest(r, s.cfg.Server.MaxRequestBytes)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		ref := "bering://source/" + source.ID
		if source.CaptureDirectory != "" {
			ext := ".pb"
			if strings.Contains(strings.ToLower(r.Header.Get("Content-Type")), "json") {
				ext = ".json"
			}
			if strings.EqualFold(r.Header.Get("Content-Encoding"), "gzip") {
				ext += ".gz"
			}
			name := fmt.Sprintf("%020d-%08d-%x%s", time.Now().UnixNano(), captureSequence.Add(1), sha256.Sum256(body), ext)
			ref = filepath.Join(source.CaptureDirectory, name)
			if err := atomicfile.WriteFile(ref, body, 0o600); err != nil {
				http.Error(w, "cannot retain configured eBPF capture", http.StatusInternalServerError)
				return
			}
		}
		observations, err := ebpf.Normalize(spans, ebpf.Options{Collector: source.Collector, Domain: source.Domain}, ref)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := s.engine.IngestWithSource(r.Context(), observations.Spans, time.Now().UTC(), observations.Sources[0]); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := writeOTLPResponse(w, r); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}
