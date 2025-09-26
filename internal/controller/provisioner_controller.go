package controller

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	scorev1b1 "github.com/cappyzawa/score-orchestrator/api/v1b1"
	"github.com/cappyzawa/score-orchestrator/internal/conditions"
	"github.com/cappyzawa/score-orchestrator/internal/config"
	"github.com/cappyzawa/score-orchestrator/internal/provisioner"
	"github.com/cappyzawa/score-orchestrator/internal/provisioner/strategy"
	"github.com/cappyzawa/score-orchestrator/internal/provisioner/strategy/postgres"
	"github.com/cappyzawa/score-orchestrator/internal/provisioner/strategy/redis"
	"github.com/cappyzawa/score-orchestrator/internal/provisioner/strategy/secret"
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
	fmt.Printf("DEBUG: ProvisionerReconciler.SetupWithManager called\n")

	// Load supported types from environment or config
	fmt.Printf("DEBUG: Loading supported types\n")
	r.loadSupportedTypes()

	// Load provisioning configuration
	fmt.Printf("DEBUG: Loading provisioning configuration\n")
	r.loadProvisioningConfig()

	// Register concrete strategies
	fmt.Printf("DEBUG: Registering strategies\n")
	r.registerStrategies()

	fmt.Printf("DEBUG: Setting up controller with manager\n")
	controller := ctrl.NewControllerManagedBy(mgr).
		For(&scorev1b1.ResourceClaim{}).
		WithEventFilter(predicate.NewPredicateFuncs(r.filterSupportedTypes))

	fmt.Printf("DEBUG: Controller builder created, calling Complete\n")
	err := controller.Complete(r)
	if err != nil {
		fmt.Printf("DEBUG: Controller.Complete failed: %v\n", err)
		return err
	}

	fmt.Printf("DEBUG: Provisioner controller setup completed successfully\n")
	return nil
}

