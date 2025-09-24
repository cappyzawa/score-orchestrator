package controller

import (
	"context"
	"net/url"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	scorev1b1 "github.com/cappyzawa/score-orchestrator/api/v1b1"
	"github.com/cappyzawa/score-orchestrator/internal/status"
)

// ExposureMirrorReconciler mirrors WorkloadExposure status to corresponding Workload status
type ExposureMirrorReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=score.dev,resources=workloadexposures,verbs=get;list;watch
// +kubebuilder:rbac:groups=score.dev,resources=workloads,verbs=get;list;watch
// +kubebuilder:rbac:groups=score.dev,resources=workloads/status,verbs=update;patch

// Reconcile mirrors WorkloadExposure status to the referenced Workload
func (r *ExposureMirrorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("workloadexposure", req.NamespacedName)
	logger.V(1).Info("Reconciling WorkloadExposure for endpoint mirroring")

	// Fetch the WorkloadExposure
	var exposure scorev1b1.WorkloadExposure
	if err := r.Get(ctx, req.NamespacedName, &exposure); err != nil {
		logger.V(1).Info("WorkloadExposure not found, possibly deleted")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Get the referenced Workload
	workloadRef := exposure.Spec.WorkloadRef
	workloadNamespace := exposure.Namespace // Default to same namespace
	if workloadRef.Namespace != nil {
		workloadNamespace = *workloadRef.Namespace
	}

	workloadKey := types.NamespacedName{
		Name:      workloadRef.Name,
		Namespace: workloadNamespace,
	}

	var workload scorev1b1.Workload
	if err := r.Get(ctx, workloadKey, &workload); err != nil {
		logger.V(1).Info("Referenced Workload not found", "workload", workloadKey)
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Strong identity check (if UID provided) to prevent rename/recreate confusion
	if workloadRef.UID != "" && string(workload.UID) != workloadRef.UID {
		logger.V(1).Info("Workload UID mismatch; ignoring this exposure",
			"expected", workloadRef.UID, "actual", string(workload.UID))
		return ctrl.Result{}, nil
	}

	// Check if the observed generation is current
	if exposure.Spec.ObservedWorkloadGeneration < workload.Generation {
		logger.V(1).Info("WorkloadExposure generation is outdated, skipping",
			"observedGeneration", exposure.Spec.ObservedWorkloadGeneration,
			"currentGeneration", workload.Generation)
		return ctrl.Result{}, nil
	}

	// Create a patch base for the Workload status
	patch := client.MergeFrom(workload.DeepCopy())

	// Mirror endpoint from the first exposure if available
	updated := r.mirrorEndpoint(&workload, &exposure)

	// Mirror normalized conditions
	conditionsUpdated := r.mirrorConditions(&workload, &exposure)
	updated = updated || conditionsUpdated

	// Only patch if something changed
	if updated {
		logger.V(1).Info("Updating Workload status from WorkloadExposure")
		if err := r.Status().Patch(ctx, &workload, patch); err != nil {
			logger.Error(err, "Failed to update Workload status")
			return ctrl.Result{}, err
		}
		logger.V(1).Info("Successfully mirrored WorkloadExposure status to Workload")

		// Record event for successful mirroring
		if workload.Status.Endpoint != nil {
			r.Recorder.Eventf(&workload, "Normal", "EndpointMirrored",
				"Mirrored endpoint from WorkloadExposure: %s", *workload.Status.Endpoint)
		} else {
			r.Recorder.Eventf(&workload, "Normal", "EndpointCleared",
				"Cleared endpoint due to WorkloadExposure changes")
		}
	}

	return ctrl.Result{}, nil
}

// mirrorEndpoint updates the Workload endpoint from exposures[0] (mirror-only)
func (r *ExposureMirrorReconciler) mirrorEndpoint(workload *scorev1b1.Workload, exposure *scorev1b1.WorkloadExposure) bool {
	var newEndpoint *string

	// Mirror-only: Runtime orders exposures by priority; use the top one as-is.
	if len(exposure.Status.Exposures) > 0 {
		top := exposure.Status.Exposures[0]
		if isValidURL(top.URL) {
			newEndpoint = &top.URL
		}
	}

	// Check if endpoint needs updating
	currentEndpoint := workload.Status.Endpoint
	if (currentEndpoint == nil && newEndpoint == nil) ||
		(currentEndpoint != nil && newEndpoint != nil && *currentEndpoint == *newEndpoint) {
		return false // No change needed
	}

	workload.Status.Endpoint = newEndpoint
	return true
}

// mirrorConditions updates Workload conditions from normalized WorkloadExposure conditions
func (r *ExposureMirrorReconciler) mirrorConditions(workload *scorev1b1.Workload, exposure *scorev1b1.WorkloadExposure) bool {
	// Normalize the exposure conditions
	normalizedConditions := status.NormalizeConditions(exposure.Status.Conditions)

	if len(normalizedConditions) == 0 {
		return false
	}

	// Track if any condition was actually updated
	updated := false

	// Update conditions with proper LastTransitionTime handling
	for _, condition := range normalizedConditions {
		// Check if this condition would actually change anything
		existing := findCondition(workload.Status.Conditions, condition.Type)
		if existing == nil || existing.Status != condition.Status ||
			existing.Reason != condition.Reason || existing.Message != condition.Message {
			setStatusCondition(&workload.Status.Conditions, condition)
			updated = true
		}
	}

	return updated
}

// findCondition finds a condition by type in the conditions slice
func findCondition(conditions []metav1.Condition, conditionType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}

// setStatusCondition sets or updates a condition in the conditions slice
func setStatusCondition(conditions *[]metav1.Condition, newCondition metav1.Condition) {
	if conditions == nil {
		*conditions = []metav1.Condition{}
	}

	// Find existing condition with the same type
	for i := range *conditions {
		if (*conditions)[i].Type == newCondition.Type {
			// Update existing condition
			existing := &(*conditions)[i]

			// Only update LastTransitionTime if status changed
			if existing.Status != newCondition.Status {
				existing.LastTransitionTime = metav1.Now()
			}

			existing.Status = newCondition.Status
			existing.Reason = newCondition.Reason
			existing.Message = newCondition.Message
			return
		}
	}

	// Add new condition if not found
	if newCondition.LastTransitionTime.IsZero() {
		newCondition.LastTransitionTime = metav1.Now()
	}
	*conditions = append(*conditions, newCondition)
}

// isValidURL checks if the given URL is valid and uses http/https scheme with host
func isValidURL(rawURL string) bool {
	if rawURL == "" {
		return false
	}

	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return false
	}

	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return false
	}
	// Host is required (path-only URLs are invalid)
	if parsedURL.Host == "" {
		return false
	}
	return true
}

// SetupWithManager sets up the controller with the Manager
func (r *ExposureMirrorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&scorev1b1.WorkloadExposure{}).
		Named("exposure-mirror").
		Complete(r)
}
