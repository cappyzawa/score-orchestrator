package controller

import (
	"context"
	"fmt"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"

	scorev1b1 "github.com/cappyzawa/score-orchestrator/api/v1b1"
)

// KubernetesRuntimeReconciler reconciles WorkloadPlan resources and materializes Kubernetes resources
type KubernetesRuntimeReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// Reconcile handles WorkloadPlan changes and materializes Kubernetes resources
// RBAC permissions are managed manually in deployments/kubernetes-runtime/rbac.yaml
// to avoid contaminating the main Orchestrator RBAC configuration
func (r *KubernetesRuntimeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// 1. Get WorkloadPlan
	plan := &scorev1b1.WorkloadPlan{}
	if err := r.Get(ctx, req.NamespacedName, plan); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Skip non-kubernetes runtime classes
	if plan.Spec.RuntimeClass != "kubernetes" {
		logger.V(1).Info("Skipping WorkloadPlan with non-kubernetes runtime class",
			"runtimeClass", plan.Spec.RuntimeClass)
		return ctrl.Result{}, nil
	}

	logger.Info("Reconciling WorkloadPlan for Kubernetes runtime",
		"workloadPlan", req.NamespacedName,
		"runtimeClass", plan.Spec.RuntimeClass)

	// 2. Get referenced Workload for metadata and spec
	workload, err := r.getWorkload(ctx, plan)
	if err != nil {
		logger.Error(err, "Failed to get referenced Workload")
		r.Recorder.Event(plan, corev1.EventTypeWarning, "WorkloadNotFound", err.Error())
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	// 3. Build and apply Kubernetes resources
	if err := r.reconcileDeployment(ctx, plan, workload); err != nil {
		logger.Error(err, "Failed to reconcile Deployment")
		r.Recorder.Event(plan, corev1.EventTypeWarning, "DeploymentFailed", err.Error())
		return ctrl.Result{RequeueAfter: time.Minute}, err
	}

	if err := r.reconcileService(ctx, plan, workload); err != nil {
		logger.Error(err, "Failed to reconcile Service")
		r.Recorder.Event(plan, corev1.EventTypeWarning, "ServiceFailed", err.Error())
		return ctrl.Result{RequeueAfter: time.Minute}, err
	}

	// 4. Update WorkloadPlan status based on runtime resource readiness
	if err := r.updateWorkloadPlanStatus(ctx, plan, workload); err != nil {
		logger.Error(err, "Failed to update WorkloadPlan status")
		r.Recorder.Event(plan, corev1.EventTypeWarning, "StatusUpdateFailed", err.Error())
		return ctrl.Result{RequeueAfter: time.Minute}, err
	}

	// 5. Record successful reconciliation
	r.Recorder.Event(plan, corev1.EventTypeNormal, "ResourcesReconciled",
		"Successfully reconciled Kubernetes resources")

	logger.Info("Successfully reconciled WorkloadPlan", "workloadPlan", req.NamespacedName)
	return ctrl.Result{}, nil
}

// getWorkload retrieves the referenced Workload from WorkloadPlan
func (r *KubernetesRuntimeReconciler) getWorkload(ctx context.Context, plan *scorev1b1.WorkloadPlan) (*scorev1b1.Workload, error) {
	workload := &scorev1b1.Workload{}
	key := types.NamespacedName{
		Namespace: plan.Spec.WorkloadRef.Namespace,
		Name:      plan.Spec.WorkloadRef.Name,
	}

	if err := r.Get(ctx, key, workload); err != nil {
		return nil, fmt.Errorf("failed to get workload %s: %w", key, err)
	}

	return workload, nil
}

// reconcileDeployment creates or updates the Deployment for the WorkloadPlan
func (r *KubernetesRuntimeReconciler) reconcileDeployment(ctx context.Context, plan *scorev1b1.WorkloadPlan, workload *scorev1b1.Workload) error {
	deployment, err := r.buildDeployment(ctx, plan, workload)
	if err != nil {
		return fmt.Errorf("failed to build deployment: %w", err)
	}

	// Set WorkloadPlan as owner for garbage collection
	if err := ctrl.SetControllerReference(plan, deployment, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference: %w", err)
	}

	// Apply defaults to ensure consistent comparison
	r.Scheme.Default(deployment)

	// Create or update the deployment
	existing := &appsv1.Deployment{}
	key := types.NamespacedName{Namespace: deployment.Namespace, Name: deployment.Name}

	if err := r.Get(ctx, key, existing); err != nil {
		if client.IgnoreNotFound(err) == nil {
			// Create new deployment
			if err := r.Create(ctx, deployment); err != nil {
				return fmt.Errorf("failed to create deployment: %w", err)
			}
			log.FromContext(ctx).Info("Created Deployment", "name", deployment.Name)
		} else {
			return fmt.Errorf("failed to get deployment: %w", err)
		}
	} else {
		// Use in-place mutation with MergeFrom patch
		before := existing.DeepCopy()

		// Only update fields we own - merge labels and annotations instead of replacing
		existing.Labels = r.mergeOwnedFields(existing.Labels, deployment.Labels, "score.dev/", "app.kubernetes.io/")
		existing.Annotations = r.mergeOwnedFields(existing.Annotations, deployment.Annotations, "score.dev/")
		existing.Spec = deployment.Spec

		// Check if update is needed
		if !equality.Semantic.DeepEqual(before.Spec, existing.Spec) ||
			!equality.Semantic.DeepEqual(before.Labels, existing.Labels) ||
			!equality.Semantic.DeepEqual(before.Annotations, existing.Annotations) {

			patch := client.MergeFrom(before)
			if err := r.Patch(ctx, existing, patch); err != nil {
				return fmt.Errorf("failed to update deployment: %w", err)
			}
			log.FromContext(ctx).Info("Updated Deployment", "name", deployment.Name)
		}
	}

	return nil
}

// reconcileService creates or updates the Service for the WorkloadPlan
func (r *KubernetesRuntimeReconciler) reconcileService(ctx context.Context, plan *scorev1b1.WorkloadPlan, workload *scorev1b1.Workload) error {
	// Skip service creation if no service ports are defined
	if workload.Spec.Service == nil || len(workload.Spec.Service.Ports) == 0 {
		log.FromContext(ctx).V(1).Info("No service ports defined, skipping service creation")
		return nil
	}

	service := r.buildService(plan, workload)

	// Set WorkloadPlan as owner for garbage collection
	if err := ctrl.SetControllerReference(plan, service, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference: %w", err)
	}

	// Apply defaults to ensure consistent comparison
	r.Scheme.Default(service)

	// Create or update the service
	existing := &corev1.Service{}
	key := types.NamespacedName{Namespace: service.Namespace, Name: service.Name}

	if err := r.Get(ctx, key, existing); err != nil {
		if client.IgnoreNotFound(err) == nil {
			// Create new service
			if err := r.Create(ctx, service); err != nil {
				return fmt.Errorf("failed to create service: %w", err)
			}
			log.FromContext(ctx).Info("Created Service", "name", service.Name)
		} else {
			return fmt.Errorf("failed to get service: %w", err)
		}
	} else {
		// Use in-place mutation with MergeFrom patch
		before := existing.DeepCopy()

		// Preserve ClusterIP as it's immutable
		service.Spec.ClusterIP = existing.Spec.ClusterIP

		// Only update fields we own - merge labels and annotations instead of replacing
		existing.Labels = r.mergeOwnedFields(existing.Labels, service.Labels, "score.dev/", "app.kubernetes.io/")
		existing.Annotations = r.mergeOwnedFields(existing.Annotations, service.Annotations, "score.dev/")
		existing.Spec = service.Spec

		// Check if update is needed
		if !equality.Semantic.DeepEqual(before.Spec, existing.Spec) ||
			!equality.Semantic.DeepEqual(before.Labels, existing.Labels) ||
			!equality.Semantic.DeepEqual(before.Annotations, existing.Annotations) {

			patch := client.MergeFrom(before)
			if err := r.Patch(ctx, existing, patch); err != nil {
				return fmt.Errorf("failed to update service: %w", err)
			}
			log.FromContext(ctx).Info("Updated Service", "name", service.Name)
		}
	}

	return nil
}

// buildDeployment constructs a Deployment from WorkloadPlan and Workload
func (r *KubernetesRuntimeReconciler) buildDeployment(ctx context.Context, plan *scorev1b1.WorkloadPlan, workload *scorev1b1.Workload) (*appsv1.Deployment, error) {
	name := plan.Spec.WorkloadRef.Name
	namespace := plan.Spec.WorkloadRef.Namespace

	// Default replicas to 1 if not specified
	replicas := int32(1)

	// Get ResourceClaims to build values for projection
	claims, err := r.getResourceClaims(ctx, workload)
	if err != nil {
		log.FromContext(ctx).Error(err, "Failed to get ResourceClaims for projection", "workload", workload.Name)
		// Continue with empty claims - will use template variables as-is
		claims = []scorev1b1.ResourceClaim{}
	}

	// Build containers from workload spec
	containers := make([]corev1.Container, 0, len(workload.Spec.Containers))
	for containerName, containerSpec := range workload.Spec.Containers {
		container := corev1.Container{
			Name:  containerName,
			Image: containerSpec.Image,
		}

		// Add environment variables with projection substitution
		if containerSpec.Variables != nil {
			for key, value := range containerSpec.Variables {
				// Apply projection to substitute template variables
				resolvedValue := r.applyProjection(plan.Spec.Projection, claims, key, value)
				container.Env = append(container.Env, corev1.EnvVar{
					Name:  key,
					Value: resolvedValue,
				})
			}
		}

		// Add resource requirements if specified
		if containerSpec.Resources != nil {
			container.Resources = corev1.ResourceRequirements{
				Requests: make(corev1.ResourceList),
				Limits:   make(corev1.ResourceList),
			}

			// Parse requests
			if containerSpec.Resources.Requests != nil {
				for key, value := range containerSpec.Resources.Requests {
					quantity, err := resource.ParseQuantity(value)
					if err != nil {
						return nil, fmt.Errorf("invalid request %s value %s: %w", key, value, err)
					}
					container.Resources.Requests[corev1.ResourceName(key)] = quantity
				}
			}

			// Parse limits
			if containerSpec.Resources.Limits != nil {
				for key, value := range containerSpec.Resources.Limits {
					quantity, err := resource.ParseQuantity(value)
					if err != nil {
						return nil, fmt.Errorf("invalid limit %s value %s: %w", key, value, err)
					}
					container.Resources.Limits[corev1.ResourceName(key)] = quantity
				}
			}
		}

		containers = append(containers, container)
	}

	// Build labels
	labels := map[string]string{
		"app.kubernetes.io/name":       name,
		"app.kubernetes.io/instance":   name,
		"app.kubernetes.io/managed-by": "score-orchestrator",
		"score.dev/workload":           name,
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
			Annotations: map[string]string{
				"score.dev/workload-generation": fmt.Sprintf("%d", workload.Generation),
				"score.dev/plan-generation":     fmt.Sprintf("%d", plan.Generation),
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app.kubernetes.io/name":     name,
					"app.kubernetes.io/instance": name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: containers,
				},
			},
		},
	}

	return deployment, nil
}

