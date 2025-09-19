package controller

import (
	"context"
	"fmt"
	"os"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	scorev1b1 "github.com/cappyzawa/score-orchestrator/api/v1b1"
	"github.com/cappyzawa/score-orchestrator/internal/config"
	"github.com/cappyzawa/score-orchestrator/internal/provisioner"
	"github.com/cappyzawa/score-orchestrator/internal/provisioner/strategy"
)

// Event constants for ProvisionerReconciler
const (
	EventReasonProvisioning      = "Provisioning"
	EventReasonProvisioned       = "Provisioned"
	EventReasonProvisionFailed   = "ProvisionFailed"
	EventReasonDeprovisioning    = "Deprovisioning"
	EventReasonDeprovisioned     = "Deprovisioned"
	EventReasonDeprovisionFailed = "DeprovisionFailed"
)

// ProvisionerReconciler reconciles ResourceClaim objects
type ProvisionerReconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	Recorder         record.EventRecorder
	ConfigLoader     config.ConfigLoader
	StrategySelector *strategy.Selector
	OutputManager    *provisioner.OutputManager
	LifecycleManager *ResourceClaimLifecycleManager
	supportedTypes   map[string]bool
}

// NewProvisionerReconciler creates a new ProvisionerReconciler
func NewProvisionerReconciler(
	k8sClient client.Client,
	scheme *runtime.Scheme,
	recorder record.EventRecorder,
	configLoader config.ConfigLoader,
) *ProvisionerReconciler {
	return &ProvisionerReconciler{
		Client:           k8sClient,
		Scheme:           scheme,
		Recorder:         recorder,
		ConfigLoader:     configLoader,
		StrategySelector: strategy.NewSelector(),
		OutputManager:    provisioner.NewOutputManager(),
		LifecycleManager: NewResourceClaimLifecycleManager(),
		supportedTypes:   make(map[string]bool),
	}
}

// SetupWithManager sets up the controller with the Manager
func (r *ProvisionerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Load supported types from environment or config
	r.loadSupportedTypes()

	// Load provisioning configuration
	r.loadProvisioningConfig()

	return ctrl.NewControllerManagedBy(mgr).
		For(&scorev1b1.ResourceClaim{}).
		WithEventFilter(predicate.NewPredicateFuncs(r.filterSupportedTypes)).
		Complete(r)
}

// Reconcile handles ResourceClaim reconciliation
// +kubebuilder:rbac:groups=score.dev,resources=resourceclaims,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=score.dev,resources=resourceclaims/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
func (r *ProvisionerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	// Fetch the ResourceClaim
	claim := &scorev1b1.ResourceClaim{}
	if err := r.Get(ctx, req.NamespacedName, claim); err != nil {
		log.Error(err, "Failed to fetch ResourceClaim")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Check if we should reconcile this claim
	if !r.LifecycleManager.ShouldReconcile(claim) {
		log.V(1).Info("Skipping reconciliation", "phase", claim.Status.Phase)
		return ctrl.Result{}, nil
	}

	log.Info("Reconciling ResourceClaim", "type", claim.Spec.Type, "phase", claim.Status.Phase)

	// Handle deletion
	if r.LifecycleManager.IsBeingDeleted(claim) {
		return r.handleDeletion(ctx, claim)
	}

	// Ensure finalizer is present
	if !r.LifecycleManager.HasFinalizer(claim) {
		r.LifecycleManager.AddFinalizer(claim)
		if err := r.Update(ctx, claim); err != nil {
			log.Error(err, "Failed to add finalizer")
			return ctrl.Result{}, err
		}
		log.V(1).Info("Added finalizer to ResourceClaim")
		// Set initial phase and requeue using GetReconcileResult for consistency
		r.LifecycleManager.SetPending(claim, ReasonClaimPending, "Initializing resource claim")
		// Update status after setting initial phase
		if statusErr := r.Status().Update(ctx, claim); statusErr != nil {
			log.Error(statusErr, "Failed to update ResourceClaim status after adding finalizer")
			return ctrl.Result{}, statusErr
		}
		return r.LifecycleManager.GetReconcileResult(ctx, claim, nil)
	}

	// Handle provisioning
	_, err := r.handleProvisioning(ctx, claim)

	// Update status
	if statusErr := r.Status().Update(ctx, claim); statusErr != nil {
		log.Error(statusErr, "Failed to update ResourceClaim status")
		if err == nil {
			err = statusErr
		}
	}

	return r.LifecycleManager.GetReconcileResult(ctx, claim, err)
}

// handleProvisioning handles the provisioning logic
func (r *ProvisionerReconciler) handleProvisioning(ctx context.Context, claim *scorev1b1.ResourceClaim) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	// Get strategy for this resource type
	provisioningStrategy, err := r.StrategySelector.GetStrategy(claim.Spec.Type)
	if err != nil {
		r.LifecycleManager.SetFailed(claim, ReasonClaimFailed, fmt.Sprintf("No strategy available: %v", err))
		r.Recorder.Event(claim, "Warning", EventReasonProvisionFailed, err.Error())
		return ctrl.Result{}, fmt.Errorf("strategy not found: %w", err)
	}

	// Handle phase transitions
	switch claim.Status.Phase {
	case "", scorev1b1.ResourceClaimPhasePending:
		return r.handlePendingPhase(ctx, claim, provisioningStrategy)
	case scorev1b1.ResourceClaimPhaseClaiming:
		return r.handleClaimingPhase(ctx, claim, provisioningStrategy)
	case scorev1b1.ResourceClaimPhaseBound:
		return r.handleBoundPhase(ctx, claim, provisioningStrategy)
	case scorev1b1.ResourceClaimPhaseFailed:
		return r.handleFailedPhase(ctx, claim, provisioningStrategy)
	default:
		log.Info("Unknown phase, resetting to Pending", "phase", claim.Status.Phase)
		r.LifecycleManager.SetPending(claim, ReasonClaimPending, "Unknown phase, resetting to Pending")
		return ctrl.Result{Requeue: true}, nil
	}
}

