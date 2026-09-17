package orchestrator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type DraftRequirement struct {
	ID              string   `json:"id"`
	Description     string   `json:"description"`
	SourceSection   string   `json:"source_section,omitempty"`
	SourceRef       string   `json:"source_ref,omitempty"`
	SourceLine      int      `json:"source_line,omitempty"`
	SubRequirements []string `json:"sub_requirements,omitempty"`
}

type DraftRequirementsFile struct {
	Requirements []DraftRequirement `json:"requirements"`
}

type DraftAssumption struct {
	ID            string   `json:"id"`
	Statement     string   `json:"statement"`
	Confidence    string   `json:"confidence"`
	Impact        string   `json:"impact,omitempty"`
	Evidence      []string `json:"evidence,omitempty"`
	AffectedTasks []string `json:"affected_tasks,omitempty"`
}

type DraftAssumptionsFile struct {
	Assumptions []DraftAssumption `json:"assumptions"`
}

type DraftRisk struct {
	ID          string   `json:"id"`
	Type        string   `json:"type,omitempty"`
	Level       string   `json:"level"`
	Description string   `json:"description"`
	Mitigation  []string `json:"mitigation,omitempty"`
}

type DraftRisksFile struct {
	Risks []DraftRisk `json:"risks"`
}

type DraftImpactEntry struct {
	Repository string   `json:"repository"`
	Areas      []string `json:"areas"`
}

type DraftImpactFile struct {
	Impact []DraftImpactEntry `json:"impact"`
}

type DraftRepoAnalysisEntry struct {
	Repository string   `json:"repository"`
	Evidence   []string `json:"evidence"`
}

type DraftRepoAnalysisFile struct {
	Repositories []DraftRepoAnalysisEntry `json:"repositories"`
}

type DraftWorkspaceVerifyFile struct {
	Commands []string `json:"commands"`
}

type PlanningDraftBundle struct {
	Requirements DraftRequirementsFile
	Assumptions  DraftAssumptionsFile
	Risks        DraftRisksFile
	Impact       DraftImpactFile
	RepoAnalysis DraftRepoAnalysisFile
	WorkspaceVerify DraftWorkspaceVerifyFile
	Loaded       map[string]bool
	LoadErrors   []error
}

func PlanDraftDir(workspaceRoot string) string {
	return filepath.Join(PlansDir(workspaceRoot), "draft")
}

func draftFilePath(workspaceRoot, name string) string {
	return filepath.Join(PlanDraftDir(workspaceRoot), name)
}

func PlanDraftRequirementsPath(workspaceRoot string) string {
	return draftFilePath(workspaceRoot, "requirements.json")
}

func PlanDraftAssumptionsPath(workspaceRoot string) string {
	return draftFilePath(workspaceRoot, "assumptions.json")
}

func PlanDraftRisksPath(workspaceRoot string) string {
	return draftFilePath(workspaceRoot, "risks.json")
}

func PlanDraftImpactPath(workspaceRoot string) string {
	return draftFilePath(workspaceRoot, "impact.json")
}

func PlanDraftRepoAnalysisPath(workspaceRoot string) string {
	return draftFilePath(workspaceRoot, "repo-analysis.json")
}

func PlanDraftWorkspaceVerifyPath(workspaceRoot string) string {
	return draftFilePath(workspaceRoot, "workspace-verify.json")
}

func PlanDraftLastValidationPath(workspaceRoot string) string {
	return draftFilePath(workspaceRoot, "last-validation.json")
}

func PlanDraftClarificationRequestPath(workspaceRoot string) string {
	return draftFilePath(workspaceRoot, "clarification-request.json")
}

func PlanDraftClarificationResponsesPath(workspaceRoot string) string {
	return draftFilePath(workspaceRoot, "clarification-responses.json")
}

func loadDraftJSON(path string, dest any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, dest); err != nil {
		return fmt.Errorf("unmarshal %s: %w", path, err)
	}
	return nil
}