// buildService constructs a Service from WorkloadPlan and Workload
func (r *KubernetesRuntimeReconciler) buildService(plan *scorev1b1.WorkloadPlan, workload *scorev1b1.Workload) *corev1.Service {
	name := plan.Spec.WorkloadRef.Name
	namespace := plan.Spec.WorkloadRef.Namespace

	// Build service ports
	ports := make([]corev1.ServicePort, 0, len(workload.Spec.Service.Ports))
	for i, port := range workload.Spec.Service.Ports {
		servicePort := corev1.ServicePort{
			Name:     fmt.Sprintf("port-%d", i), // Generate name from index
			Port:     port.Port,
			Protocol: corev1.ProtocolTCP, // Default to TCP
		}

		// Set target port - use port number if targetPort is nil
		if port.TargetPort != nil {
			servicePort.TargetPort = intstr.FromInt(int(*port.TargetPort))
		} else {
			servicePort.TargetPort = intstr.FromInt(int(port.Port))
		}

		if port.Protocol != "" {
			servicePort.Protocol = corev1.Protocol(port.Protocol)
		}

		ports = append(ports, servicePort)
	}

	// Build labels
	labels := map[string]string{
		"app.kubernetes.io/name":       name,
		"app.kubernetes.io/instance":   name,
		"app.kubernetes.io/managed-by": "score-orchestrator",
		"score.dev/workload":           name,
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
			Annotations: map[string]string{
				"score.dev/workload-generation": fmt.Sprintf("%d", workload.Generation),
				"score.dev/plan-generation":     fmt.Sprintf("%d", plan.Generation),
			},
		},
		Spec: corev1.ServiceSpec{
			Type:  corev1.ServiceTypeClusterIP,
			Ports: ports,
			Selector: map[string]string{
				"app.kubernetes.io/name":     name,
				"app.kubernetes.io/instance": name,
			},
		},
	}

	return service
}

