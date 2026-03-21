package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	collecttracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/MB3R-Lab/Bering/internal/config"
	"github.com/MB3R-Lab/Bering/internal/connectors/otlp"
	"github.com/MB3R-Lab/Bering/internal/discovery"
	"github.com/MB3R-Lab/Bering/internal/overlay"
	"github.com/MB3R-Lab/Bering/internal/snapshot"
)

type Service struct {
	cfg          config.ServeConfig
	logger       *slog.Logger
	metrics      *Metrics
	engine       *Engine
	httpServer   *http.Server
	httpListener net.Listener
	grpcServer   *grpc.Server
	grpcListener net.Listener
	ready        atomic.Bool
}

func NewService(cfg config.ServeConfig, overlays []overlay.File, logger *slog.Logger) (*Service, error) {
	metrics := NewMetrics()
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(ioDiscard{}, nil))
	}
	_, reportPath, rawWindowPath, stableCorePath := resolveReconciliationPaths(cfg)
	engine, err := NewEngine(EngineConfig{
		WindowSize:               cfg.Runtime.WindowSize.Duration(),
		MaxInMemorySpans:         cfg.Runtime.MaxInMemorySpans,
		LateSpanPolicy:           cfg.Runtime.LateSpanPolicy,
		Sink:                     FileSink{Directory: cfg.Sink.Directory, LatestPath: cfg.Sink.LatestPath},
		Metrics:                  metrics,
		Logger:                   logger,
		SourceRef:                discovery.BuildServeSourceRef(cfg.Server.ListenAddress),
		Sources:                  []snapshot.SourceSummary{{Type: "traces", Connector: otlp.HTTPConnectorName, Ref: discovery.BuildServeSourceRef(cfg.Server.ListenAddress)}},
		Overlays:                 overlays,
		ReconciliationConfig:     buildRuntimeReconciliationConfig(cfg),
		ReconciliationReportPath: reportPath,
		RawWindowPath:            rawWindowPath,
		StableCorePath:           stableCorePath,
	})
	if err != nil {
		return nil, err
	}
	service := &Service{cfg: cfg, logger: logger, metrics: metrics, engine: engine}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/traces", service.handleOTLP)
	mux.HandleFunc("/healthz", service.handleHealth)
	mux.HandleFunc("/readyz", service.handleReady)
	mux.HandleFunc("/reconciliation/report", service.handleReconciliationReport)
	mux.Handle("/metrics", metrics.Handler())
	service.httpServer = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	if strings.TrimSpace(cfg.Server.GRPCListenAddress) != "" {
		service.grpcServer = grpc.NewServer(grpc.MaxRecvMsgSize(grpcMaxRecvMsgSize(cfg.Server.MaxRequestBytes)))
		collecttracev1.RegisterTraceServiceServer(service.grpcServer, grpcTraceService{service: service})
	}
	return service, nil
}

func (s *Service) Addr() string {
	if s.httpListener == nil {
		return ""
	}
	return s.httpListener.Addr().String()
}

func (s *Service) GRPCAddr() string {
	if s.grpcListener == nil {
		return ""
	}
	return s.grpcListener.Addr().String()
}

func (s *Service) Run(ctx context.Context) error {
	httpListener, err := net.Listen("tcp", s.cfg.Server.ListenAddress)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", s.cfg.Server.ListenAddress, err)
	}
	s.httpListener = httpListener
	if s.grpcServer != nil {
		grpcListener, err := net.Listen("tcp", s.cfg.Server.GRPCListenAddress)
		if err != nil {
			_ = httpListener.Close()
			return fmt.Errorf("listen on %s: %w", s.cfg.Server.GRPCListenAddress, err)
		}
		s.grpcListener = grpcListener
	}
	s.ready.Store(true)

	errCh := make(chan error, 1)
	var workers sync.WaitGroup
	workers.Add(1)
	go func() {
		defer workers.Done()
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
	workers.Add(1)
	go func() {
		defer workers.Done()
		if err := s.httpServer.Serve(httpListener); err != nil && err != http.ErrServerClosed {
			sendServeError(errCh, err)
		}
	}()
	if s.grpcServer != nil {
		workers.Add(1)
		go func() {
			defer workers.Done()
			if err := s.grpcServer.Serve(s.grpcListener); err != nil {
				sendServeError(errCh, err)
			}
		}()
	}

	s.logger.Info("bering runtime HTTP service listening", slog.String("address", httpListener.Addr().String()))
	if s.grpcListener != nil {
		s.logger.Info("bering runtime gRPC service listening", slog.String("address", s.grpcListener.Addr().String()))
	}

	select {
	case <-ctx.Done():
		s.ready.Store(false)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("http shutdown: %w", err)
		}
		if err := stopGRPCServer(shutdownCtx, s.grpcServer); err != nil {
			return fmt.Errorf("grpc shutdown: %w", err)
		}
		workers.Wait()
		if err := s.engine.Close(shutdownCtx); err != nil {
			return fmt.Errorf("flush final snapshot: %w", err)
		}
		return nil
	case err := <-errCh:
		s.ready.Store(false)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.httpServer.Shutdown(shutdownCtx)
		_ = stopGRPCServer(shutdownCtx, s.grpcServer)
		workers.Wait()
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
	if err := s.engine.IngestWithSource(r.Context(), spans, time.Now().UTC(), snapshot.SourceSummary{
		Type:      "traces",
		Connector: otlp.HTTPConnectorName,
		Ref:       discovery.BuildServeSourceRef(s.cfg.Server.ListenAddress),
	}); err != nil {
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

func (s *Service) handleReconciliationReport(w http.ResponseWriter, _ *http.Request) {
	report, ok := s.engine.LatestReconciliationReport()
	if !ok {
		http.Error(w, "reconciliation report not available", http.StatusNotFound)
		return
	}
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(raw)
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

type grpcTraceService struct {
	collecttracev1.UnimplementedTraceServiceServer
	service *Service
}

func (g grpcTraceService) Export(ctx context.Context, req *collecttracev1.ExportTraceServiceRequest) (*collecttracev1.ExportTraceServiceResponse, error) {
	spans := otlp.NormalizeRequest(req)
	if err := g.service.engine.IngestWithSource(ctx, spans, time.Now().UTC(), snapshot.SourceSummary{
		Type:      "traces",
		Connector: otlp.GRPCConnectorName,
		Ref:       discovery.BuildServeSourceRef(g.service.cfg.Server.GRPCListenAddress),
	}); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &collecttracev1.ExportTraceServiceResponse{}, nil
}

func grpcMaxRecvMsgSize(maxBytes int64) int {
	maxInt := int(^uint(0) >> 1)
	if maxBytes <= 0 {
		return maxInt
	}
	if maxBytes > int64(maxInt) {
		return maxInt
	}
	return int(maxBytes)
}

func sendServeError(errCh chan<- error, err error) {
	if err == nil {
		return
	}
	select {
	case errCh <- err:
	default:
	}
}

func stopGRPCServer(ctx context.Context, server *grpc.Server) error {
	if server == nil {
		return nil
	}
	done := make(chan struct{})
	go func() {
		server.GracefulStop()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		server.Stop()
		<-done
		return ctx.Err()
	}
}
