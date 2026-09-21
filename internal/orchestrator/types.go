package orchestrator

import (
	"errors"
	"time"
)

const (
	DefaultFeatureDir = ".feature"
	DefaultConfigFile = "config.yaml"
	DefaultRepoFile   = "repositories.json"
)

type WorkspaceConfig struct {
	SchemaVersion       string                    `json:"schema_version" yaml:"schema_version"`
	RepositoryDiscovery RepositoryDiscoveryConfig `json:"repository_discovery" yaml:"repository_discovery"`
}

type RepositoryDiscoveryConfig struct {
	Mode    string   `json:"mode" yaml:"mode"`
	Include []string `json:"include" yaml:"include"`
	Exclude []string `json:"exclude" yaml:"exclude"`
}

type Repository struct {
	ID      string `json:"id" yaml:"id"`
	Path    string `json:"path" yaml:"path"`
	GitRoot string `json:"git_root" yaml:"git_root"`
	Mode    string `json:"mode" yaml:"mode"`
}

type RepositoryState struct {
	ID             string    `json:"id" yaml:"id"`
	Path           string    `json:"path" yaml:"path"`
	GitRoot        string    `json:"git_root" yaml:"git_root"`
	Branch         string    `json:"branch" yaml:"branch"`
	Head           string    `json:"head" yaml:"head"`
	Status         string    `json:"status" yaml:"status"`
	ChangedFiles   []string  `json:"changed_files" yaml:"changed_files"`
	UntrackedFiles []string  `json:"untracked_files" yaml:"untracked_files"`
	DiffSummary    string    `json:"diff_summary" yaml:"diff_summary"`
	MergeState     string    `json:"merge_state" yaml:"merge_state"`
	UpdatedAt      time.Time `json:"updated_at" yaml:"updated_at"`
}

type VerificationResult struct {
	RepositoryID    string    `json:"repository_id" yaml:"repository_id"`
	WorkingDir      string    `json:"working_directory" yaml:"working_directory"`
	BuildSystem     string    `json:"build_system" yaml:"build_system"`
	Command         string    `json:"command" yaml:"command"`
	TimeoutSeconds  int       `json:"timeout_seconds" yaml:"timeout_seconds"`
	ExitCode        int       `json:"exit_code" yaml:"exit_code"`
	Output          string    `json:"output" yaml:"output"`
	TaskID          string    `json:"task_id,omitempty" yaml:"task_id,omitempty"`
	VerificationURL string    `json:"verification_url,omitempty" yaml:"verification_url,omitempty"`
	ExecutedAt      time.Time `json:"executed_at" yaml:"executed_at"`
}

type WorkflowStatus string

const (
	WorkflowNew            WorkflowStatus = "NEW"
	WorkflowPlanning       WorkflowStatus = "PLANNING"
	WorkflowPlanGenerated  WorkflowStatus = "PLAN_GENERATED"
	WorkflowReviewPending       WorkflowStatus = "REVIEW_PENDING"
	WorkflowClarificationNeeded WorkflowStatus = "CLARIFICATION_NEEDED"
	WorkflowReplanning          WorkflowStatus = "REPLANNING"
	WorkflowRejected       WorkflowStatus = "REJECTED"
	WorkflowApproved       WorkflowStatus = "APPROVED"
	WorkflowExecuting      WorkflowStatus = "EXECUTING"
	WorkflowVerifying      WorkflowStatus = "VERIFYING"
	WorkflowRework         WorkflowStatus = "REWORK"
	WorkflowBlocked        WorkflowStatus = "BLOCKED"
	WorkflowFailed         WorkflowStatus = "FAILED"
	WorkflowCompleted      WorkflowStatus = "COMPLETED"
	WorkflowCancelled      WorkflowStatus = "CANCELLED"
)

var (
	ErrPlanNotApproved        = errors.New("current feature plan has not been approved")
	ErrPlanRevisionMismatch   = errors.New("approved plan revision does not match current revision")
	ErrStaleRevisionApproval  = errors.New("cannot approve stale plan revision")
	ErrPlanNotReviewPending   = errors.New("plan is not in REVIEW_PENDING state")
	ErrPlanValidationFailed   = errors.New("plan validation failed")
	ErrPlanFingerprintMismatch = errors.New("plan fingerprint does not match approved fingerprint")
)

type ApprovalHistoryEntry struct {
	Revision    int            `json:"revision"`
	Status      string         `json:"status"`
	Reason      string         `json:"reason,omitempty"`
	Fingerprint string         `json:"fingerprint,omitempty"`
	Timestamp   time.Time      `json:"timestamp"`
}