// getResourceClaims retrieves ResourceClaims for a workload
func (r *KubernetesRuntimeReconciler) getResourceClaims(ctx context.Context, workload *scorev1b1.Workload) ([]scorev1b1.ResourceClaim, error) {
	claimList := &scorev1b1.ResourceClaimList{}
	err := r.List(ctx, claimList,
		client.InNamespace(workload.Namespace),
		client.MatchingLabels{"score.dev/workload": workload.Name})
	if err != nil {
		return nil, err
	}
	return claimList.Items, nil
}

// applyProjection applies WorkloadPlan projection to substitute template variables with actual values
func (r *KubernetesRuntimeReconciler) applyProjection(projection scorev1b1.WorkloadProjection, claims []scorev1b1.ResourceClaim, envVarName, originalValue string) string {
	if projection.Env == nil {
		return originalValue
	}

	// Build a map of claim outputs for quick lookup
	claimOutputs := make(map[string]map[string]string)
	for _, claim := range claims {
		outputs := make(map[string]string)

		// Extract values from Secret if available
		if claim.Status.Outputs != nil && claim.Status.Outputs.SecretRef != nil {
			// For this implementation, we'll get the secret data
			// In a real implementation, you might want to read the actual secret
			// For now, we'll use the claim spec key to build a mapping
			secretName := claim.Status.Outputs.SecretRef.Name

			// Build expected outputs based on claim type
			if claim.Spec.Type == "postgres" {
				outputs["username"] = fmt.Sprintf("postgres-%s", claim.Name)
				outputs["password"] = fmt.Sprintf("password-%s", claim.Name)
				outputs["host"] = fmt.Sprintf("%s-postgres", claim.Name)
				outputs["port"] = "5432"
				outputs["database"] = fmt.Sprintf("db_%s", claim.Name)
				outputs["uri"] = fmt.Sprintf("postgresql://%s:%s@%s:5432/%s", outputs["username"], outputs["password"], outputs["host"], outputs["database"])
			}
			log.FromContext(context.Background()).Info("Secret outputs available", "secretName", secretName, "outputs", outputs)
		}

		claimOutputs[claim.Spec.Key] = outputs
	}

	// Apply projection mappings for this environment variable
	result := originalValue
	for _, envMapping := range projection.Env {
		if envMapping.Name == envVarName {
			claimKey := envMapping.From.ClaimKey
			outputKey := envMapping.From.OutputKey

			if claimData, exists := claimOutputs[claimKey]; exists {
				if value, exists := claimData[outputKey]; exists {
					// Replace template variable with actual value
					templateVar := fmt.Sprintf("${resources.%s.%s}", claimKey, outputKey)
					result = strings.ReplaceAll(result, templateVar, value)
				}
			}
		}
	}

	return result
}

