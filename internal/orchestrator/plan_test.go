package orchestrator

import "testing"

func TestPlanFingerprint_Deterministic(t *testing.T) {
	doc := loadFixturePlan(t, "valid-minimal")
	fp1, err := ComputePlanFingerprint(doc)
	if err != nil {
		t.Fatalf("ComputePlanFingerprint: %v", err)
	}
	fp2, err := ComputePlanFingerprint(doc)
	if err != nil {
		t.Fatalf("ComputePlanFingerprint: %v", err)
	}
	if fp1 != fp2 {
		t.Fatalf("expected deterministic fingerprint, got %s vs %s", fp1, fp2)
	}
}

func TestPlanFingerprint_ChangesOnStructuralEdit(t *testing.T) {
	doc := loadFixturePlan(t, "valid-minimal")
	fp1, _ := ComputePlanFingerprint(doc)
	doc.Tasks = append(doc.Tasks, PlanTask{ID: "T002", Title: "New", Repository: "repo-a"})
	fp2, _ := ComputePlanFingerprint(doc)
	if fp1 == fp2 {
		t.Fatal("expected fingerprint to change after structural edit")
	}
}
