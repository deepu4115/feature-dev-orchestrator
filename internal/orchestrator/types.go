package orchestrator

import "time"

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

type TaskStatus string

const (
	StatusPlanned     TaskStatus = "PLANNED"
	StatusReady       TaskStatus = "READY"
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
	Verification       []VerificationStep `json:"verification,omitempty" yaml:"verification,omitempty"`
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
	Timestamp  time.Time  `json:"timestamp" yaml:"timestamp"`
	TaskID     string     `json:"task_id" yaml:"task_id"`
	Repository string     `json:"repository" yaml:"repository"`
	Status     TaskStatus `json:"status" yaml:"status"`
	Event      string     `json:"event" yaml:"event"`
	Message    string     `json:"message" yaml:"message"`
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
		StatusPlanned: {
			StatusReady:   true,
			StatusBlocked: true,
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
