package phases

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	scorev1b1 "github.com/cappyzawa/score-orchestrator/api/v1b1"
	"github.com/cappyzawa/score-orchestrator/internal/controller/managers"
	"github.com/cappyzawa/score-orchestrator/internal/status"
)

// PhaseResult represents the result of a phase execution
type PhaseResult struct {
	// Skip indicates whether to skip remaining phases
	Skip bool
	// Requeue indicates whether to requeue the reconciliation
	Requeue bool
	// RequeueAfter indicates when to requeue the reconciliation
	RequeueAfter time.Duration
	// Error is the error that occurred during phase execution
	Error error
}

// PhaseContext contains shared context and data between phases
type PhaseContext struct {
	// Client is the Kubernetes client
	Client client.Client
	// Workload is the Workload being reconciled
	Workload *scorev1b1.Workload
	// Logger is the controller logger
	Logger logr.Logger
	// Recorder is the event recorder
	Recorder record.EventRecorder
	// Managers provide domain-specific operations
	ClaimManager  *managers.ClaimManager
	PlanManager   *managers.PlanManager
	StatusManager *managers.StatusManager

	// Phase-specific data
	Claims            []scorev1b1.ResourceClaim
	ClaimAgg          status.ClaimAggregation
	Plan              *scorev1b1.WorkloadPlan
	InputsValid       bool
	ValidationReason  string
	ValidationMessage string
}

// Phase represents a single phase in the reconciliation pipeline
type Phase interface {
	// Name returns the name of the phase for logging
	Name() string
	// Execute runs the phase logic
	Execute(ctx context.Context, phaseCtx *PhaseContext) PhaseResult
	// ShouldSkip determines if this phase should be skipped based on context
	ShouldSkip(ctx context.Context, phaseCtx *PhaseContext) bool
}

// Pipeline manages the execution of reconciliation phases
type Pipeline interface {
	// Execute runs all phases in order
	Execute(ctx context.Context, phaseCtx *PhaseContext) (ctrl.Result, error)
}
