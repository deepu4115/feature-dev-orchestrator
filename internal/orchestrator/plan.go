package orchestrator

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

type PlanFeature struct {
	ID    string `json:"id" yaml:"id"`
	Title string `json:"title" yaml:"title"`
}

type PlanMeta struct {
	Revision    int       `json:"revision" yaml:"revision"`
	Status      string    `json:"status" yaml:"status"`
	GeneratedAt time.Time `json:"generated_at" yaml:"generated_at"`
	Fingerprint string    `json:"fingerprint,omitempty" yaml:"fingerprint,omitempty"`
}

type PlanRepository struct {
	ID       string `json:"id" yaml:"id"`
	Relevant bool   `json:"relevant" yaml:"relevant"`
}

type PlanRequirement struct {
	ID          string   `json:"id" yaml:"id"`
	Description string   `json:"description" yaml:"description"`
	Tasks       []string `json:"tasks,omitempty" yaml:"tasks,omitempty"`
}

type PlanAcceptanceCriterion struct {
	ID                    string   `json:"id" yaml:"id"`
	Description           string   `json:"description,omitempty" yaml:"description,omitempty"`
	ImplementationTasks   []string `json:"implementation_tasks" yaml:"implementation_tasks"`
	PlannedVerification   []string `json:"planned_verification" yaml:"planned_verification"`
}

type PlanTask struct {
	ID           string   `json:"id" yaml:"id"`
	Title        string   `json:"title" yaml:"title"`
	Repository   string   `json:"repository" yaml:"repository"`
	Goal         string   `json:"goal,omitempty" yaml:"goal,omitempty"`
	Dependencies []string `json:"dependencies,omitempty" yaml:"dependencies,omitempty"`
	Verification []VerificationStep `json:"verification,omitempty" yaml:"verification,omitempty"`
}

type PlanAssumption struct {
	ID         string   `json:"id" yaml:"id"`
	Statement  string   `json:"statement" yaml:"statement"`
	Confidence string   `json:"confidence" yaml:"confidence"`
	Impact     string   `json:"impact,omitempty" yaml:"impact,omitempty"`
	Evidence   []string `json:"evidence,omitempty" yaml:"evidence,omitempty"`
	AffectedTasks []string `json:"affected_tasks,omitempty" yaml:"affected_tasks,omitempty"`
}

type PlanRisk struct {
	ID          string   `json:"id" yaml:"id"`
	Type        string   `json:"type,omitempty" yaml:"type,omitempty"`
	Level       string   `json:"level" yaml:"level"`
	Description string   `json:"description" yaml:"description"`
	Mitigation  []string `json:"mitigation,omitempty" yaml:"mitigation,omitempty"`
}

type PlanVerificationStrategy struct {
	Strategy string            `json:"strategy" yaml:"strategy"`
	ByRepo   map[string][]string `json:"by_repository,omitempty" yaml:"by_repository,omitempty"`
}

type PlanSnapshotRepo struct {
	Branch string `json:"branch" yaml:"branch"`
	Head   string `json:"head" yaml:"head"`
}

type PlanSnapshot struct {
	Repositories map[string]PlanSnapshotRepo `json:"repositories" yaml:"repositories"`
}

type PlanApprovalBlock struct {
	Required          bool `json:"required" yaml:"required"`
	ApprovedRevision  *int `json:"approved_revision" yaml:"approved_revision"`
}

type PlanDocument struct {
	SchemaVersion        string                    `json:"schema_version" yaml:"schema_version"`
	Feature              PlanFeature               `json:"feature" yaml:"feature"`
	Plan                 PlanMeta                  `json:"plan" yaml:"plan"`
	Repositories         []PlanRepository          `json:"repositories,omitempty" yaml:"repositories,omitempty"`
	Requirements         []PlanRequirement         `json:"requirements,omitempty" yaml:"requirements,omitempty"`
	AcceptanceCriteria   []PlanAcceptanceCriterion `json:"acceptance_criteria,omitempty" yaml:"acceptance_criteria,omitempty"`
	Tasks                []PlanTask                `json:"tasks" yaml:"tasks"`
	Assumptions          []PlanAssumption          `json:"assumptions,omitempty" yaml:"assumptions,omitempty"`
	Risks                []PlanRisk                `json:"risks,omitempty" yaml:"risks,omitempty"`
	Verification         PlanVerificationStrategy  `json:"verification,omitempty" yaml:"verification,omitempty"`
	ExpectedChangeAreas  map[string][]string       `json:"expected_change_areas,omitempty" yaml:"expected_change_areas,omitempty"`
	PlanSnapshot         PlanSnapshot              `json:"plan_snapshot,omitempty" yaml:"plan_snapshot,omitempty"`
	Approval             PlanApprovalBlock         `json:"approval,omitempty" yaml:"approval,omitempty"`
	OpenQuestions        []string                  `json:"open_questions,omitempty" yaml:"open_questions,omitempty"`
}

