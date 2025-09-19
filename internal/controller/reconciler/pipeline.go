package reconciler

import (
	"context"

	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	scorev1b1 "github.com/cappyzawa/score-orchestrator/api/v1b1"
	"github.com/cappyzawa/score-orchestrator/internal/controller/managers"
	"github.com/cappyzawa/score-orchestrator/internal/controller/phases"
	"github.com/cappyzawa/score-orchestrator/internal/reconcile"
)

// WorkloadPipeline implements the phase-based reconciliation pipeline
type WorkloadPipeline struct {
	client        client.Client
	recorder      record.EventRecorder
	claimManager  *managers.ClaimManager
	planManager   *managers.PlanManager
	statusManager *managers.StatusManager

	// Phases for normal reconciliation
	normalPhases []phases.Phase
	// Phase for deletion
	deletionPhase phases.Phase
}

// NewWorkloadPipeline creates a new WorkloadPipeline
func NewWorkloadPipeline(
	k8sClient client.Client,
	recorder record.EventRecorder,
	claimManager *managers.ClaimManager,
	planManager *managers.PlanManager,
	statusManager *managers.StatusManager,
) *WorkloadPipeline {
	return &WorkloadPipeline{
		client:        k8sClient,
		recorder:      recorder,
		claimManager:  claimManager,
		planManager:   planManager,
		statusManager: statusManager,
		normalPhases: []phases.Phase{
			&phases.ValidationPhase{},
			&phases.ClaimPhase{},
			&phases.PlanPhase{},
			&phases.StatusPhase{},
		},
		deletionPhase: &phases.DeletionPhase{},
	}
}

// Execute runs the reconciliation pipeline
func (p *WorkloadPipeline) Execute(ctx context.Context, workload *scorev1b1.Workload) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx).WithValues("pipeline", "workload")
	log.V(1).Info("Starting pipeline execution", "generation", workload.Generation, "resourceVersion", workload.ResourceVersion)

	// Create phase context
	phaseCtx := &phases.PhaseContext{
		Client:        p.client,
		Workload:      workload,
		Logger:        log,
		Recorder:      p.recorder,
		ClaimManager:  p.claimManager,
		PlanManager:   p.planManager,
		StatusManager: p.statusManager,
	}

	// Handle deletion vs normal reconciliation
	if !workload.DeletionTimestamp.IsZero() {
		log.V(1).Info("Workload is being deleted, executing deletion pipeline")
		return p.executeDeletionPipeline(ctx, phaseCtx)
	}

	// Ensure finalizer for normal reconciliation
	if err := reconcile.EnsureFinalizer(ctx, p.client, workload); err != nil {
		log.Error(err, "Failed to ensure finalizer")
		return ctrl.Result{}, err
	}

	log.V(1).Info("Executing normal reconciliation pipeline")
	return p.executeNormalPipeline(ctx, phaseCtx)
}

// executeNormalPipeline executes the normal reconciliation phases
func (p *WorkloadPipeline) executeNormalPipeline(ctx context.Context, phaseCtx *phases.PhaseContext) (ctrl.Result, error) {
	log := phaseCtx.Logger.WithValues("pipeline", "normal")

	for _, phase := range p.normalPhases {
		// Check if phase should be skipped
		if phase.ShouldSkip(ctx, phaseCtx) {
			log.V(1).Info("Skipping phase", "phase", phase.Name())
			continue
		}

		log.V(1).Info("Executing phase", "phase", phase.Name())
		result := phase.Execute(ctx, phaseCtx)

		// Handle phase result
		if result.Error != nil {
			log.Error(result.Error, "Phase execution failed", "phase", phase.Name())
			return ctrl.Result{}, result.Error
		}

		if result.Requeue {
			log.V(1).Info("Phase requested requeue", "phase", phase.Name(), "after", result.RequeueAfter)
			return ctrl.Result{
				Requeue:      true,
				RequeueAfter: result.RequeueAfter,
			}, nil
		}

		if result.Skip {
			log.V(1).Info("Phase requested to skip remaining phases", "phase", phase.Name())
			break
		}

		log.V(1).Info("Phase completed successfully", "phase", phase.Name())
	}

	log.V(1).Info("Normal pipeline execution completed")
	return ctrl.Result{}, nil
}

// executeDeletionPipeline executes the deletion phase
func (p *WorkloadPipeline) executeDeletionPipeline(ctx context.Context, phaseCtx *phases.PhaseContext) (ctrl.Result, error) {
	log := phaseCtx.Logger.WithValues("pipeline", "deletion")

	if p.deletionPhase.ShouldSkip(ctx, phaseCtx) {
		log.V(1).Info("Deletion phase skipped")
		return ctrl.Result{}, nil
	}

	log.V(1).Info("Executing deletion phase", "phase", p.deletionPhase.Name())
	result := p.deletionPhase.Execute(ctx, phaseCtx)

	// Handle phase result
	if result.Error != nil {
		log.Error(result.Error, "Deletion phase execution failed", "phase", p.deletionPhase.Name())
		return ctrl.Result{}, result.Error
	}

	if result.Requeue {
		log.V(1).Info("Deletion phase requested requeue", "phase", p.deletionPhase.Name(), "after", result.RequeueAfter)
		return ctrl.Result{
			Requeue:      true,
			RequeueAfter: result.RequeueAfter,
		}, nil
	}

	log.V(1).Info("Deletion pipeline execution completed")
	return ctrl.Result{}, nil
}