func LoadPlanningDraftBundle(workspaceRoot string) (PlanningDraftBundle, error) {
	bundle := PlanningDraftBundle{Loaded: map[string]bool{}}
	if err := EnsureDir(PlanDraftDir(workspaceRoot)); err != nil {
		return bundle, err
	}

	type loader struct {
		name string
		path string
		fn   func() error
	}
	loaders := []loader{
		{"requirements", PlanDraftRequirementsPath(workspaceRoot), func() error {
			return loadDraftJSON(PlanDraftRequirementsPath(workspaceRoot), &bundle.Requirements)
		}},
		{"assumptions", PlanDraftAssumptionsPath(workspaceRoot), func() error {
			return loadDraftJSON(PlanDraftAssumptionsPath(workspaceRoot), &bundle.Assumptions)
		}},
		{"risks", PlanDraftRisksPath(workspaceRoot), func() error {
			return loadDraftJSON(PlanDraftRisksPath(workspaceRoot), &bundle.Risks)
		}},
		{"impact", PlanDraftImpactPath(workspaceRoot), func() error {
			return loadDraftJSON(PlanDraftImpactPath(workspaceRoot), &bundle.Impact)
		}},
		{"repo-analysis", PlanDraftRepoAnalysisPath(workspaceRoot), func() error {
			return loadDraftJSON(PlanDraftRepoAnalysisPath(workspaceRoot), &bundle.RepoAnalysis)
		}},
		{"workspace-verify", PlanDraftWorkspaceVerifyPath(workspaceRoot), func() error {
			if !Exists(PlanDraftWorkspaceVerifyPath(workspaceRoot)) {
				return nil
			}
			return loadDraftJSON(PlanDraftWorkspaceVerifyPath(workspaceRoot), &bundle.WorkspaceVerify)
		}},
	}
	for _, l := range loaders {
		if l.name == "workspace-verify" && !Exists(l.path) {
			continue
		}
		if !Exists(l.path) {
			continue
		}
		if err := l.fn(); err != nil {
			bundle.LoadErrors = append(bundle.LoadErrors, err)
			continue
		}
		bundle.Loaded[l.name] = true
	}
	return bundle, nil
}

func SnapshotPlanningDraftBundle(workspaceRoot string, revision int) error {
	revDir := PlanRevisionDir(workspaceRoot, revision)
	if err := EnsureDir(revDir); err != nil {
		return err
	}
	names := []string{
		"requirements.json",
		"assumptions.json",
		"risks.json",
		"impact.json",
		"repo-analysis.json",
		"workspace-verify.json",
	}
	for _, name := range names {
		src := filepath.Join(PlanDraftDir(workspaceRoot), name)
		if !Exists(src) {
			continue
		}
		data, err := os.ReadFile(src)
		if err != nil {
			return err
		}
		if err := WriteFileAtomically(filepath.Join(revDir, name), data); err != nil {
			return err
		}
	}
	return nil
}

func LoadRevisionPlanningBundle(workspaceRoot string, revision int) (PlanningDraftBundle, error) {
	bundle := PlanningDraftBundle{Loaded: map[string]bool{}}
	revDir := PlanRevisionDir(workspaceRoot, revision)
	type spec struct {
		name string
		file string
		dest any
	}
	specs := []spec{
		{"requirements", "requirements.json", &bundle.Requirements},
		{"assumptions", "assumptions.json", &bundle.Assumptions},
		{"risks", "risks.json", &bundle.Risks},
		{"impact", "impact.json", &bundle.Impact},
		{"repo-analysis", "repo-analysis.json", &bundle.RepoAnalysis},
		{"workspace-verify", "workspace-verify.json", &bundle.WorkspaceVerify},
	}
	for _, s := range specs {
		path := filepath.Join(revDir, s.file)
		if !Exists(path) {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return bundle, err
		}
		if err := json.Unmarshal(data, s.dest); err != nil {
			return bundle, fmt.Errorf("unmarshal %s: %w", path, err)
		}
		bundle.Loaded[s.name] = true
	}
	return bundle, nil
}