// +kubebuilder:rbac:groups=score.dev,resources=resourceclaims,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=score.dev,resources=resourceclaims/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile handles ResourceClaim reconciliation
func (r *ProvisionerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	fmt.Printf("DEBUG: Provisioner.Reconcile called for %s/%s\n", req.Namespace, req.Name)

	// Fetch the ResourceClaim
	claim := &scorev1b1.ResourceClaim{}
	if err := r.Get(ctx, req.NamespacedName, claim); err != nil {
		fmt.Printf("DEBUG: Failed to fetch ResourceClaim %s/%s: %v\n", req.Namespace, req.Name, err)
		log.Error(err, "Failed to fetch ResourceClaim")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	fmt.Printf("DEBUG: Found ResourceClaim %s/%s with type=%s, phase=%s\n",
		claim.Namespace, claim.Name, claim.Spec.Type, claim.Status.Phase)

	// Check if we should reconcile this claim
	if !r.LifecycleManager.ShouldReconcile(claim) {
		fmt.Printf("DEBUG: Skipping reconciliation for %s/%s, phase=%s\n", claim.Namespace, claim.Name, claim.Status.Phase)
		log.V(1).Info("Skipping reconciliation", "phase", claim.Status.Phase)
		return ctrl.Result{}, nil
	}

	fmt.Printf("DEBUG: Proceeding with reconciliation for %s/%s\n", claim.Namespace, claim.Name)
	log.Info("Reconciling ResourceClaim", "type", claim.Spec.Type, "phase", claim.Status.Phase)

	// Handle deletion
	if r.LifecycleManager.IsBeingDeleted(claim) {
		fmt.Printf("DEBUG: ResourceClaim %s/%s is being deleted\n", claim.Namespace, claim.Name)
		return r.handleDeletion(ctx, claim)
	}

	// Ensure finalizer is present
	if !r.LifecycleManager.HasFinalizer(claim) {
		fmt.Printf("DEBUG: Adding finalizer to ResourceClaim %s/%s\n", claim.Namespace, claim.Name)
		r.LifecycleManager.AddFinalizer(claim)
		if err := r.Update(ctx, claim); err != nil {
			fmt.Printf("DEBUG: Failed to add finalizer to %s/%s: %v\n", claim.Namespace, claim.Name, err)
			log.Error(err, "Failed to add finalizer")
			return ctrl.Result{}, err
		}
		log.V(1).Info("Added finalizer to ResourceClaim")
		// Set initial phase and requeue using GetReconcileResult for consistency
		r.LifecycleManager.SetPending(claim, conditions.ReasonClaimPending, "Initializing resource claim")
		// Update status after setting initial phase
		if statusErr := r.Status().Update(ctx, claim); statusErr != nil {
			fmt.Printf("DEBUG: Failed to update ResourceClaim status after adding finalizer %s/%s: %v\n", claim.Namespace, claim.Name, statusErr)
			log.Error(statusErr, "Failed to update ResourceClaim status after adding finalizer")
			return ctrl.Result{}, statusErr
		}
		fmt.Printf("DEBUG: Successfully added finalizer and set initial phase for %s/%s\n", claim.Namespace, claim.Name)
		return r.LifecycleManager.GetReconcileResult(ctx, claim, nil)
	}

	fmt.Printf("DEBUG: Handling provisioning for ResourceClaim %s/%s\n", claim.Namespace, claim.Name)
	// Handle provisioning
	_, err := r.handleProvisioning(ctx, claim)

	// Update status
	if statusErr := r.Status().Update(ctx, claim); statusErr != nil {
		fmt.Printf("DEBUG: Failed to update ResourceClaim status %s/%s: %v\n", claim.Namespace, claim.Name, statusErr)
		log.Error(statusErr, "Failed to update ResourceClaim status")
		if err == nil {
			err = statusErr
		}
	}

	fmt.Printf("DEBUG: Provisioner.Reconcile completed for %s/%s, err=%v\n", claim.Namespace, claim.Name, err)
	return r.LifecycleManager.GetReconcileResult(ctx, claim, err)
}

// handleProvisioning handles the provisioning logic
func (r *ProvisionerReconciler) handleProvisioning(ctx context.Context, claim *scorev1b1.ResourceClaim) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	// Get strategy for this resource type
	provisioningStrategy, err := r.StrategySelector.GetStrategy(claim.Spec.Type)
	if err != nil {
		r.LifecycleManager.SetFailed(claim, conditions.ReasonClaimFailed, fmt.Sprintf("No strategy available: %v", err))
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
		r.LifecycleManager.SetPending(claim, conditions.ReasonClaimPending, "Unknown phase, resetting to Pending")
		return ctrl.Result{Requeue: true}, nil
	}
}

// handlePendingPhase handles the Pending phase
func (r *ProvisionerReconciler) handlePendingPhase(ctx context.Context, claim *scorev1b1.ResourceClaim, provisioningStrategy strategy.Strategy) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	log.Info("Starting provisioning", "type", claim.Spec.Type)

	// Call the Provision method to create the resource
	outputs, err := provisioningStrategy.Provision(ctx, claim)
	if err != nil {
		log.Error(err, "Failed to provision resource")
		r.LifecycleManager.SetFailed(claim, conditions.ReasonClaimFailed, fmt.Sprintf("Provisioning failed: %v", err))
		r.Recorder.Event(claim, "Warning", EventReasonProvisionFailed, err.Error())
		return ctrl.Result{}, err
	}

	// Validate outputs
	if err := r.OutputManager.ValidateOutputs(outputs); err != nil {
		log.Error(err, "Invalid outputs from strategy")
		r.LifecycleManager.SetFailed(claim, conditions.ReasonProjectionError, fmt.Sprintf("Invalid outputs: %v", err))
		return ctrl.Result{}, err
	}

	// Set to Bound phase with outputs
	r.LifecycleManager.SetBound(claim, outputs)
	r.Recorder.Event(claim, "Normal", EventReasonProvisioned, "Resource successfully provisioned")
	log.Info("Resource provisioned successfully")

	return ctrl.Result{}, nil
}

