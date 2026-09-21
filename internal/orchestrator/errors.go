package orchestrator

import (
	"errors"
	"fmt"
)

const (
	ErrCodeWorkflowBusy         = "workflow_busy"
	ErrCodeLeaseExpired         = "lease_expired"
	ErrCodeInvariantViolation   = "invariant_violation"
	ErrCodeTaskNotExecutable    = "task_not_executable"
	ErrCodeAlreadyDone          = "already_done"
	ErrCodeSchedulingPaused     = "scheduling_paused"
	ErrCodeVerificationCmdMissing = "verification_command_unavailable"
)

type CodedError struct {
	Code    string
	Message string
	Err     error
}

func (e *CodedError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *CodedError) Unwrap() error {
	return e.Err
}

func NewCodedError(code, message string) *CodedError {
	return &CodedError{Code: code, Message: message}
}

func IsCodedError(err error, code string) bool {
	var ce *CodedError
	if errors.As(err, &ce) {
		return ce.Code == code
	}
	return false
}

func CodedErrorCode(err error) string {
	var ce *CodedError
	if errors.As(err, &ce) {
		return ce.Code
	}
	return ""
}
