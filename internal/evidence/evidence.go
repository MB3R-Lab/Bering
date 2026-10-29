// Package evidence defines the versioned observation-evidence value stored in
// snapshot discovery metadata attributes.
package evidence

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

const (
	AttributeKey  = "bering.observation_evidence.v1"
	SchemaVersion = "1.0.0"
)

type Source struct {
	Kind         string `json:"kind,omitempty"`
	Connector    string `json:"connector,omitempty"`
	Domain       string `json:"domain,omitempty"`
	Ref          string `json:"ref,omitempty"`
	ScopeName    string `json:"scope_name,omitempty"`
	ScopeVersion string `json:"scope_version,omitempty"`
}

type Peer struct {
	Service      string   `json:"service,omitempty"`
	Address      string   `json:"address,omitempty"`
	Port         *int     `json:"port,omitempty"`
	NetworkScope string   `json:"network_scope,omitempty"`
	IdentityKind string   `json:"identity_kind"`
	Sources      []Source `json:"sources,omitempty"`
}

type Instance struct {
	ID             string            `json:"id"`
	IdentityStatus string            `json:"identity_status,omitempty"`
	Attributes     map[string]string `json:"attributes,omitempty"`
	Sources        []Source          `json:"sources,omitempty"`
}

type ReplicaCount struct {
	ModelValue         int    `json:"model_value"`
	Basis              string `json:"basis"`
	ObservedIdentities int    `json:"observed_identities"`
	Configured         *int   `json:"configured,omitempty"`
}

type Database struct {
	System             string   `json:"system"`
	Address            string   `json:"address"`
	Port               *int     `json:"port,omitempty"`
	Namespaces         []string `json:"namespaces,omitempty"`
	NetworkScope       string   `json:"network_scope,omitempty"`
	IdentityKind       string   `json:"identity_kind"`
	PhysicalInstanceID string   `json:"physical_instance_id,omitempty"`
	Sources            []Source `json:"sources,omitempty"`
}

type Relation struct {
	Kind          string   `json:"kind"`
	Basis         string   `json:"basis"`
	FromInstances []string `json:"from_instances,omitempty"`
	ToInstances   []string `json:"to_instances,omitempty"`
	Sources       []Source `json:"sources,omitempty"`
}

type Record struct {
	SchemaVersion   string        `json:"schema_version"`
	Replicas        *ReplicaCount `json:"replicas,omitempty"`
	Instances       []Instance    `json:"instances,omitempty"`
	Database        *Database     `json:"database,omitempty"`
	DatabaseAliases []Database    `json:"database_aliases,omitempty"`
	Peer            *Peer         `json:"peer,omitempty"`
	Relation        *Relation     `json:"relation,omitempty"`
	Warnings        []string      `json:"warnings,omitempty"`
}

func Encode(record Record) (string, error) {
	if version := strings.TrimSpace(record.SchemaVersion); version != "" && version != SchemaVersion {
		return "", fmt.Errorf("unsupported observation evidence schema_version %q", version)
	}
	record.SchemaVersion = SchemaVersion
	record = Normalize(record)
	raw, err := json.Marshal(record)
	if err != nil {
		return "", fmt.Errorf("encode observation evidence: %w", err)
	}
	return string(raw), nil
}

func Decode(value string) (Record, error) {
	if err := validateJSON(value); err != nil {
		return Record{}, fmt.Errorf("decode observation evidence: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewBufferString(value))
	decoder.DisallowUnknownFields()
	var record Record
	if err := decoder.Decode(&record); err != nil {
		return Record{}, fmt.Errorf("decode observation evidence: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return Record{}, fmt.Errorf("decode observation evidence: multiple JSON values")
		}
		return Record{}, fmt.Errorf("decode observation evidence trailing data: %w", err)
	}
	if record.SchemaVersion != SchemaVersion {
		return Record{}, fmt.Errorf("unsupported observation evidence schema_version %q", record.SchemaVersion)
	}
	return Normalize(record), nil
}

func validateJSON(value string) error {
	decoder := json.NewDecoder(bytes.NewBufferString(value))
	decoder.UseNumber()
	if err := validateJSONValue(decoder); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return fmt.Errorf("trailing data: %w", err)
	}
	return nil
}

func validateJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]struct{}{}
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("object key is not a string")
			}
			if _, exists := seen[key]; exists {
				return fmt.Errorf("duplicate JSON object key %q", key)
			}
			seen[key] = struct{}{}
			if err := validateJSONValue(decoder); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil {
			return err
		}
		if closing != json.Delim('}') {
			return fmt.Errorf("object is not closed")
		}
	case '[':
		for decoder.More() {
			if err := validateJSONValue(decoder); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil {
			return err
		}
		if closing != json.Delim(']') {
			return fmt.Errorf("array is not closed")
		}
	default:
		return fmt.Errorf("unexpected JSON delimiter %q", delim)
	}
	return nil
}