func PlansDir(workspaceRoot string) string {
	return filepath.Join(ResolveFeatureDir(workspaceRoot), "plans")
}

func PlanDraftPath(workspaceRoot string) string {
	return filepath.Join(PlansDir(workspaceRoot), "draft", "plan.yaml")
}

func PlanRevisionDir(workspaceRoot string, revision int) string {
	return filepath.Join(PlansDir(workspaceRoot), fmt.Sprintf("revision-%03d", revision))
}

func PlanRevisionPath(workspaceRoot string, revision int) string {
	return filepath.Join(PlanRevisionDir(workspaceRoot, revision), "plan.yaml")
}

func CurrentPlanPath(workspaceRoot string, ws WorkflowState) string {
	if ws.CurrentPlanRevision <= 0 {
		return ""
	}
	return PlanRevisionPath(workspaceRoot, ws.CurrentPlanRevision)
}

func LoadPlanDocument(path string) (PlanDocument, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return PlanDocument{}, fmt.Errorf("read plan: %w", err)
	}
	var doc PlanDocument
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return PlanDocument{}, fmt.Errorf("unmarshal plan: %w", err)
	}
	return doc, nil
}

func SavePlanDocument(path string, doc PlanDocument) error {
	if doc.SchemaVersion == "" {
		doc.SchemaVersion = "1.0"
	}
	data, err := yaml.Marshal(doc)
	if err != nil {
		return fmt.Errorf("marshal plan: %w", err)
	}
	return WriteFileAtomically(path, data)
}

func LoadCurrentPlanDocument(workspaceRoot string) (PlanDocument, error) {
	ws, err := LoadWorkflowState(workspaceRoot)
	if err != nil {
		return PlanDocument{}, err
	}
	if ws.CurrentPlanRevision <= 0 {
		return PlanDocument{}, fmt.Errorf("no plan revision exists")
	}
	return LoadPlanDocument(PlanRevisionPath(workspaceRoot, ws.CurrentPlanRevision))
}

func PlanTasksToRuntimeTasks(planTasks []PlanTask) []Task {
	tasks := make([]Task, 0, len(planTasks))
	for _, pt := range planTasks {
		tasks = append(tasks, Task{
			ID:           pt.ID,
			Title:        pt.Title,
			Repository:   pt.Repository,
			Goal:         pt.Goal,
			Dependencies: pt.Dependencies,
			Verification: pt.Verification,
			Status:       StatusReviewPending,
		})
	}
	return tasks
}

func SyncPlanTasksToRuntime(workspaceRoot string, planTasks []PlanTask) ([]Task, error) {
	existing, err := LoadTasks(workspaceRoot)
	if err != nil {
		return nil, err
	}
	byID := map[string]Task{}
	for _, t := range existing {
		byID[t.ID] = t
	}

	out := make([]Task, 0, len(planTasks))
	for _, pt := range planTasks {
		rt := Task{
			ID:           pt.ID,
			Title:        pt.Title,
			Repository:   pt.Repository,
			Goal:         pt.Goal,
			Dependencies: pt.Dependencies,
			Verification: pt.Verification,
			Status:       StatusReviewPending,
			UpdatedAt:    time.Now().UTC(),
		}
		if prev, ok := byID[pt.ID]; ok {
			rt.RepositoryRationale = prev.RepositoryRationale
			rt.OwnershipConfidence = prev.OwnershipConfidence
			rt.RequirementIDs = prev.RequirementIDs
			rt.PlannedVerification = prev.PlannedVerification
			rt.AcceptanceCriteria = prev.AcceptanceCriteria
			rt.ExpectedFiles = prev.ExpectedFiles
			rt.ContextRefs = prev.ContextRefs
			switch prev.Status {
			case StatusDone, StatusRunning, StatusImplemented, StatusVerifying:
				rt.Status = prev.Status
				rt.VerificationStale = prev.VerificationStale
			case StatusReady, StatusBlocked, StatusFailed, StatusRework:
				rt.Status = prev.Status
			default:
				rt.Status = StatusReviewPending
			}
		}
		out = append(out, rt)
	}
	return out, nil
}

