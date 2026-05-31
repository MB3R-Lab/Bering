package reconciliation

import (
	"fmt"
	"sort"
	"strings"

	"github.com/MB3R-Lab/Bering/internal/snapshot"
)

func BuildOperatorSummary(report Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Bering Runtime Reconciliation Summary\n\n")
	fmt.Fprintf(&b, "Window: `%s` to `%s`\n\n", report.WindowStart, report.WindowEnd)
	fmt.Fprintf(&b, "Observation version: `%d`\n", report.Versions.ObservationVersion)
	fmt.Fprintf(&b, "Stable core version: `%d`\n", report.Versions.StableCoreVersion)
	fmt.Fprintf(&b, "Guardrail union version: `%d`\n\n", report.Versions.GuardrailUnionVersion)

	fmt.Fprintf(&b, "## Telemetry Health\n\n")
	fmt.Fprintf(&b, "- Score: `%.3f`\n", report.TelemetryHealth.Score)
	fmt.Fprintf(&b, "- Drop ratio: `%.3f`\n", report.TelemetryHealth.DropRatio)
	fmt.Fprintf(&b, "- Late ratio: `%.3f`\n", report.TelemetryHealth.LateRatio)
	fmt.Fprintf(&b, "- Volume ratio: `%.3f`\n", report.TelemetryHealth.VolumeRatio)
	fmt.Fprintf(&b, "- Retirement frozen: `%t`\n", report.TelemetryHealth.RetirementFrozen)
	if len(report.TelemetryHealth.Reasons) > 0 {
		fmt.Fprintf(&b, "- Reasons: %s\n", strings.Join(report.TelemetryHealth.Reasons, "; "))
	}
	fmt.Fprintf(&b, "\n")

	fmt.Fprintf(&b, "## Projections\n\n")
	fmt.Fprintf(&b, "| View | Version | Changed | Services | Edges | Endpoints | Diff |\n")
	fmt.Fprintf(&b, "| --- | ---: | --- | ---: | ---: | ---: | --- |\n")
	for _, projection := range report.Projections {
		fmt.Fprintf(&b, "| `%s` | `%d` | `%t` | %d | %d | %d | %s |\n",
			projection.Name,
			projection.Version,
			projection.Changed,
			projection.Counts.Services,
			projection.Counts.Edges,
			projection.Counts.Endpoints,
			formatDiff(projection.Diff),
		)
	}
	fmt.Fprintf(&b, "\n")

	fmt.Fprintf(&b, "## Entity Lifecycle\n\n")
	fmt.Fprintf(&b, "| Kind | Candidate | Active | Stale | Retired |\n")
	fmt.Fprintf(&b, "| --- | ---: | ---: | ---: | ---: |\n")
	lifecycleCounts := countLifecycles(report.Entities)
	for _, kind := range []Kind{KindService, KindEdge, KindEndpoint} {
		counts := lifecycleCounts[kind]
		fmt.Fprintf(&b, "| `%s` | %d | %d | %d | %d |\n",
			kind,
			counts[LifecycleCandidate],
			counts[LifecycleActive],
			counts[LifecycleStale],
			counts[LifecycleRetired],
		)
	}
	fmt.Fprintf(&b, "\n")

	writeAttentionEntities(&b, report.Entities)
	writeTransitions(&b, report.Transitions)
	writeCompaction(&b, report.Compaction)
	writeLoadWarnings(&b, report.LoadWarnings)
	return b.String()
}

func WriteSummary(path string, report Report) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	raw := []byte(BuildOperatorSummary(report))
	if len(raw) == 0 || raw[len(raw)-1] != '\n' {
		raw = append(raw, '\n')
	}
	if err := writeAtomic(path, raw); err != nil {
		return fmt.Errorf("write reconciliation summary: %w", err)
	}
	return nil
}

func countLifecycles(items []EntityStateSummary) map[Kind]map[Lifecycle]int {
	out := map[Kind]map[Lifecycle]int{}
	for _, kind := range []Kind{KindService, KindEdge, KindEndpoint} {
		out[kind] = map[Lifecycle]int{}
	}
	for _, item := range items {
		if out[item.Kind] == nil {
			out[item.Kind] = map[Lifecycle]int{}
		}
		out[item.Kind][item.Lifecycle]++
	}
	return out
}

