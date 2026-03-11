package discovery

import (
	"fmt"
	"strings"
	"time"

	"github.com/MB3R-Lab/Bering/internal/connectors/topology"
	"github.com/MB3R-Lab/Bering/internal/model"
	"github.com/MB3R-Lab/Bering/internal/overlay"
	"github.com/MB3R-Lab/Bering/internal/schema"
	"github.com/MB3R-Lab/Bering/internal/snapshot"
)

func DiscoverTopology(doc topology.Document, opts Options) (Result, error) {
	discoveredAt := strings.TrimSpace(opts.DiscoveredAt)
	if discoveredAt == "" {
		discoveredAt = time.Now().UTC().Format(time.RFC3339)
	}
	if _, err := time.Parse(time.RFC3339, discoveredAt); err != nil {
		return Result{}, fmt.Errorf("discovered_at must be RFC3339: %w", err)
	}

	sourceRef := strings.TrimSpace(opts.SourceRef)
	if sourceRef == "" {
		sourceRef = "bering://discover"
	}
	sourceType := strings.TrimSpace(doc.Source.Type)
	if sourceType == "" {
		sourceType = topology.SourceTypeTopologyAPI
	}
	provenanceRef := strings.TrimSpace(doc.Source.Ref)
	if provenanceRef == "" {
		provenanceRef = sourceRef
	}

	allOverlays := make([]overlay.File, 0, len(opts.Overlays)+1)
	if len(opts.ReplicasOverride) > 0 {
		allOverlays = append(allOverlays, overlay.FromReplicas(opts.ReplicasOverride, "legacy:replicas"))
	}
	allOverlays = append(allOverlays, opts.Overlays...)

	serviceReplicas := make(map[string]int, len(doc.Services))
	for _, item := range doc.Services {
		serviceReplicas[item.ID] = valueOrDefault(item.Replicas, 1)
	}
	if err := applyReplicaOverlays(serviceReplicas, allOverlays); err != nil {
		return Result{}, err
	}

	provenance := snapshot.Provenance{Type: sourceType, Connector: topology.ConnectorName, Ref: provenanceRef}
	services := make([]model.Service, 0, len(doc.Services))
	serviceRecords := make([]snapshot.ServiceRecord, 0, len(doc.Services))
	for _, item := range doc.Services {
		replicas := serviceReplicas[item.ID]
		services = append(services, model.Service{ID: item.ID, Name: item.Name, Replicas: replicas})
		serviceRecords = append(serviceRecords, snapshot.ServiceRecord{
			ID:         item.ID,
			Name:       item.Name,
			Replicas:   replicas,
			Support:    topologySupport(item.Support),
			FirstSeen:  strings.TrimSpace(item.FirstSeen),
			LastSeen:   strings.TrimSpace(item.LastSeen),
			Provenance: []snapshot.Provenance{provenance},
			Metadata: snapshot.ServiceMetadata{
				CommonMetadata: snapshot.CommonMetadata{
					Labels:  cloneStringMap(item.Labels),
					Tags:    append([]string(nil), item.Tags...),
					SLORefs: append([]string(nil), item.SLORefs...),
				},
				FailureEligible: item.FailureEligible,
				Attributes:      cloneStringMap(item.Attributes),
			},
		})
	}

	edges := make([]model.Edge, 0, len(doc.Edges))
	edgeRecords := make([]snapshot.EdgeRecord, 0, len(doc.Edges))
	for _, item := range doc.Edges {
		kind := model.EdgeKind(strings.TrimSpace(item.Kind))
		edges = append(edges, model.Edge{From: item.From, To: item.To, Kind: kind, Blocking: valueOrDefault(item.Blocking, kind == model.EdgeKindSync)})
		edgeRecords = append(edgeRecords, snapshot.EdgeRecord{
			ID:         item.ID,
			From:       item.From,
			To:         item.To,
			Kind:       kind,
			Blocking:   valueOrDefault(item.Blocking, kind == model.EdgeKindSync),
			Support:    topologySupport(item.Support),
			FirstSeen:  strings.TrimSpace(item.FirstSeen),
			LastSeen:   strings.TrimSpace(item.LastSeen),
			Provenance: []snapshot.Provenance{provenance},
			Metadata: snapshot.EdgeMetadata{
				CommonMetadata: snapshot.CommonMetadata{
					Labels:  cloneStringMap(item.Labels),
					Tags:    append([]string(nil), item.Tags...),
					SLORefs: append([]string(nil), item.SLORefs...),
				},
				Weight:     item.Weight,
				Attributes: cloneStringMap(item.Attributes),
			},
		})
	}

	endpoints := make([]model.Endpoint, 0, len(doc.Endpoints))
	endpointRecords := make([]snapshot.EndpointRecord, 0, len(doc.Endpoints))
	for _, item := range doc.Endpoints {
		predicateRef := strings.TrimSpace(item.PredicateRef)
		if predicateRef == "" {
			predicateRef = item.ID
		}
		endpoints = append(endpoints, model.Endpoint{
			ID:                  item.ID,
			EntryService:        item.EntryService,
			SuccessPredicateRef: predicateRef,
		})
		endpointRecords = append(endpointRecords, snapshot.EndpointRecord{
			ID:           item.ID,
			EntryService: item.EntryService,
			Method:       strings.TrimSpace(item.Method),
			Path:         strings.TrimSpace(item.Path),
			Support:      topologySupport(item.Support),
			FirstSeen:    strings.TrimSpace(item.FirstSeen),
			LastSeen:     strings.TrimSpace(item.LastSeen),
			Provenance:   []snapshot.Provenance{provenance},
			Metadata: snapshot.EndpointMetadata{
				CommonMetadata: snapshot.CommonMetadata{
					Labels:  cloneStringMap(item.Labels),
					Tags:    append([]string(nil), item.Tags...),
					SLORefs: append([]string(nil), item.SLORefs...),
				},
				Weight:       item.Weight,
				PredicateRef: predicateRef,
				Attributes:   cloneStringMap(item.Attributes),
			},
		})
	}

	confidence := 1.0
	mdl := model.ResilienceModel{
		Services:  services,
		Edges:     edges,
		Endpoints: endpoints,
		Metadata: model.Metadata{
			SourceType:   SourceTypeBering,
			SourceRef:    sourceRef,
			DiscoveredAt: discoveredAt,
			Confidence:   confidence,
			Schema: model.SchemaRef{
				Name:    schema.ExpectedSchemaName,
				Version: schema.ExpectedSchemaVersion,
				URI:     schema.ExpectedSchemaURI,
				Digest:  schema.ExpectedSchemaDigest,
			},
		},
	}
	if err := mdl.ValidateSemantic(); err != nil {
		return Result{}, err
	}
	mdl.SortDeterministic()

	applications, err := applyOverlays(&mdl, serviceRecords, edgeRecords, endpointRecords, allOverlays)
	if err != nil {
		return Result{}, err
	}
	mdl.SortDeterministic()
	serviceRecords = rebuildServiceRecords(serviceRecords, mdl.Services)

	coverage := snapshot.CoverageSummary{
		Confidence:         confidence,
		ServiceSupportMin:  minSupportServices(serviceRecords),
		EdgeSupportMin:     minSupportEdges(edgeRecords),
		EndpointSupportMin: minSupportEndpoints(endpointRecords),
	}

	result := Result{
		Model: mdl,
		Discovery: snapshot.DiscoveryDetails{
			Services:  serviceRecords,
			Edges:     edgeRecords,
			Endpoints: endpointRecords,
			Overlays:  applications,
		},
		Sources: []snapshot.SourceSummary{{
			Type:         sourceType,
			Connector:    topology.ConnectorName,
			Ref:          provenanceRef,
			Observations: len(doc.Services) + len(doc.Edges) + len(doc.Endpoints),
		}},
		Coverage: coverage,
	}
	return result, nil
}

func topologySupport(item topology.Support) snapshot.SupportSummary {
	return snapshot.SupportSummary{
		Observations: item.Observations,
		TraceCount:   0,
		Evidence:     append([]string(nil), item.Evidence...),
	}
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func valueOrDefault[T any](value *T, fallback T) T {
	if value == nil {
		return fallback
	}
	return *value
}
