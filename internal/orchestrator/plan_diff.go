package orchestrator

import (
	"fmt"
	"strings"
)

type PlanDiff struct {
	FromRevision int
	ToRevision   int
	Added        []string
	Removed      []string
	Modified     []string
	DepChanges   []string
	RiskChanges  []string
}

func ComparePlanDocuments(from, to PlanDocument) PlanDiff {
	diff := PlanDiff{}
	fromTasks := map[string]PlanTask{}
	toTasks := map[string]PlanTask{}
	for _, t := range from.Tasks {
		fromTasks[t.ID] = t
	}
	for _, t := range to.Tasks {
		toTasks[t.ID] = t
	}
	for id, t := range toTasks {
		if _, ok := fromTasks[id]; !ok {
			diff.Added = append(diff.Added, fmt.Sprintf("+ %s %s", id, t.Title))
		}
	}
	for id, t := range fromTasks {
		if _, ok := toTasks[id]; !ok {
			diff.Removed = append(diff.Removed, fmt.Sprintf("- %s %s", id, t.Title))
		}
	}
	for id, nt := range toTasks {
		ft, ok := fromTasks[id]
		if !ok {
			continue
		}
		if taskStructurallyDifferent(ft, nt) {
			diff.Modified = append(diff.Modified, fmt.Sprintf("%s: updated task definition", id))
		}
		if !equalStringSlices(ft.Dependencies, nt.Dependencies) {
			diff.DepChanges = append(diff.DepChanges, fmt.Sprintf("%s dependencies changed", id))
		}
	}
	fromRisks := map[string]string{}
	toRisks := map[string]string{}
	for _, r := range from.Risks {
		fromRisks[r.ID] = r.Level
	}
	for _, r := range to.Risks {
		toRisks[r.ID] = r.Level
	}
	for id, lvl := range toRisks {
		if prev, ok := fromRisks[id]; ok && prev != lvl {
			diff.RiskChanges = append(diff.RiskChanges, fmt.Sprintf("%s risk %s -> %s", id, prev, lvl))
		}
	}
	return diff
}

func taskStructurallyDifferent(a, b PlanTask) bool {
	return a.Title != b.Title || a.Repository != b.Repository || a.Goal != b.Goal ||
		!equalStringSlices(a.Dependencies, b.Dependencies)
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func RenderPlanDiffText(diff PlanDiff) string {
	var b strings.Builder
	if diff.FromRevision > 0 && diff.ToRevision > 0 {
		b.WriteString(fmt.Sprintf("Revision: %d -> %d\n\n", diff.FromRevision, diff.ToRevision))
	}
	if len(diff.Added) > 0 {
		b.WriteString("Added:\n")
		for _, line := range diff.Added {
			b.WriteString(line + "\n")
		}
		b.WriteString("\n")
	}
	if len(diff.Removed) > 0 {
		b.WriteString("Removed:\n")
		for _, line := range diff.Removed {
			b.WriteString(line + "\n")
		}
		b.WriteString("\n")
	}
	if len(diff.Modified) > 0 {
		b.WriteString("Modified:\n")
		for _, line := range diff.Modified {
			b.WriteString(line + "\n")
		}
		b.WriteString("\n")
	}
	if len(diff.DepChanges) > 0 {
		b.WriteString("Dependencies:\n")
		for _, line := range diff.DepChanges {
			b.WriteString(line + "\n")
		}
		b.WriteString("\n")
	}
	if len(diff.RiskChanges) > 0 {
		b.WriteString("Risk:\n")
		for _, line := range diff.RiskChanges {
			b.WriteString(line + "\n")
		}
	}
	if b.Len() == 0 {
		return "- No structural changes\n"
	}
	return b.String()
}

func PlanDiffBetweenRevisions(workspaceRoot string, fromRev, toRev int) (PlanDiff, error) {
	ws, err := LoadWorkflowState(workspaceRoot)
	if err != nil {
		return PlanDiff{}, err
	}
	if toRev <= 0 {
		toRev = ws.CurrentPlanRevision
	}
	if fromRev <= 0 {
		fromRev = toRev - 1
	}
	if fromRev <= 0 || toRev <= 0 {
		return PlanDiff{}, fmt.Errorf("invalid revision range")
	}
	fromDoc, err := LoadPlanDocument(PlanRevisionPath(workspaceRoot, fromRev))
	if err != nil {
		return PlanDiff{}, err
	}
	toDoc, err := LoadPlanDocument(PlanRevisionPath(workspaceRoot, toRev))
	if err != nil {
		return PlanDiff{}, err
	}
	diff := ComparePlanDocuments(fromDoc, toDoc)
	diff.FromRevision = fromRev
	diff.ToRevision = toRev
	return diff, nil
}

func FormatPlanDiffOutput(diff PlanDiff) string {
	var b strings.Builder
	b.WriteString("Plan Changes\n")
	b.WriteString(strings.Repeat("─", 24) + "\n\n")
	b.WriteString(RenderPlanDiffText(diff))
	return b.String()
}
