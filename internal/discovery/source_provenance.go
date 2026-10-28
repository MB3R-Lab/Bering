package discovery

import (
	"encoding/json"
	"sort"

	"github.com/MB3R-Lab/Bering/internal/connectors/traces"
	"github.com/MB3R-Lab/Bering/internal/evidence"
	"github.com/MB3R-Lab/Bering/internal/snapshot"
)

func applySourceProvenance(details *snapshot.DiscoveryDetails, spans []traces.Span, index map[string]traces.Span, opts Options, fallback string) {
	services := map[string][]snapshot.Provenance{}
	endpoints := map[string][]snapshot.Provenance{}
	convert := func(source evidence.Source) snapshot.Provenance {
		kind, connector := source.Kind, inferConnector(opts.Sources)
		if kind == "" {
			kind = "traces"
		}
		if kind == "ebpf" {
			connector = "obi"
		}
		return snapshot.Provenance{Type: kind, Connector: connector, Ref: source.Ref}
	}
	unique := func(items []snapshot.Provenance) []snapshot.Provenance {
		byKey := map[string]snapshot.Provenance{}
		keys := []string{}
		for _, item := range items {
			raw, _ := json.Marshal(item)
			key := string(raw)
			if _, exists := byKey[key]; !exists {
				keys = append(keys, key)
			}
			byKey[key] = item
		}
		sort.Strings(keys)
		out := make([]snapshot.Provenance, 0, len(keys))
		for _, key := range keys {
			out = append(out, byKey[key])
		}
		return out
	}
	for _, span := range spans {
		provenance := convert(observationSource(span, fallback))
		services[span.Service] = append(services[span.Service], provenance)
		parent, hasParent := parentSpan(span, index)
		if endpoint, ok := inferEndpoint(span, parent, hasParent); ok {
			endpoints[endpoint.ID] = append(endpoints[endpoint.ID], provenance)
		}
	}
	for i := range details.Services {
		if sources := services[details.Services[i].ID]; len(sources) > 0 {
			details.Services[i].Provenance = unique(sources)
		} else if meta := details.Services[i].Metadata; meta != nil {
			record, err := evidence.Decode(meta.Attributes[evidence.AttributeKey])
			if err != nil {
				continue
			}
			var sources []snapshot.Provenance
			if record.Database != nil {
				for _, source := range record.Database.Sources {
					sources = append(sources, convert(source))
				}
			}
			if record.Peer != nil {
				for _, source := range record.Peer.Sources {
					sources = append(sources, convert(source))
				}
			}
			if len(sources) > 0 {
				details.Services[i].Provenance = unique(sources)
			}
		}
	}
	for i := range details.Endpoints {
		if sources := endpoints[details.Endpoints[i].ID]; len(sources) > 0 {
			details.Endpoints[i].Provenance = unique(sources)
		}
	}
	for i := range details.Edges {
		meta := details.Edges[i].Metadata
		if meta == nil {
			continue
		}
		record, err := evidence.Decode(meta.Attributes[evidence.AttributeKey])
		if err != nil || record.Relation == nil {
			continue
		}
		var sources []snapshot.Provenance
		for _, source := range record.Relation.Sources {
			sources = append(sources, convert(source))
		}
		if len(sources) > 0 {
			details.Edges[i].Provenance = unique(sources)
		}
	}
}