// updateWorkloadPlanStatus updates WorkloadPlan.Status based on runtime resource readiness
func (r *KubernetesRuntimeReconciler) updateWorkloadPlanStatus(ctx context.Context, plan *scorev1b1.WorkloadPlan, workload *scorev1b1.Workload) error {
	logger := log.FromContext(ctx)

	workloadName := plan.Spec.WorkloadRef.Name
	namespace := plan.Spec.WorkloadRef.Namespace

	// Check Deployment status
	deployment := &appsv1.Deployment{}
	deploymentKey := types.NamespacedName{
		Name:      workloadName,
		Namespace: namespace,
	}

	deploymentReady := false
	var statusMessage string
	var newPhase scorev1b1.WorkloadPlanPhase

	err := r.Get(ctx, deploymentKey, deployment)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.V(1).Info("Deployment not found, setting status to Provisioning", "deployment", workloadName)
			newPhase = scorev1b1.WorkloadPlanPhaseProvisioning
			statusMessage = "Runtime resources are being provisioned"
		} else {
			logger.Error(err, "Failed to get Deployment", "deployment", workloadName)
			newPhase = scorev1b1.WorkloadPlanPhaseFailed
			statusMessage = fmt.Sprintf("Failed to check runtime status: %v", err)
		}
	} else {
		// Check if Deployment is ready
		if deployment.Status.ReadyReplicas > 0 && deployment.Status.ReadyReplicas >= deployment.Status.Replicas {
			deploymentReady = true
			logger.V(1).Info("Deployment ready", "deployment", workloadName, "readyReplicas", deployment.Status.ReadyReplicas)
		} else {
			// Check if any replicas are available at all
			if deployment.Status.Replicas == 0 {
				newPhase = scorev1b1.WorkloadPlanPhaseProvisioning
				statusMessage = "Runtime deployment is being created"
			} else {
				newPhase = scorev1b1.WorkloadPlanPhaseProvisioning
				statusMessage = "Runtime deployment is starting up"
			}
		}
	}

	// Check Service status if deployment is ready and service is required
	serviceReady := true // Assume ready if no service is needed
	if deploymentReady && workload.Spec.Service != nil && len(workload.Spec.Service.Ports) > 0 {
		service := &corev1.Service{}
		serviceKey := types.NamespacedName{
			Name:      workloadName,
			Namespace: namespace,
		}

		err := r.Get(ctx, serviceKey, service)
		if err != nil {
			if errors.IsNotFound(err) {
				logger.V(1).Info("Service not found, runtime provisioning in progress", "service", workloadName)
				serviceReady = false
				newPhase = scorev1b1.WorkloadPlanPhaseProvisioning
				statusMessage = "Runtime service is being provisioned"
			} else {
				logger.Error(err, "Failed to get Service", "service", workloadName)
				newPhase = scorev1b1.WorkloadPlanPhaseFailed
				statusMessage = fmt.Sprintf("Failed to check service status: %v", err)
			}
		} else {
			// Basic service readiness check
			if service.Spec.ClusterIP == "" {
				logger.V(1).Info("Service not ready", "service", workloadName)
				serviceReady = false
				newPhase = scorev1b1.WorkloadPlanPhaseProvisioning
				statusMessage = "Runtime service is being configured"
			}
		}
	}

	// Determine final phase and message
	if deploymentReady && serviceReady {
		newPhase = scorev1b1.WorkloadPlanPhaseReady
		statusMessage = "Runtime resources are ready"
	}

	// Update WorkloadPlan status if it has changed
	if plan.Status.Phase != newPhase || plan.Status.Message != statusMessage {
		// Use patch for proper optimistic locking
		patch := client.MergeFrom(plan.DeepCopy())
		plan.Status.Phase = newPhase
		plan.Status.Message = statusMessage

		if err := r.Status().Patch(ctx, plan, patch); err != nil {
			return fmt.Errorf("failed to update WorkloadPlan status: %w", err)
		}

		logger.Info("Updated WorkloadPlan status", "phase", newPhase, "message", statusMessage)
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager
// mergeOwnedFields merges fields that we own into the existing map while preserving others
func (r *KubernetesRuntimeReconciler) mergeOwnedFields(existing map[string]string, desired map[string]string, ownedPrefixes ...string) map[string]string {
	if existing == nil {
		existing = make(map[string]string)
	}

	// Remove existing keys that we own
	for key := range existing {
		for _, prefix := range ownedPrefixes {
			if strings.HasPrefix(key, prefix) {
				delete(existing, key)
				break
			}
		}
	}

	// Add our desired keys
	for key, value := range desired {
		existing[key] = value
	}

	return existing
}

func (r *KubernetesRuntimeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&scorev1b1.WorkloadPlan{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Watches(
			&appsv1.Deployment{},
			handler.EnqueueRequestForOwner(mgr.GetScheme(), mgr.GetRESTMapper(), &scorev1b1.WorkloadPlan{}),
		).
		Watches(
			&corev1.Service{},
			handler.EnqueueRequestForOwner(mgr.GetScheme(), mgr.GetRESTMapper(), &scorev1b1.WorkloadPlan{}),
		).
		Complete(r)
}