func writeAttentionEntities(b *strings.Builder, items []EntityStateSummary) {
	attention := make([]EntityStateSummary, 0)
	for _, item := range items {
		if item.Lifecycle == LifecycleStale || item.Lifecycle == LifecycleRetired || item.QualifiedMiss {
			attention = append(attention, item)
		}
	}
	sort.Slice(attention, func(i, j int) bool {
		left, right := attention[i], attention[j]
		if left.Lifecycle != right.Lifecycle {
			return lifecycleRank(left.Lifecycle) > lifecycleRank(right.Lifecycle)
		}
		if left.MissStreak != right.MissStreak {
			return left.MissStreak > right.MissStreak
		}
		if left.Kind != right.Kind {
			return left.Kind < right.Kind
		}
		return left.ID < right.ID
	})
	fmt.Fprintf(b, "## Attention\n\n")
	if len(attention) == 0 {
		fmt.Fprintf(b, "No stale, retired, or qualified-miss entities in this window.\n\n")
		return
	}
	fmt.Fprintf(b, "| Kind | ID | Lifecycle | Miss streak | Existence | Activity | Reason |\n")
	fmt.Fprintf(b, "| --- | --- | --- | ---: | ---: | ---: | --- |\n")
	for i, item := range attention {
		if i >= 10 {
			break
		}
		fmt.Fprintf(b, "| `%s` | `%s` | `%s` | %d | %.3f | %.3f | %s |\n",
			item.Kind,
			item.ID,
			item.Lifecycle,
			item.MissStreak,
			item.ExistenceConfidence,
			item.ActivityConfidence,
			tableText(item.Reason),
		)
	}
	fmt.Fprintf(b, "\n")
}

func writeTransitions(b *strings.Builder, transitions []Transition) {
	fmt.Fprintf(b, "## Transitions\n\n")
	if len(transitions) == 0 {
		fmt.Fprintf(b, "No lifecycle transitions in this window.\n\n")
		return
	}
	fmt.Fprintf(b, "| Kind | ID | From | To | Reason |\n")
	fmt.Fprintf(b, "| --- | --- | --- | --- | --- |\n")
	for i, item := range transitions {
		if i >= 20 {
			break
		}
		fmt.Fprintf(b, "| `%s` | `%s` | `%s` | `%s` | %s |\n",
			item.Kind,
			item.ID,
			item.From,
			item.To,
			tableText(item.Reason),
		)
	}
	fmt.Fprintf(b, "\n")
}

func writeCompaction(b *strings.Builder, compaction CompactionSummary) {
	fmt.Fprintf(b, "## Compaction\n\n")
	fmt.Fprintf(b, "- Performed: `%t`\n", compaction.Performed)
	fmt.Fprintf(b, "- Expired retired: `%d`\n", compaction.ExpiredRetired)
	fmt.Fprintf(b, "- Trimmed per kind: `%d`\n", compaction.TrimmedPerKind)
	fmt.Fprintf(b, "- Trimmed overall: `%d`\n\n", compaction.TrimmedOverall)
}

func writeLoadWarnings(b *strings.Builder, warnings []string) {
	if len(warnings) == 0 {
		return
	}
	fmt.Fprintf(b, "## Load Warnings\n\n")
	for _, warning := range warnings {
		fmt.Fprintf(b, "- %s\n", warning)
	}
	fmt.Fprintf(b, "\n")
}

func lifecycleRank(lifecycle Lifecycle) int {
	switch lifecycle {
	case LifecycleRetired:
		return 4
	case LifecycleStale:
		return 3
	case LifecycleCandidate:
		return 2
	case LifecycleActive:
		return 1
	default:
		return 0
	}
}

func formatDiff(diff snapshot.DiffSummary) string {
	return tableText(fmt.Sprintf("svc +%d/-%d/~%d, edge +%d/-%d/~%d, endpoint +%d/-%d/~%d",
		diff.AddedServices,
		diff.RemovedServices,
		diff.ChangedServices,
		diff.AddedEdges,
		diff.RemovedEdges,
		diff.ChangedEdges,
		diff.AddedEndpoints,
		diff.RemovedEndpoints,
		diff.ChangedEndpoints,
	))
}

func tableText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = strings.ReplaceAll(value, "|", "\\|")
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	return value
}