func MergeDraftBundleIntoPlan(bundle PlanningDraftBundle, tasks []Task, feature PlanFeature) PlanDocument {
	doc := BuildPlanFromTasks("", tasks, feature)

	if len(bundle.Requirements.Requirements) > 0 {
		reqs := make([]PlanRequirement, 0, len(bundle.Requirements.Requirements))
		reqTaskMap := map[string][]string{}
		for _, task := range tasks {
			for _, reqID := range task.RequirementIDs {
				reqTaskMap[reqID] = append(reqTaskMap[reqID], task.ID)
			}
		}
		for _, r := range bundle.Requirements.Requirements {
			reqs = append(reqs, PlanRequirement{
				ID:          r.ID,
				Description: r.Description,
				Tasks:       reqTaskMap[r.ID],
			})
		}
		doc.Requirements = reqs
	}

	if len(bundle.Assumptions.Assumptions) > 0 {
		assumptions := make([]PlanAssumption, 0, len(bundle.Assumptions.Assumptions))
		for _, a := range bundle.Assumptions.Assumptions {
			assumptions = append(assumptions, PlanAssumption{
				ID:            a.ID,
				Statement:     a.Statement,
				Confidence:    a.Confidence,
				Impact:        a.Impact,
				Evidence:      a.Evidence,
				AffectedTasks: a.AffectedTasks,
			})
		}
		doc.Assumptions = assumptions
	}

	if len(bundle.Risks.Risks) > 0 {
		risks := make([]PlanRisk, 0, len(bundle.Risks.Risks))
		for _, r := range bundle.Risks.Risks {
			risks = append(risks, PlanRisk{
				ID:          r.ID,
				Type:        r.Type,
				Level:       r.Level,
				Description: r.Description,
				Mitigation:  r.Mitigation,
			})
		}
		doc.Risks = risks
	}

	if len(bundle.Impact.Impact) > 0 {
		areas := map[string][]string{}
		for _, entry := range bundle.Impact.Impact {
			areas[entry.Repository] = append(areas[entry.Repository], entry.Areas...)
		}
		doc.ExpectedChangeAreas = areas
	}

	if len(bundle.WorkspaceVerify.Commands) > 0 {
		doc.Verification.Strategy = "repository-aware"
		if doc.Verification.ByRepo == nil {
			doc.Verification.ByRepo = map[string][]string{}
		}
		doc.Verification.ByRepo["workspace"] = bundle.WorkspaceVerify.Commands
	}

	return doc
}

func WritePlanningBundleFixture(workspaceRoot string, bundle PlanningDraftBundle) error {
	if err := EnsureDir(PlanDraftDir(workspaceRoot)); err != nil {
		return err
	}
	writes := []struct {
		path string
		v    any
	}{
		{PlanDraftRequirementsPath(workspaceRoot), bundle.Requirements},
		{PlanDraftAssumptionsPath(workspaceRoot), bundle.Assumptions},
		{PlanDraftRisksPath(workspaceRoot), bundle.Risks},
		{PlanDraftImpactPath(workspaceRoot), bundle.Impact},
		{PlanDraftRepoAnalysisPath(workspaceRoot), bundle.RepoAnalysis},
	}
	for _, w := range writes {
		data, err := json.MarshalIndent(w.v, "", "  ")
		if err != nil {
			return err
		}
		if err := WriteFileAtomically(w.path, append(data, '\n')); err != nil {
			return err
		}
	}
	if len(bundle.WorkspaceVerify.Commands) > 0 {
		data, err := json.MarshalIndent(bundle.WorkspaceVerify, "", "  ")
		if err != nil {
			return err
		}
		if err := WriteFileAtomically(PlanDraftWorkspaceVerifyPath(workspaceRoot), append(data, '\n')); err != nil {
			return err
		}
	}
	return nil
}

func ValidateDraftBundleEarly(workspaceRoot string, bundle PlanningDraftBundle) PlanValidationReport {
	report := PlanValidationReport{Valid: true, Checks: map[string]string{}}
	required := []struct {
		name  string
		code  string
		path  func(string) string
		check func(PlanningDraftBundle) bool
	}{
		{"requirements", "missing_requirements_file", PlanDraftRequirementsPath, func(b PlanningDraftBundle) bool {
			return b.Loaded["requirements"] && len(b.Requirements.Requirements) > 0
		}},
		{"assumptions", "missing_assumptions_file", PlanDraftAssumptionsPath, func(b PlanningDraftBundle) bool {
			return b.Loaded["assumptions"] && len(b.Assumptions.Assumptions) > 0
		}},
		{"risks", "missing_risks_file", PlanDraftRisksPath, func(b PlanningDraftBundle) bool {
			return b.Loaded["risks"] && len(b.Risks.Risks) > 0
		}},
		{"impact", "missing_impact_file", PlanDraftImpactPath, func(b PlanningDraftBundle) bool {
			return b.Loaded["impact"] && len(b.Impact.Impact) > 0
		}},
		{"repo-analysis", "missing_repo_analysis_file", PlanDraftRepoAnalysisPath, func(b PlanningDraftBundle) bool {
			return b.Loaded["repo-analysis"] && len(b.RepoAnalysis.Repositories) > 0
		}},
	}
	for _, r := range required {
		if !Exists(r.path(workspaceRoot)) {
			report.Valid = false
			report.Errors = append(report.Errors, PlanValidationIssue{
				Level: "error", Code: r.code,
				Message: fmt.Sprintf("missing required draft file: %s", r.path(workspaceRoot)),
			})
			continue
		}
		if !r.check(bundle) {
			report.Valid = false
			report.Errors = append(report.Errors, PlanValidationIssue{
				Level: "error", Code: r.code,
				Message: fmt.Sprintf("draft file %s is empty or invalid", r.name),
			})
		}
	}
	for _, err := range bundle.LoadErrors {
		report.Valid = false
		report.Errors = append(report.Errors, translateDraftLoadError(err))
	}
	if report.Valid {
		report.Checks["draft_bundle"] = "PASS"
	} else {
		report.Checks["draft_bundle"] = "FAIL"
	}
	return report
}