// handleClaimingPhase handles the Claiming phase
func (r *ProvisionerReconciler) handleClaimingPhase(ctx context.Context, claim *scorev1b1.ResourceClaim, provisioningStrategy strategy.Strategy) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	// Check current status from strategy
	phase, reason, message, err := provisioningStrategy.GetStatus(ctx, claim)
	if err != nil {
		log.Error(err, "Failed to get status from strategy")
		r.LifecycleManager.SetFailed(claim, conditions.ReasonClaimFailed, fmt.Sprintf("Status check failed: %v", err))
		return ctrl.Result{}, err
	}

	// If already bound, get outputs and update status
	if phase == scorev1b1.ResourceClaimPhaseBound {
		// Since we're already bound, we need to get the outputs without re-provisioning
		// This happens when the resource was created but the status wasn't updated yet
		outputs, err := provisioningStrategy.Provision(ctx, claim)
		if err != nil {
			log.Error(err, "Failed to get outputs from strategy")
			r.LifecycleManager.SetFailed(claim, conditions.ReasonClaimFailed, fmt.Sprintf("Failed to get outputs: %v", err))
			r.Recorder.Event(claim, "Warning", EventReasonProvisionFailed, err.Error())
			return ctrl.Result{}, err
		}

		// Validate outputs
		if err := r.OutputManager.ValidateOutputs(outputs); err != nil {
			log.Error(err, "Invalid outputs from strategy")
			r.LifecycleManager.SetFailed(claim, conditions.ReasonProjectionError, fmt.Sprintf("Invalid outputs: %v", err))
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

	// Continue claiming - still in progress
	log.V(1).Info("Provisioning in progress", "reason", reason, "message", message)
	return ctrl.Result{RequeueAfter: time.Second * 10}, nil
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
		r.LifecycleManager.SetClaiming(claim, conditions.ReasonClaiming, "Retrying provisioning after recovery")
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
		// Default supported types including new concrete strategies
		envTypes = "postgres,redis,secret,test,mock"
	}

	fmt.Printf("DEBUG: SUPPORTED_RESOURCE_TYPES env var: '%s'\n", envTypes)
	types := strings.Split(envTypes, ",")
	for _, t := range types {
		t = strings.TrimSpace(t)
		if t != "" {
			r.supportedTypes[t] = true
			fmt.Printf("DEBUG: Added supported type: '%s'\n", t)
		}
	}
	fmt.Printf("DEBUG: Final supportedTypes map: %+v\n", r.supportedTypes)
}

// loadProvisioningConfig loads provisioning configuration from orchestrator config
func (r *ProvisionerReconciler) loadProvisioningConfig() {
	// For Phase 1, we'll use a minimal configuration
	// In later phases, this will load from OrchestratorConfig
	configs := []strategy.ProvisioningConfig{
		{
			Type:     "postgres",
			Strategy: "postgres",
			Config:   map[string]any{},
		},
		{
			Type:     "redis",
			Strategy: "redis",
			Config:   map[string]any{},
		},
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

// registerStrategies registers concrete strategy implementations
func (r *ProvisionerReconciler) registerStrategies() {
	// Register Postgres strategy
	postgresStrategy := postgres.NewPostgresStrategy(r.Client)
	r.StrategySelector.RegisterStrategy(postgresStrategy)

	// Register Redis strategy
	redisStrategy := redis.NewRedisStrategy(r.Client)
	r.StrategySelector.RegisterStrategy(redisStrategy)

	// Register Secret strategy
	secretStrategy := secret.NewSecretStrategy(r.Client)
	r.StrategySelector.RegisterStrategy(secretStrategy)
}

// filterSupportedTypes filters ResourceClaims to only reconcile supported types
func (r *ProvisionerReconciler) filterSupportedTypes(obj client.Object) bool {
	claim, ok := obj.(*scorev1b1.ResourceClaim)
	if !ok {
		fmt.Printf("DEBUG: filterSupportedTypes - not a ResourceClaim: %T\n", obj)
		return false
	}

	supported := r.supportedTypes[claim.Spec.Type]
	fmt.Printf("DEBUG: filterSupportedTypes - ResourceClaim %s/%s type=%s, supported=%v, supportedTypes=%+v\n",
		claim.Namespace, claim.Name, claim.Spec.Type, supported, r.supportedTypes)
	return supported
}