type WorkflowState struct {
	SchemaVersion            string                 `json:"schema_version"`
	CurrentPlanRevision      int                    `json:"current_plan_revision"`
	ApprovedPlanRevision     *int                   `json:"approved_plan_revision"`
	ApprovedPlanFingerprint  string                 `json:"approved_plan_fingerprint,omitempty"`
	WorkflowStatus           WorkflowStatus         `json:"workflow_status"`
	ApprovalHistory          []ApprovalHistoryEntry `json:"approval_history,omitempty"`
	CoverageDeferralReason   string                 `json:"coverage_deferral_reason,omitempty"`
	ActiveTaskID             string                 `json:"active_task_id,omitempty"`
	ActiveLeaseID            string                 `json:"active_lease_id,omitempty"`
	ActiveLeaseExpiresAt     *time.Time             `json:"active_lease_expires_at,omitempty"`
	ActiveClaimedAt          *time.Time             `json:"active_claimed_at,omitempty"`
	ActiveClaimedBy          string                 `json:"active_claimed_by,omitempty"`
	SchedulingPausedReason   string                 `json:"scheduling_paused_reason,omitempty"`
	UpdatedAt                time.Time              `json:"updated_at"`
}

const (
	BlockKindDependency  = "dependency"
	BlockKindStaleRunning = "stale_running"
	BlockKindMissingRepo = "missing_repo"
	BlockKindManual      = "manual"
	BlockKindLeaseExpired = "lease_expired"
)

type BlockMeta struct {
	Reason          string    `json:"blocked_reason,omitempty"`
	BlockedBy       []string  `json:"blocked_by,omitempty"`
	BlockedAt       time.Time `json:"blocked_at,omitempty"`
	RecoveryCommand string    `json:"recovery_command,omitempty"`
	BlockKind       string    `json:"block_kind,omitempty"`
}

type TaskStatus string

const (
	StatusDraft         TaskStatus = "DRAFT"
	StatusReviewPending TaskStatus = "REVIEW_PENDING"
	StatusPlanned       TaskStatus = "PLANNED"
	StatusReady         TaskStatus = "READY"
	StatusRunning     TaskStatus = "RUNNING"
	StatusImplemented TaskStatus = "IMPLEMENTED"
	StatusVerifying   TaskStatus = "VERIFYING"
	StatusDone        TaskStatus = "DONE"
	StatusBlocked     TaskStatus = "BLOCKED"
	StatusFailed      TaskStatus = "FAILED"
	StatusRework      TaskStatus = "REWORK"
)

type VerificationStep struct {
	Name    string `json:"name,omitempty" yaml:"name,omitempty"`
	Command string `json:"command" yaml:"command"`
}

type Task struct {
	ID                 string             `json:"id" yaml:"id"`
	Title              string             `json:"title" yaml:"title"`
	Repository         string             `json:"repository" yaml:"repository"`
	WorkingDirectory   string             `json:"working_directory,omitempty" yaml:"working_directory,omitempty"`
	Status             TaskStatus         `json:"status" yaml:"status"`
	Goal               string             `json:"goal,omitempty" yaml:"goal,omitempty"`
	Dependencies       []string           `json:"dependencies,omitempty" yaml:"dependencies,omitempty"`
	ExpectedFiles      []string           `json:"expected_files,omitempty" yaml:"expected_files,omitempty"`
	ContextRefs        []string           `json:"context_refs,omitempty" yaml:"context_refs,omitempty"`
	AcceptanceCriteria []string           `json:"acceptance_criteria,omitempty" yaml:"acceptance_criteria,omitempty"`
	Verification                 []VerificationStep `json:"verification,omitempty" yaml:"verification,omitempty"`
	VerificationWorkingDirectory string             `json:"verification_working_directory,omitempty" yaml:"verification_working_directory,omitempty"`
	VerificationStale  bool               `json:"verification_stale,omitempty" yaml:"verification_stale,omitempty"`
	RepositoryRationale string            `json:"repository_rationale,omitempty" yaml:"repository_rationale,omitempty"`
	OwnershipConfidence string            `json:"ownership_confidence,omitempty" yaml:"ownership_confidence,omitempty"`
	RequirementIDs     []string           `json:"requirement_ids,omitempty" yaml:"requirement_ids,omitempty"`
	PlannedVerification []string          `json:"planned_verification,omitempty" yaml:"planned_verification,omitempty"`
	BlockedReason      string             `json:"blocked_reason,omitempty" yaml:"blocked_reason,omitempty"`
	BlockedBy          []string           `json:"blocked_by,omitempty" yaml:"blocked_by,omitempty"`
	BlockedAt          *time.Time         `json:"blocked_at,omitempty" yaml:"blocked_at,omitempty"`
	RecoveryCommand    string             `json:"recovery_command,omitempty" yaml:"recovery_command,omitempty"`
	BlockKind          string             `json:"block_kind,omitempty" yaml:"block_kind,omitempty"`
	UpdatedAt          time.Time          `json:"updated_at,omitempty" yaml:"updated_at,omitempty"`
}