func ValidateDraftBundleMerged(workspaceRoot string, bundle PlanningDraftBundle, tasks []Task, doc PlanDocument) PlanValidationReport {
	report := PlanValidationReport{Valid: true, Checks: map[string]string{}}
	repos, _ := readRepositoryRegistry(workspaceRoot)
	repoIDs := map[string]bool{}
	for _, r := range repos {
		repoIDs[r.ID] = true
	}

	reqIDs := map[string]bool{}
	for _, r := range bundle.Requirements.Requirements {
		reqIDs[r.ID] = true
	}
	taskCoversReq := map[string]bool{}
	for _, task := range tasks {
		for _, reqID := range task.RequirementIDs {
			if !reqIDs[reqID] {
				report.Valid = false
				report.Errors = append(report.Errors, PlanValidationIssue{
					Level: "error", Code: "unknown_requirement_id",
					Message: fmt.Sprintf("task %s references unknown requirement %s", task.ID, reqID),
				})
			}
			taskCoversReq[reqID] = true
		}
	}
	for _, req := range bundle.Requirements.Requirements {
		covered := taskCoversReq[req.ID]
		if !covered {
			for _, pt := range doc.Requirements {
				if pt.ID == req.ID && len(pt.Tasks) > 0 {
					covered = true
					break
				}
			}
		}
		if !covered {
			report.Valid = false
			report.Errors = append(report.Errors, PlanValidationIssue{
				Level: "error", Code: "orphan_requirement",
				Message: fmt.Sprintf("requirement %s is not covered by any task", req.ID),
			})
		}
	}

	taskRepos := map[string]bool{}
	for _, task := range tasks {
		if task.Repository != "" {
			taskRepos[task.Repository] = true
		}
	}
	analyzed := map[string]bool{}
	for _, entry := range bundle.RepoAnalysis.Repositories {
		if len(entry.Evidence) == 0 {
			report.Valid = false
			report.Errors = append(report.Errors, PlanValidationIssue{
				Level: "error", Code: "repo_analysis_incomplete",
				Message: fmt.Sprintf("repo-analysis for %s has no evidence paths", entry.Repository),
			})
		}
		analyzed[entry.Repository] = true
	}
	for repo := range taskRepos {
		if !analyzed[repo] {
			report.Valid = false
			report.Errors = append(report.Errors, PlanValidationIssue{
				Level: "error", Code: "repo_analysis_incomplete",
				Message: fmt.Sprintf("repo-analysis missing evidence for repository %s used by tasks", repo),
			})
		}
	}

	for _, entry := range bundle.Impact.Impact {
		if !repoIDs[entry.Repository] {
			report.Valid = false
			report.Errors = append(report.Errors, PlanValidationIssue{
				Level: "error", Code: "invalid_impact_repository",
				Message: fmt.Sprintf("impact references unknown repository %s", entry.Repository),
			})
		}
		if len(entry.Areas) == 0 {
			report.Valid = false
			report.Errors = append(report.Errors, PlanValidationIssue{
				Level: "error", Code: "missing_expected_change_areas",
				Message: fmt.Sprintf("impact entry for %s has no change areas", entry.Repository),
			})
		}
	}

	if len(doc.ExpectedChangeAreas) == 0 && len(bundle.Impact.Impact) > 0 {
		report.Valid = false
		report.Errors = append(report.Errors, PlanValidationIssue{
			Level: "error", Code: "missing_expected_change_areas",
			Message: "expected change areas missing from merged plan",
		})
	}

	if report.Valid {
		report.Checks["draft_bundle_merged"] = "PASS"
	} else {
		report.Checks["draft_bundle_merged"] = "FAIL"
	}
	return report
}