func BuildPlanFromTasks(workspaceRoot string, tasks []Task, feature PlanFeature) PlanDocument {
	reqMap := map[string]*PlanRequirement{}
	acs := []PlanAcceptanceCriterion{}

	for i, task := range tasks {
		if len(task.RequirementIDs) > 0 {
			for _, reqID := range task.RequirementIDs {
				if reqMap[reqID] == nil {
					reqMap[reqID] = &PlanRequirement{ID: reqID, Description: task.Title}
				}
				reqMap[reqID].Tasks = append(reqMap[reqID].Tasks, task.ID)
			}
		} else {
			reqID := fmt.Sprintf("R%03d", i+1)
			reqMap[reqID] = &PlanRequirement{
				ID:          reqID,
				Description: task.Title,
				Tasks:       []string{task.ID},
			}
		}

		pv := task.PlannedVerification
		if len(pv) == 0 {
			for _, v := range task.Verification {
				if v.Name != "" {
					pv = append(pv, v.Name)
				} else if v.Command != "" {
					pv = append(pv, v.Command)
				}
			}
		}
		if len(pv) == 0 {
			pv = []string{"task-verification"}
		}
		if len(task.AcceptanceCriteria) > 0 || len(task.Verification) > 0 || len(task.PlannedVerification) > 0 {
			acs = append(acs, PlanAcceptanceCriterion{
				ID:                  fmt.Sprintf("AC%03d", i+1),
				Description:         task.Title,
				ImplementationTasks: []string{task.ID},
				PlannedVerification: pv,
			})
		}
	}

	reqs := make([]PlanRequirement, 0, len(reqMap))
	for _, req := range reqMap {
		reqs = append(reqs, *req)
	}

	planTasks := make([]PlanTask, 0, len(tasks))
	repos := map[string]bool{}
	for _, task := range tasks {
		planTasks = append(planTasks, PlanTask{
			ID:           task.ID,
			Title:        task.Title,
			Repository:   task.Repository,
			Goal:         task.Goal,
			Dependencies: task.Dependencies,
			Verification: task.Verification,
		})
		if task.Repository != "" {
			repos[task.Repository] = true
		}
	}
	if len(acs) == 0 {
		for i, task := range tasks {
			acs = append(acs, PlanAcceptanceCriterion{
				ID:                  fmt.Sprintf("AC%03d", i+1),
				Description:         task.Title,
				ImplementationTasks: []string{task.ID},
				PlannedVerification: []string{"task-verification"},
			})
		}
	}

	planRepos := []PlanRepository{}
	for id := range repos {
		planRepos = append(planRepos, PlanRepository{ID: id, Relevant: true})
	}

	byRepo := map[string][]string{}
	for _, task := range tasks {
		if task.Repository == "" {
			continue
		}
		cmds := []string{}
		for _, v := range task.Verification {
			if v.Command != "" {
				cmds = append(cmds, v.Command)
			}
		}
		if len(cmds) == 0 {
			cmds = []string{"compile", "focused tests"}
		}
		byRepo[task.Repository] = cmds
	}

	if feature.ID == "" {
		feature.ID = "feature"
	}
	if feature.Title == "" {
		feature.Title = "Feature implementation plan"
	}

	return PlanDocument{
		SchemaVersion: "1.0",
		Feature:       feature,
		Requirements:  reqs,
		AcceptanceCriteria: acs,
		Tasks:         planTasks,
		Repositories:  planRepos,
		Verification: PlanVerificationStrategy{
			Strategy: "repository-aware",
			ByRepo:   byRepo,
		},
		Approval: PlanApprovalBlock{Required: true},
	}
}
