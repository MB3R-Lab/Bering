package reconciliation

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/MB3R-Lab/Bering/internal/jsoncanon"
	"github.com/MB3R-Lab/Bering/internal/model"
)

func loadState(path string) (persistedState, []string, error) {
	if path == "" {
		return newPersistedState(), nil, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return newPersistedState(), nil, nil
		}
		return persistedState{}, nil, fmt.Errorf("read reconciliation state: %w", err)
	}
	var state persistedState
	if err := json.Unmarshal(raw, &state); err != nil {
		return newPersistedState(), []string{fmt.Sprintf("ignored corrupt reconciliation state at %s: %v", path, err)}, nil
	}
	if state.FormatVersion == 0 {
		state.FormatVersion = defaultFormatVersion
	}
	if state.Services == nil {
		state.Services = map[string]*entityState{}
	}
	if state.Edges == nil {
		state.Edges = map[string]*entityState{}
	}
	if state.Endpoints == nil {
		state.Endpoints = map[string]*entityState{}
	}
	return state, nil, nil
}

func saveState(path string, state persistedState) error {
	if path == "" {
		return nil
	}
	state.FormatVersion = defaultFormatVersion
	raw, err := jsoncanon.MarshalIndent(state)
	if err != nil {
		return fmt.Errorf("marshal reconciliation state: %w", err)
	}
	if err := writeAtomic(path, raw); err != nil {
		return fmt.Errorf("write reconciliation state: %w", err)
	}
	return nil
}

func WriteReport(path string, report Report) error {
	if path == "" {
		return nil
	}
	raw, err := jsoncanon.MarshalIndent(report)
	if err != nil {
		return fmt.Errorf("marshal reconciliation report: %w", err)
	}
	if err := writeAtomic(path, raw); err != nil {
		return fmt.Errorf("write reconciliation report: %w", err)
	}
	return nil
}

func writeAtomic(path string, raw []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if _, err := temp.Write(raw); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	_ = os.Remove(path)
	return os.Rename(tempPath, path)
}

type topologyShape struct {
	Services  []model.Service  `json:"services"`
	Edges     []model.Edge     `json:"edges"`
	Endpoints []model.Endpoint `json:"endpoints"`
}

func topologyFingerprint(mdl model.ResilienceModel) (string, error) {
	shape := topologyShape{
		Services:  append([]model.Service(nil), mdl.Services...),
		Edges:     append([]model.Edge(nil), mdl.Edges...),
		Endpoints: append([]model.Endpoint(nil), mdl.Endpoints...),
	}
	sort.Slice(shape.Services, func(i, j int) bool { return shape.Services[i].ID < shape.Services[j].ID })
	sort.Slice(shape.Edges, func(i, j int) bool { return edgeIdentity(shape.Edges[i]) < edgeIdentity(shape.Edges[j]) })
	sort.Slice(shape.Endpoints, func(i, j int) bool { return shape.Endpoints[i].ID < shape.Endpoints[j].ID })
	raw, err := jsoncanon.MarshalIndent(shape)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return "sha256:" + fmt.Sprintf("%x", sum[:]), nil
}

func trimRetiredBucket(bucket map[string]*entityState, maxRetired int) int {
	if maxRetired <= 0 {
		return 0
	}
	retired := retiredEntries(bucket)
	if len(retired) <= maxRetired {
		return 0
	}
	sort.Slice(retired, func(i, j int) bool {
		return retired[i].retiredAt.Before(retired[j].retiredAt)
	})
	trim := len(retired) - maxRetired
	for i := 0; i < trim; i++ {
		delete(bucket, retired[i].id)
	}
	return trim
}

func trimRetiredOverall(buckets []map[string]*entityState, trim int) int {
	if trim <= 0 {
		return 0
	}
	type retiredRef struct {
		bucket    map[string]*entityState
		id        string
		retiredAt time.Time
	}
	all := make([]retiredRef, 0)
	for _, bucket := range buckets {
		for _, item := range retiredEntries(bucket) {
			all = append(all, retiredRef{bucket: bucket, id: item.id, retiredAt: item.retiredAt})
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].retiredAt.Before(all[j].retiredAt) })
	if trim > len(all) {
		trim = len(all)
	}
	for i := 0; i < trim; i++ {
		delete(all[i].bucket, all[i].id)
	}
	return trim
}

type retiredEntry struct {
	id        string
	retiredAt time.Time
}

func retiredEntries(bucket map[string]*entityState) []retiredEntry {
	entries := make([]retiredEntry, 0)
	for id, state := range bucket {
		if state.Lifecycle != LifecycleRetired || state.RetiredAt == "" {
			continue
		}
		retiredAt, err := time.Parse(time.RFC3339, state.RetiredAt)
		if err != nil {
			retiredAt = time.Unix(0, 0)
		}
		entries = append(entries, retiredEntry{id: id, retiredAt: retiredAt})
	}
	return entries
}

func retiredCount(bucket map[string]*entityState) int {
	return len(retiredEntries(bucket))
}
