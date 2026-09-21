package orchestrator

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

const DefaultLeaseTTL = 30 * time.Minute

func IsActiveStatus(status TaskStatus) bool {
	return status == StatusRunning || status == StatusImplemented || status == StatusVerifying
}

func ActiveTasks(tasks []Task) []Task {
	out := make([]Task, 0)
	for _, t := range tasks {
		if IsActiveStatus(t.Status) {
			out = append(out, t)
		}
	}
	return out
}

func HasActiveTask(tasks []Task) bool {
	return len(ActiveTasks(tasks)) > 0
}

func LeaseExpired(ws WorkflowState, now time.Time) bool {
	if ws.ActiveTaskID == "" || ws.ActiveLeaseExpiresAt == nil {
		return false
	}
	return !ws.ActiveLeaseExpiresAt.After(now)
}

func LeaseHeld(ws WorkflowState, now time.Time) bool {
	if ws.ActiveTaskID == "" {
		return false
	}
	if ws.ActiveLeaseExpiresAt == nil {
		return true
	}
	return ws.ActiveLeaseExpiresAt.After(now)
}

func newLeaseID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func ClaimExecution(ws *WorkflowState, taskID, claimedBy string, ttl time.Duration, now time.Time) error {
	if ttl <= 0 {
		ttl = DefaultLeaseTTL
	}
	if ws.SchedulingPausedReason != "" {
		return NewCodedError(ErrCodeSchedulingPaused, ws.SchedulingPausedReason)
	}
	if LeaseHeld(*ws, now) && ws.ActiveTaskID != taskID {
		return NewCodedError(ErrCodeWorkflowBusy, fmt.Sprintf("active task %s holds execution lease", ws.ActiveTaskID))
	}
	if LeaseExpired(*ws, now) && ws.ActiveTaskID != "" && ws.ActiveTaskID != taskID {
		return NewCodedError(ErrCodeLeaseExpired, fmt.Sprintf("lease for %s expired; run feature-dev task recover %s", ws.ActiveTaskID, ws.ActiveTaskID))
	}
	expires := now.Add(ttl)
	claimed := now
	ws.ActiveTaskID = taskID
	ws.ActiveLeaseID = newLeaseID()
	ws.ActiveLeaseExpiresAt = &expires
	ws.ActiveClaimedAt = &claimed
	ws.ActiveClaimedBy = claimedBy
	return nil
}

func HeartbeatLease(ws *WorkflowState, taskID string, ttl time.Duration, now time.Time) error {
	if ttl <= 0 {
		ttl = DefaultLeaseTTL
	}
	if ws.ActiveTaskID == "" {
		return ClaimExecution(ws, taskID, "heartbeat", ttl, now)
	}
	if ws.ActiveTaskID != taskID {
		return NewCodedError(ErrCodeWorkflowBusy, fmt.Sprintf("lease owned by %s, not %s", ws.ActiveTaskID, taskID))
	}
	expires := now.Add(ttl)
	ws.ActiveLeaseExpiresAt = &expires
	return nil
}

func ClearExecutionLease(ws *WorkflowState) {
	ws.ActiveTaskID = ""
	ws.ActiveLeaseID = ""
	ws.ActiveLeaseExpiresAt = nil
	ws.ActiveClaimedAt = nil
	ws.ActiveClaimedBy = ""
}

func PauseScheduling(ws *WorkflowState, reason string) {
	ws.SchedulingPausedReason = reason
}

func ClearSchedulingPause(ws *WorkflowState) {
	ws.SchedulingPausedReason = ""
}
