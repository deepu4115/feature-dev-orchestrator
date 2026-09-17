package orchestrator

import (
	"embed"
	"fmt"
	"io/fs"
	"strings"
)

//go:embed templates/*
var embeddedTemplates embed.FS

var schemaArtifacts = map[string]schemaArtifact{
	"tasks": {
		TemplateFile: "templates/tasks.json",
		OutputPath:   func(ws string) string { return TaskStoragePath(ws) },
		Description:  "Task DAG stored as a top-level JSON array in .feature/tasks/tasks.json",
		RootKey:      "(top-level array)",
		RequiredFields: []string{
			"id (string)", "title (string)", "repository (string)",
			"verification (array of {command, name?})",
			"verification_working_directory (repository_root|workspace_root|custom, default repository_root)",
			"requirement_ids (string array)",
		},
	},
	"requirements": {
		TemplateFile: "templates/requirements.json",
		OutputPath:   PlanDraftRequirementsPath,
		Description:  "Planning draft requirements with wrapper object",
		RootKey:      "requirements",
		RequiredFields: []string{
			"requirements (array of {id, description})",
		},
	},
	"assumptions": {
		TemplateFile: "templates/assumptions.json",
		OutputPath:   PlanDraftAssumptionsPath,
		Description:  "Planning draft assumptions with wrapper object",
		RootKey:      "assumptions",
		RequiredFields: []string{
			"assumptions (array of {id, statement, confidence, impact?, evidence?, affected_tasks?})",
		},
	},
	"risks": {
		TemplateFile: "templates/risks.json",
		OutputPath:   PlanDraftRisksPath,
		Description:  "Planning draft risks with wrapper object",
		RootKey:      "risks",
		RequiredFields: []string{
			"risks (array of {id, level, description, type?, mitigation?})",
		},
	},
	"impact": {
		TemplateFile: "templates/impact.json",
		OutputPath:   PlanDraftImpactPath,
		Description:  "Planning draft impact with wrapper object",
		RootKey:      "impact",
		RequiredFields: []string{
			"impact (array of {repository, areas})",
		},
	},
	"repo-analysis": {
		TemplateFile: "templates/repo-analysis.json",
		OutputPath:   PlanDraftRepoAnalysisPath,
		Description:  "Planning draft repository analysis with wrapper object",
		RootKey:      "repositories",
		RequiredFields: []string{
			"repositories (array of {repository, evidence})",
		},
	},
}

type schemaArtifact struct {
	TemplateFile   string
	OutputPath     func(workspaceRoot string) string
	Description    string
	RootKey        string
	RequiredFields []string
}

func ListSchemaArtifacts() []string {
	names := make([]string, 0, len(schemaArtifacts))
	for name := range schemaArtifacts {
		names = append(names, name)
	}
	return names
}

func ReadEmbeddedTemplate(name string) ([]byte, error) {
	artifact, ok := schemaArtifacts[name]
	if !ok {
		return nil, fmt.Errorf("unknown schema artifact: %s (valid: %s)", name, strings.Join(ListSchemaArtifacts(), ", "))
	}
	data, err := fs.ReadFile(embeddedTemplates, artifact.TemplateFile)
	if err != nil {
		return nil, fmt.Errorf("read embedded template %s: %w", name, err)
	}
	return data, nil
}

func SchemaShow(name string) (map[string]any, error) {
	artifact, ok := schemaArtifacts[name]
	if !ok {
		return nil, fmt.Errorf("unknown schema artifact: %s (valid: %s)", name, strings.Join(ListSchemaArtifacts(), ", "))
	}
	example, err := ReadEmbeddedTemplate(name)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"artifact":        name,
		"description":     artifact.Description,
		"root_key":        artifact.RootKey,
		"required_fields": artifact.RequiredFields,
		"example":         strings.TrimSpace(string(example)),
		"init_command":    schemaInitCommand(name),
		"show_command":    fmt.Sprintf("feature-dev schema show %s --json", name),
	}, nil
}

func schemaInitCommand(name string) string {
	switch name {
	case "tasks":
		return "feature-dev task init"
	default:
		return "feature-dev plan draft init"
	}
}

func InitPlanningDraftFromTemplates(workspaceRoot string, force bool) ([]string, error) {
	if err := EnsureDir(PlanDraftDir(workspaceRoot)); err != nil {
		return nil, err
	}
	written := []string{}
	for name, artifact := range schemaArtifacts {
		if name == "tasks" {
			continue
		}
		dest := artifact.OutputPath(workspaceRoot)
		if Exists(dest) && !force {
			continue
		}
		data, err := ReadEmbeddedTemplate(name)
		if err != nil {
			return written, err
		}
		if err := WriteFileAtomically(dest, append(data, '\n')); err != nil {
			return written, fmt.Errorf("write %s: %w", dest, err)
		}
		written = append(written, dest)
	}
	return written, nil
}

func InitTasksFromTemplate(workspaceRoot string, force bool) (string, error) {
	if err := EnsureDir(TasksDir(workspaceRoot)); err != nil {
		return "", err
	}
	dest := TaskStoragePath(workspaceRoot)
	if Exists(dest) && !force {
		return "", fmt.Errorf("%s already exists (use --force to overwrite)", dest)
	}
	data, err := ReadEmbeddedTemplate("tasks")
	if err != nil {
		return "", err
	}
	if err := WriteFileAtomically(dest, append(data, '\n')); err != nil {
		return "", err
	}
	return dest, nil
}

func WritePlanningBundleFromTemplates(workspaceRoot string, force bool) ([]string, error) {
	return InitPlanningDraftFromTemplates(workspaceRoot, force)
}
