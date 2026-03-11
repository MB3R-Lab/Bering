package runtime

import (
	"net/http"
	"time"

	"github.com/MB3R-Lab/Bering/internal/snapshot"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metrics struct {
	registry *prometheus.Registry

	spansIngested         prometheus.Counter
	spansDropped          prometheus.Counter
	snapshotsEmitted      prometheus.Counter
	snapshotBuildDuration prometheus.Histogram
	currentServices       prometheus.Gauge
	currentEdges          prometheus.Gauge
	currentEndpoints      prometheus.Gauge
	windowLagSeconds      prometheus.Gauge
	lastSnapshotUnix      prometheus.Gauge
	snapshotAgeSeconds    prometheus.Gauge
	diffAddedServices     prometheus.Gauge
	diffRemovedServices   prometheus.Gauge
	diffAddedEdges        prometheus.Gauge
	diffRemovedEdges      prometheus.Gauge
	diffAddedEndpoints    prometheus.Gauge
	diffRemovedEndpoints  prometheus.Gauge
	diffChangedServices   prometheus.Gauge
	diffChangedEdges      prometheus.Gauge
	diffChangedEndpoints  prometheus.Gauge
}

func NewMetrics() *Metrics {
	registry := prometheus.NewRegistry()
	m := &Metrics{
		registry:              registry,
		spansIngested:         prometheus.NewCounter(prometheus.CounterOpts{Name: "spans_ingested_total", Help: "Total spans accepted by the runtime ingest path."}),
		spansDropped:          prometheus.NewCounter(prometheus.CounterOpts{Name: "spans_dropped_total", Help: "Total spans dropped due to late policy, buffer pressure, or decode rejection."}),
		snapshotsEmitted:      prometheus.NewCounter(prometheus.CounterOpts{Name: "snapshots_emitted_total", Help: "Total discovery snapshots written to configured sinks."}),
		snapshotBuildDuration: prometheus.NewHistogram(prometheus.HistogramOpts{Name: "snapshot_build_duration_seconds", Help: "Time spent building and writing one snapshot.", Buckets: prometheus.DefBuckets}),
		currentServices:       prometheus.NewGauge(prometheus.GaugeOpts{Name: "current_services", Help: "Service count in the latest emitted snapshot."}),
		currentEdges:          prometheus.NewGauge(prometheus.GaugeOpts{Name: "current_edges", Help: "Edge count in the latest emitted snapshot."}),
		currentEndpoints:      prometheus.NewGauge(prometheus.GaugeOpts{Name: "current_endpoints", Help: "Endpoint count in the latest emitted snapshot."}),
		windowLagSeconds:      prometheus.NewGauge(prometheus.GaugeOpts{Name: "window_lag_seconds", Help: "How far the runtime is behind the active window end when flushing."}),
		lastSnapshotUnix:      prometheus.NewGauge(prometheus.GaugeOpts{Name: "last_snapshot_unixtime", Help: "Unix timestamp of the latest emitted snapshot."}),
		snapshotAgeSeconds:    prometheus.NewGauge(prometheus.GaugeOpts{Name: "snapshot_age_seconds", Help: "Age in seconds of the latest emitted snapshot."}),
		diffAddedServices:     prometheus.NewGauge(prometheus.GaugeOpts{Name: "diff_added_services", Help: "Services added in the latest snapshot diff."}),
		diffRemovedServices:   prometheus.NewGauge(prometheus.GaugeOpts{Name: "diff_removed_services", Help: "Services removed in the latest snapshot diff."}),
		diffAddedEdges:        prometheus.NewGauge(prometheus.GaugeOpts{Name: "diff_added_edges", Help: "Edges added in the latest snapshot diff."}),
		diffRemovedEdges:      prometheus.NewGauge(prometheus.GaugeOpts{Name: "diff_removed_edges", Help: "Edges removed in the latest snapshot diff."}),
		diffAddedEndpoints:    prometheus.NewGauge(prometheus.GaugeOpts{Name: "diff_added_endpoints", Help: "Endpoints added in the latest snapshot diff."}),
		diffRemovedEndpoints:  prometheus.NewGauge(prometheus.GaugeOpts{Name: "diff_removed_endpoints", Help: "Endpoints removed in the latest snapshot diff."}),
		diffChangedServices:   prometheus.NewGauge(prometheus.GaugeOpts{Name: "diff_changed_services", Help: "Services changed in the latest snapshot diff."}),
		diffChangedEdges:      prometheus.NewGauge(prometheus.GaugeOpts{Name: "diff_changed_edges", Help: "Edges changed in the latest snapshot diff."}),
		diffChangedEndpoints:  prometheus.NewGauge(prometheus.GaugeOpts{Name: "diff_changed_endpoints", Help: "Endpoints changed in the latest snapshot diff."}),
	}
	registry.MustRegister(
		m.spansIngested,
		m.spansDropped,
		m.snapshotsEmitted,
		m.snapshotBuildDuration,
		m.currentServices,
		m.currentEdges,
		m.currentEndpoints,
		m.windowLagSeconds,
		m.lastSnapshotUnix,
		m.snapshotAgeSeconds,
		m.diffAddedServices,
		m.diffRemovedServices,
		m.diffAddedEdges,
		m.diffRemovedEdges,
		m.diffAddedEndpoints,
		m.diffRemovedEndpoints,
		m.diffChangedServices,
		m.diffChangedEdges,
		m.diffChangedEndpoints,
	)
	return m
}

func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{EnableOpenMetrics: true})
}

func (m *Metrics) AddIngested(count int) {
	if count > 0 {
		m.spansIngested.Add(float64(count))
	}
}

func (m *Metrics) AddDropped(count int) {
	if count > 0 {
		m.spansDropped.Add(float64(count))
	}
}

func (m *Metrics) SetWindowLag(now, end time.Time) {
	if now.IsZero() || end.IsZero() || !now.After(end) {
		m.windowLagSeconds.Set(0)
		return
	}
	m.windowLagSeconds.Set(now.Sub(end).Seconds())
}

func (m *Metrics) RecordSnapshot(env snapshot.Envelope, buildDuration time.Duration, now time.Time) {
	m.snapshotsEmitted.Inc()
	m.snapshotBuildDuration.Observe(buildDuration.Seconds())
	m.currentServices.Set(float64(env.Counts.Services))
	m.currentEdges.Set(float64(env.Counts.Edges))
	m.currentEndpoints.Set(float64(env.Counts.Endpoints))
	if emittedAt, err := time.Parse(time.RFC3339, env.Metadata.EmittedAt); err == nil {
		m.lastSnapshotUnix.Set(float64(emittedAt.Unix()))
		if !now.IsZero() {
			m.snapshotAgeSeconds.Set(now.Sub(emittedAt).Seconds())
		}
	}
	m.diffAddedServices.Set(float64(env.Diff.AddedServices))
	m.diffRemovedServices.Set(float64(env.Diff.RemovedServices))
	m.diffChangedServices.Set(float64(env.Diff.ChangedServices))
	m.diffAddedEdges.Set(float64(env.Diff.AddedEdges))
	m.diffRemovedEdges.Set(float64(env.Diff.RemovedEdges))
	m.diffChangedEdges.Set(float64(env.Diff.ChangedEdges))
	m.diffAddedEndpoints.Set(float64(env.Diff.AddedEndpoints))
	m.diffRemovedEndpoints.Set(float64(env.Diff.RemovedEndpoints))
	m.diffChangedEndpoints.Set(float64(env.Diff.ChangedEndpoints))
}