// handlePendingPhase handles the Pending phase
func (r *ProvisionerReconciler) handlePendingPhase(ctx context.Context, claim *scorev1b1.ResourceClaim, _ strategy.Strategy) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	log.Info("Starting provisioning", "type", claim.Spec.Type)
	r.LifecycleManager.SetClaiming(claim, ReasonBindingPending, "Starting resource provisioning")
	r.Recorder.Event(claim, "Normal", EventReasonProvisioning, "Starting resource provisioning")

	return ctrl.Result{Requeue: true}, nil
}

// handleClaimingPhase handles the Claiming phase
func (r *ProvisionerReconciler) handleClaimingPhase(ctx context.Context, claim *scorev1b1.ResourceClaim, provisioningStrategy strategy.Strategy) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	// Check current status from strategy
	phase, reason, message, err := provisioningStrategy.GetStatus(ctx, claim)
	if err != nil {
		log.Error(err, "Failed to get status from strategy")
		r.LifecycleManager.SetFailed(claim, ReasonClaimFailed, fmt.Sprintf("Status check failed: %v", err))
		return ctrl.Result{}, err
	}

	// If already bound, move to bound phase
	if phase == scorev1b1.ResourceClaimPhaseBound {
		outputs, err := provisioningStrategy.Provision(ctx, claim)
		if err != nil {
			log.Error(err, "Failed to get outputs from strategy")
			r.LifecycleManager.SetFailed(claim, ReasonBindingFailed, fmt.Sprintf("Failed to get outputs: %v", err))
			r.Recorder.Event(claim, "Warning", EventReasonProvisionFailed, err.Error())
			return ctrl.Result{}, err
		}

		// Validate outputs
		if err := r.OutputManager.ValidateOutputs(outputs); err != nil {
			log.Error(err, "Invalid outputs from strategy")
			r.LifecycleManager.SetFailed(claim, ReasonProjectionError, fmt.Sprintf("Invalid outputs: %v", err))
			return ctrl.Result{}, err
		}

		r.LifecycleManager.SetBound(claim, outputs)
		r.Recorder.Event(claim, "Normal", EventReasonProvisioned, "Resource successfully provisioned")
		log.Info("Resource provisioned successfully")
		return ctrl.Result{}, nil
	}

	// If failed, update to failed phase
	if phase == scorev1b1.ResourceClaimPhaseFailed {
		r.LifecycleManager.SetFailed(claim, reason, message)
		r.Recorder.Event(claim, "Warning", EventReasonProvisionFailed, message)
		return ctrl.Result{}, fmt.Errorf("provisioning failed: %s", message)
	}

	// Continue claiming
	log.V(1).Info("Provisioning in progress", "reason", reason, "message", message)
	return ctrl.Result{Requeue: true}, nil
}