func Normalize(record Record) Record {
	record.SchemaVersion = strings.TrimSpace(record.SchemaVersion)
	if record.Replicas != nil {
		record.Replicas.Basis = strings.TrimSpace(record.Replicas.Basis)
	}
	if record.Peer != nil {
		record.Peer.Service = strings.TrimSpace(record.Peer.Service)
		record.Peer.Address = strings.TrimSpace(record.Peer.Address)
		record.Peer.NetworkScope = strings.TrimSpace(record.Peer.NetworkScope)
		record.Peer.IdentityKind = strings.TrimSpace(record.Peer.IdentityKind)
		record.Peer.Sources = normalizeSources(record.Peer.Sources)
	}
	for index := range record.Instances {
		item := &record.Instances[index]
		item.ID = strings.TrimSpace(item.ID)
		item.IdentityStatus = strings.TrimSpace(item.IdentityStatus)
		item.Attributes = normalizeMap(item.Attributes)
		item.Sources = normalizeSources(item.Sources)
	}
	record.Instances = mergeInstances(record.Instances)
	sort.Slice(record.Instances, func(i, j int) bool {
		left, _ := json.Marshal(record.Instances[i])
		right, _ := json.Marshal(record.Instances[j])
		return string(left) < string(right)
	})
	if record.Database != nil {
		normalizeDatabase(record.Database)
	}
	for index := range record.DatabaseAliases {
		normalizeDatabase(&record.DatabaseAliases[index])
	}
	record.DatabaseAliases = mergeDatabases(record.DatabaseAliases)
	sort.Slice(record.DatabaseAliases, func(i, j int) bool {
		left, _ := json.Marshal(record.DatabaseAliases[i])
		right, _ := json.Marshal(record.DatabaseAliases[j])
		return string(left) < string(right)
	})
	if record.Relation != nil {
		record.Relation.Kind = strings.TrimSpace(record.Relation.Kind)
		record.Relation.Basis = strings.TrimSpace(record.Relation.Basis)
		record.Relation.FromInstances = normalizeStrings(record.Relation.FromInstances)
		record.Relation.ToInstances = normalizeStrings(record.Relation.ToInstances)
		record.Relation.Sources = normalizeSources(record.Relation.Sources)
	}
	record.Warnings = normalizeStrings(record.Warnings)
	return record
}

func normalizeDatabase(database *Database) {
	database.System = strings.TrimSpace(database.System)
	database.Address = strings.TrimSpace(database.Address)
	database.Namespaces = normalizeStrings(database.Namespaces)
	database.NetworkScope = strings.TrimSpace(database.NetworkScope)
	database.IdentityKind = strings.TrimSpace(database.IdentityKind)
	database.PhysicalInstanceID = strings.TrimSpace(database.PhysicalInstanceID)
	database.Sources = normalizeSources(database.Sources)
}

func mergeDatabases(values []Database) []Database {
	positions := make(map[string]int, len(values))
	out := make([]Database, 0, len(values))
	for _, value := range values {
		port := ""
		if value.Port != nil {
			port = strconv.Itoa(*value.Port)
		}
		keyRaw, _ := json.Marshal([]string{
			value.System, value.Address, port, value.NetworkScope, value.IdentityKind, value.PhysicalInstanceID,
		})
		key := string(keyRaw)
		if index, ok := positions[key]; ok {
			out[index].Namespaces = normalizeStrings(append(out[index].Namespaces, value.Namespaces...))
			out[index].Sources = normalizeSources(append(out[index].Sources, value.Sources...))
			continue
		}
		positions[key] = len(out)
		out = append(out, value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string, len(values))
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := values[key]
		key = strings.TrimSpace(key)
		if key != "" {
			out[key] = strings.TrimSpace(value)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeSources(values []Source) []Source {
	seen := map[Source]struct{}{}
	out := make([]Source, 0, len(values))
	for _, value := range values {
		value.Kind = strings.TrimSpace(value.Kind)
		value.Connector = strings.TrimSpace(value.Connector)
		value.Domain = strings.TrimSpace(value.Domain)
		value.Ref = strings.TrimSpace(value.Ref)
		value.ScopeName = strings.TrimSpace(value.ScopeName)
		value.ScopeVersion = strings.TrimSpace(value.ScopeVersion)
		if value == (Source{}) {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool {
		left, right := out[i], out[j]
		if left.Kind != right.Kind {
			return left.Kind < right.Kind
		}
		if left.Connector != right.Connector {
			return left.Connector < right.Connector
		}
		if left.Domain != right.Domain {
			return left.Domain < right.Domain
		}
		if left.Ref != right.Ref {
			return left.Ref < right.Ref
		}
		if left.ScopeName != right.ScopeName {
			return left.ScopeName < right.ScopeName
		}
		return left.ScopeVersion < right.ScopeVersion
	})
	if len(out) == 0 {
		return nil
	}
	return out
}

func mergeInstances(values []Instance) []Instance {
	positions := make(map[string]int, len(values))
	out := make([]Instance, 0, len(values))
	for _, value := range values {
		raw, _ := json.Marshal(struct {
			ID             string            `json:"id"`
			IdentityStatus string            `json:"identity_status,omitempty"`
			Attributes     map[string]string `json:"attributes,omitempty"`
		}{value.ID, value.IdentityStatus, value.Attributes})
		key := string(raw)
		if index, ok := positions[key]; ok {
			out[index].Sources = normalizeSources(append(out[index].Sources, value.Sources...))
			continue
		}
		positions[key] = len(out)
		out = append(out, value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
