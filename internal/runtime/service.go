package runtime

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	collecttracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/MB3R-Lab/Bering/internal/config"
	"github.com/MB3R-Lab/Bering/internal/connectors/otlp"
	"github.com/MB3R-Lab/Bering/internal/discovery"
	"github.com/MB3R-Lab/Bering/internal/overlay"
	"github.com/MB3R-Lab/Bering/internal/snapshot"
)

type Service struct {
	cfg       config.ServeConfig
	logger    *slog.Logger
	metrics   *Metrics
	engine    *Engine
	server    *http.Server
	listener  net.Listener
	ready     atomic.Bool
}

func NewService(cfg config.ServeConfig, overlays []overlay.File, logger *slog.Logger) (*Service, error) {
	metrics := NewMetrics()
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(ioDiscard{}, nil))
	}
	engine, err := NewEngine(EngineConfig{
		WindowSize:       cfg.Runtime.WindowSize.Duration(),
		MaxInMemorySpans: cfg.Runtime.MaxInMemorySpans,
		LateSpanPolicy:   cfg.Runtime.LateSpanPolicy,
		Sink:             FileSink{Directory: cfg.Sink.Directory, LatestPath: cfg.Sink.LatestPath},
		Metrics:          metrics,
		Logger:           logger,
		SourceRef:        discovery.BuildServeSourceRef(cfg.Server.ListenAddress),
		Sources:          []snapshot.SourceSummary{{Type: "traces", Connector: otlp.ConnectorName, Ref: discovery.BuildServeSourceRef(cfg.Server.ListenAddress)}},
		Overlays:         overlays,
	})
	if err != nil {
		return nil, err
	}
	service := &Service{cfg: cfg, logger: logger, metrics: metrics, engine: engine}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/traces", service.handleOTLP)
	mux.HandleFunc("/healthz", service.handleHealth)
	mux.HandleFunc("/readyz", service.handleReady)
	mux.Handle("/metrics", metrics.Handler())
	service.server = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	return service, nil
}

func (s *Service) Addr() string {
	if s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}

func (s *Service) Run(ctx context.Context) error {
	listener, err := net.Listen("tcp", s.cfg.Server.ListenAddress)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", s.cfg.Server.ListenAddress, err)
	}
	s.listener = listener
	s.ready.Store(true)

	errCh := make(chan error, 1)
	go func() {
		ticker := time.NewTicker(s.cfg.Runtime.FlushInterval.Duration())
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := s.engine.FlushDue(context.Background()); err != nil {
					errCh <- err
					return
				}
			}
		}
	}()
	go func() {
		if err := s.server.Serve(listener); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	s.logger.Info("bering runtime service listening", slog.String("address", listener.Addr().String()))

	select {
	case <-ctx.Done():
		s.ready.Store(false)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("http shutdown: %w", err)
		}
		if err := s.engine.Close(shutdownCtx); err != nil {
			return fmt.Errorf("flush final snapshot: %w", err)
		}
		return nil
	case err := <-errCh:
		s.ready.Store(false)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.server.Shutdown(shutdownCtx)
		return err
	}
}

func (s *Service) handleOTLP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	defer r.Body.Close()
	spans, err := otlp.DecodeHTTPRequest(r, s.cfg.Server.MaxRequestBytes)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.engine.Ingest(r.Context(), spans, time.Now().UTC()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := writeOTLPResponse(w, r); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (s *Service) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

func (s *Service) handleReady(w http.ResponseWriter, _ *http.Request) {
	if !s.ready.Load() {
		http.Error(w, "not ready", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ready\n"))
}

func writeOTLPResponse(w http.ResponseWriter, r *http.Request) error {
	resp := &collecttracev1.ExportTraceServiceResponse{}
	if stringsContainsJSON(r.Header.Get("Content-Type")) {
		raw, err := protojson.Marshal(resp)
		if err != nil {
			return err
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, err = w.Write(raw)
		return err
	}
	raw, err := proto.Marshal(resp)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/x-protobuf")
	w.WriteHeader(http.StatusOK)
	_, err = w.Write(raw)
	return err
}

func stringsContainsJSON(contentType string) bool {
	return strings.Contains(strings.ToLower(contentType), "json")
}