// handleBoundPhase handles the Bound phase
func (r *ProvisionerReconciler) handleBoundPhase(ctx context.Context, claim *scorev1b1.ResourceClaim, provisioningStrategy strategy.Strategy) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	// Check if the resource is still healthy
	phase, reason, message, err := provisioningStrategy.GetStatus(ctx, claim)
	if err != nil {
		log.Error(err, "Failed to check resource health")
		return ctrl.Result{}, err
	}

	// If resource failed, mark as failed
	if phase == scorev1b1.ResourceClaimPhaseFailed {
		r.LifecycleManager.SetFailed(claim, reason, message)
		r.Recorder.Event(claim, "Warning", EventReasonProvisionFailed, "Resource became unhealthy")
		return ctrl.Result{}, fmt.Errorf("resource became unhealthy: %s", message)
	}

	log.V(1).Info("Resource is healthy")
	return ctrl.Result{}, nil
}

// handleFailedPhase handles the Failed phase
func (r *ProvisionerReconciler) handleFailedPhase(ctx context.Context, claim *scorev1b1.ResourceClaim, provisioningStrategy strategy.Strategy) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	// Check if the resource can be recovered
	phase, reason, message, err := provisioningStrategy.GetStatus(ctx, claim)
	if err != nil {
		log.V(1).Info("Resource still failed", "error", err)
		return ctrl.Result{}, nil
	}

	// If resource is now available, retry provisioning
	if phase == scorev1b1.ResourceClaimPhaseBound {
		log.Info("Resource recovered, retrying provisioning")
		r.LifecycleManager.SetClaiming(claim, ReasonBindingPending, "Retrying provisioning after recovery")
		return ctrl.Result{Requeue: true}, nil
	}

	log.V(1).Info("Resource still failed", "reason", reason, "message", message)
	return ctrl.Result{}, nil
}

// handleDeletion handles ResourceClaim deletion
func (r *ProvisionerReconciler) handleDeletion(ctx context.Context, claim *scorev1b1.ResourceClaim) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	if !r.LifecycleManager.HasFinalizer(claim) {
		log.V(1).Info("No finalizer, allowing deletion")
		return ctrl.Result{}, nil
	}

	log.Info("Deprovisioning resource", "type", claim.Spec.Type)
	r.Recorder.Event(claim, "Normal", EventReasonDeprovisioning, "Starting resource cleanup")

	// Get strategy and deprovision
	provisioningStrategy, err := r.StrategySelector.GetStrategy(claim.Spec.Type)
	if err != nil {
		log.Error(err, "Failed to get strategy for deprovisioning, removing finalizer anyway")
	} else {
		if err := provisioningStrategy.Deprovision(ctx, claim); err != nil {
			log.Error(err, "Failed to deprovision resource")
			r.Recorder.Event(claim, "Warning", EventReasonDeprovisionFailed, err.Error())
			return ctrl.Result{}, err
		}
	}

	// Remove finalizer to allow deletion
	r.LifecycleManager.RemoveFinalizer(claim)
	if err := r.Update(ctx, claim); err != nil {
		log.Error(err, "Failed to remove finalizer")
		return ctrl.Result{}, err
	}

	r.Recorder.Event(claim, "Normal", EventReasonDeprovisioned, "Resource cleanup completed")
	log.Info("Resource deprovisioned successfully")
	return ctrl.Result{}, nil
}

// loadSupportedTypes loads supported resource types from environment variable
func (r *ProvisionerReconciler) loadSupportedTypes() {
	envTypes := os.Getenv("SUPPORTED_RESOURCE_TYPES")
	if envTypes == "" {
		// Default supported types for Phase 1 (no actual provisioning strategies yet)
		envTypes = "test,mock"
	}

	types := strings.Split(envTypes, ",")
	for _, t := range types {
		t = strings.TrimSpace(t)
		if t != "" {
			r.supportedTypes[t] = true
		}
	}
}

// loadProvisioningConfig loads provisioning configuration from orchestrator config
func (r *ProvisionerReconciler) loadProvisioningConfig() {
	// For Phase 1, we'll use a minimal configuration
	// In later phases, this will load from OrchestratorConfig
	configs := []strategy.ProvisioningConfig{
		{
			Type:     "test",
			Strategy: "mock",
			Config:   map[string]any{},
		},
		{
			Type:     "mock",
			Strategy: "mock",
			Config:   map[string]any{},
		},
	}

	r.StrategySelector.LoadConfig(configs)
}

// filterSupportedTypes filters ResourceClaims to only reconcile supported types
func (r *ProvisionerReconciler) filterSupportedTypes(obj client.Object) bool {
	claim, ok := obj.(*scorev1b1.ResourceClaim)
	if !ok {
		return false
	}

	supported := r.supportedTypes[claim.Spec.Type]
	return supported
}
