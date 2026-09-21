package orchestrator

import (
	"fmt"
	"strings"
	"time"
)

type TransitionSpec struct {
	TaskID          string
	To              TaskStatus
	Reason          string
	Actor           string
	Command         string
	Event           string
	Message         string
	VerifierCommand string
	SetBlock        *BlockMeta
	ClearBlock      bool
	ClaimLease      bool
	ClearLease      bool
	HeartbeatLease  bool
	SkipPersist     bool // mutate + audit only; caller saves (batch)
	Tasks           []Task
	Workflow        *WorkflowState
}

type TransitionResult struct {
	Tasks    []Task
	Event    TaskSummaryRecord
	Index    int
	Workflow WorkflowState
}

// ApplyTransition validates a status change, updates block/lease metadata, audits, and persists.
func ApplyTransition(workspaceRoot string, tasks []Task, spec TransitionSpec) (TransitionResult, error) {
	var result TransitionResult
	run := func() error {
		ws, err := LoadWorkflowState(workspaceRoot)
		if err != nil {
			return err
		}
		if spec.Workflow != nil {
			ws = *spec.Workflow
		}

		diskTasks, err := LoadTasks(workspaceRoot)
		if err != nil {
			return err
		}
		if len(diskTasks) > 0 {
			tasks = diskTasks
		} else if spec.Tasks != nil {
			tasks = spec.Tasks
		} else if len(tasks) == 0 {
			return fmt.Errorf("no tasks available")
		}

		idx := findTaskIndex(tasks, spec.TaskID)
		if idx == -1 {
			return fmt.Errorf("task %s not found", spec.TaskID)
		}
		prev := tasks[idx].Status
		if prev != spec.To && !prev.CanTransitionTo(spec.To) {
			return fmt.Errorf("illegal transition: %s -> %s", prev, spec.To)
		}

		now := time.Now().UTC()
		if spec.ClaimLease {
			for _, t := range tasks {
				if t.ID != spec.TaskID && IsActiveStatus(t.Status) {
					return NewCodedError(ErrCodeWorkflowBusy, fmt.Sprintf("task %s is already active (%s)", t.ID, t.Status))
				}
			}
			if spec.To == StatusRunning {
				if prev == StatusRunning {
					return NewCodedError(ErrCodeWorkflowBusy, fmt.Sprintf("task %s is already RUNNING", spec.TaskID))
				}
				if LeaseHeld(ws, now) && ws.ActiveTaskID != "" && ws.ActiveTaskID != spec.TaskID {
					return NewCodedError(ErrCodeWorkflowBusy, fmt.Sprintf("active task %s holds execution lease", ws.ActiveTaskID))
				}
				if LeaseHeld(ws, now) && ws.ActiveTaskID == spec.TaskID {
					return NewCodedError(ErrCodeWorkflowBusy, fmt.Sprintf("task %s already holds execution lease", spec.TaskID))
				}
			}
			if err := ClaimExecution(&ws, spec.TaskID, spec.Actor, DefaultLeaseTTL, now); err != nil {
				return err
			}
		}
		if spec.HeartbeatLease {
			if err := HeartbeatLease(&ws, spec.TaskID, DefaultLeaseTTL, now); err != nil {
				return err
			}
		}
		if spec.ClearLease {
			ClearExecutionLease(&ws)
			ClearSchedulingPause(&ws)
		}

		tasks[idx].Status = spec.To
		tasks[idx].UpdatedAt = now

		if spec.SetBlock != nil {
			meta := *spec.SetBlock
			if meta.BlockedAt.IsZero() {
				meta.BlockedAt = now
			}
			tasks[idx].BlockedReason = meta.Reason
			tasks[idx].BlockedBy = append([]string{}, meta.BlockedBy...)
			blockedAt := meta.BlockedAt
			tasks[idx].BlockedAt = &blockedAt
			tasks[idx].RecoveryCommand = meta.RecoveryCommand
			tasks[idx].BlockKind = meta.BlockKind
		} else if spec.ClearBlock || prev == StatusBlocked {
			tasks[idx].BlockedReason = ""
			tasks[idx].BlockedBy = nil
			tasks[idx].BlockedAt = nil
			tasks[idx].RecoveryCommand = ""
			tasks[idx].BlockKind = ""
		}

		eventName := strings.TrimSpace(spec.Event)
		if eventName == "" {
			eventName = "status_transition"
		}
		message := strings.TrimSpace(spec.Message)
		if message == "" {
			message = fmt.Sprintf("status %s -> %s", prev, spec.To)
		}
		reason := strings.TrimSpace(spec.Reason)
		if reason == "" {
			reason = message
		}

		entry := TaskSummaryRecord{
			Timestamp:        now,
			TaskID:           tasks[idx].ID,
			Repository:       tasks[idx].Repository,
			Status:           tasks[idx].Status,
			PreviousStatus:   prev,
			NextStatus:       spec.To,
			Event:            eventName,
			Message:          message,
			Reason:           reason,
			Actor:            strings.TrimSpace(spec.Actor),
			Command:          strings.TrimSpace(spec.Command),
			WorkflowRevision: ws.CurrentPlanRevision,
			VerifierCommand:  strings.TrimSpace(spec.VerifierCommand),
		}
		if err := appendTaskSummaryRecord(workspaceRoot, entry); err != nil {
			return err
		}
		if !spec.SkipPersist {
			if err := SaveTasks(workspaceRoot, tasks); err != nil {
				return err
			}
			if spec.ClaimLease || spec.ClearLease || spec.HeartbeatLease || spec.Workflow != nil {
				if err := SaveWorkflowState(workspaceRoot, ws); err != nil {
					return err
				}
			}
		}
		result = TransitionResult{Tasks: tasks, Event: entry, Index: idx, Workflow: ws}
		if spec.Workflow != nil {
			*spec.Workflow = ws
		}
		return nil
	}

	if spec.SkipPersist {
		return result, run()
	}
	err := WithStateLock(workspaceRoot, run)
	return result, err
}

func ApplyTransitionInMemory(task *Task, to TaskStatus, setBlock *BlockMeta, clearBlock bool, now time.Time) error {
	prev := task.Status
	if prev != to && !prev.CanTransitionTo(to) {
		return fmt.Errorf("illegal transition: %s -> %s", prev, to)
	}
	task.Status = to
	task.UpdatedAt = now
	if setBlock != nil {
		meta := *setBlock
		if meta.BlockedAt.IsZero() {
			meta.BlockedAt = now
		}
		task.BlockedReason = meta.Reason
		task.BlockedBy = append([]string{}, meta.BlockedBy...)
		blockedAt := meta.BlockedAt
		task.BlockedAt = &blockedAt
		task.RecoveryCommand = meta.RecoveryCommand
		task.BlockKind = meta.BlockKind
	} else if clearBlock || prev == StatusBlocked {
		task.BlockedReason = ""
		task.BlockedBy = nil
		task.BlockedAt = nil
		task.RecoveryCommand = ""
		task.BlockKind = ""
	}
	return nil
}