type ContextBudget struct {
	MaxFiles      int `json:"max_files" yaml:"max_files"`
	MaxTotalChars int `json:"max_total_chars" yaml:"max_total_chars"`
	MaxFileChars  int `json:"max_file_chars" yaml:"max_file_chars"`
}

type ContextSnippet struct {
	Path  string `json:"path" yaml:"path"`
	Chars int    `json:"chars" yaml:"chars"`
	Text  string `json:"text" yaml:"text"`
}

type ContextPayload struct {
	TaskID             string              `json:"task_id" yaml:"task_id"`
	Repository         string              `json:"repository" yaml:"repository"`
	Goal               string              `json:"goal,omitempty" yaml:"goal,omitempty"`
	AcceptanceCriteria []string            `json:"acceptance_criteria,omitempty" yaml:"acceptance_criteria,omitempty"`
	Verification       []VerificationStep  `json:"verification,omitempty" yaml:"verification,omitempty"`
	Branch             string              `json:"branch,omitempty" yaml:"branch,omitempty"`
	Head               string              `json:"head,omitempty" yaml:"head,omitempty"`
	ChangedFiles       []string            `json:"changed_files,omitempty" yaml:"changed_files,omitempty"`
	Summaries          []TaskSummaryRecord `json:"summaries,omitempty" yaml:"summaries,omitempty"`
	Snippets           []ContextSnippet    `json:"snippets,omitempty" yaml:"snippets,omitempty"`
	EstimatedTokens    int                 `json:"estimated_tokens" yaml:"estimated_tokens"`
	Budget             ContextBudget       `json:"budget" yaml:"budget"`
	Level              string              `json:"level" yaml:"level"`
}

type TaskSummaryRecord struct {
	Timestamp         time.Time  `json:"timestamp" yaml:"timestamp"`
	TaskID            string     `json:"task_id" yaml:"task_id"`
	Repository        string     `json:"repository" yaml:"repository"`
	Status            TaskStatus `json:"status" yaml:"status"`
	PreviousStatus    TaskStatus `json:"previous_status,omitempty" yaml:"previous_status,omitempty"`
	NextStatus        TaskStatus `json:"next_status,omitempty" yaml:"next_status,omitempty"`
	Event             string     `json:"event" yaml:"event"`
	Message           string     `json:"message" yaml:"message"`
	Reason            string     `json:"reason,omitempty" yaml:"reason,omitempty"`
	Actor             string     `json:"actor,omitempty" yaml:"actor,omitempty"`
	Command           string     `json:"command,omitempty" yaml:"command,omitempty"`
	WorkflowRevision  int        `json:"workflow_revision,omitempty" yaml:"workflow_revision,omitempty"`
	VerifierCommand   string     `json:"verifier_command,omitempty" yaml:"verifier_command,omitempty"`
}

func DefaultConfig() WorkspaceConfig {
	return WorkspaceConfig{
		SchemaVersion: "1.0",
		RepositoryDiscovery: RepositoryDiscoveryConfig{
			Mode:    "auto",
			Include: []string{"./*"},
			Exclude: []string{".git", "node_modules", "dist", "build", "target"},
		},
	}
}

func (s TaskStatus) CanTransitionTo(next TaskStatus) bool {
	allowed := map[TaskStatus]map[TaskStatus]bool{
		StatusDraft: {
			StatusPlanned:       true,
			StatusReviewPending: true,
			StatusBlocked:       true,
		},
		StatusReviewPending: {
			StatusReady:   true,
			StatusPlanned: true,
			StatusBlocked: true,
		},
		StatusPlanned: {
			StatusReady:         true,
			StatusReviewPending: true,
			StatusBlocked:       true,
		},
		StatusReady: {
			StatusRunning: true,
			StatusBlocked: true,
		},
		StatusRunning: {
			StatusImplemented: true,
			StatusBlocked:     true,
			StatusFailed:      true,
		},
		StatusImplemented: {
			StatusVerifying: true,
			StatusRework:    true,
		},
		StatusVerifying: {
			StatusDone:   true,
			StatusFailed: true,
			StatusRework: true,
		},
		StatusBlocked: {
			StatusReady:   true,
			StatusPlanned: true,
		},
		StatusFailed: {
			StatusRework: true,
			StatusReady:  true,
		},
		StatusRework: {
			StatusReady:   true,
			StatusBlocked: true,
		},
		StatusDone: {
			StatusDone: true,
		},
	}
	return allowed[s][next]
}
