package orchestrator

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
)

type fingerprintPayload struct {
	Tasks              []PlanTask                `json:"tasks"`
	Repositories       []PlanRepository          `json:"repositories"`
	Requirements       []PlanRequirement         `json:"requirements"`
	AcceptanceCriteria []PlanAcceptanceCriterion `json:"acceptance_criteria"`
	Verification       PlanVerificationStrategy  `json:"verification"`
	Assumptions        []PlanAssumption          `json:"assumptions"`
	Risks              []PlanRisk                `json:"risks"`
}

func ComputePlanFingerprint(doc PlanDocument) (string, error) {
	payload := fingerprintPayload{
		Tasks:              sortedPlanTasks(doc.Tasks),
		Repositories:       sortedPlanRepos(doc.Repositories),
		Requirements:       doc.Requirements,
		AcceptanceCriteria: doc.AcceptanceCriteria,
		Verification:       doc.Verification,
		Assumptions:        doc.Assumptions,
		Risks:              doc.Risks,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func sortedPlanTasks(tasks []PlanTask) []PlanTask {
	out := append([]PlanTask(nil), tasks...)
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func sortedPlanRepos(repos []PlanRepository) []PlanRepository {
	out := append([]PlanRepository(nil), repos...)
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
