package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
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

	// First, try to get WorkloadPlan
	plan := &scorev1b1.WorkloadPlan{}
	if err := r.Get(ctx, req.NamespacedName, plan); err == nil {
		// This is a WorkloadPlan reconciliation
		return r.reconcileWorkloadPlan(ctx, req, plan)
	} else if !errors.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	// Then, try to get WorkloadExposure
	we := &scorev1b1.WorkloadExposure{}
	if err := r.Get(ctx, req.NamespacedName, we); err == nil {
		// This is a WorkloadExposure reconciliation
		return r.ReconcileWorkloadExposure(ctx, req)
	} else if !errors.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	// Neither found, resource was deleted or doesn't exist
	logger.V(1).Info("Resource not found", "request", req.NamespacedName)
	return ctrl.Result{}, nil
}

// reconcileWorkloadPlan handles WorkloadPlan changes and materializes Kubernetes resources
// RBAC permissions are managed manually in deployments/kubernetes-runtime/rbac.yaml
// to avoid contaminating the main Orchestrator RBAC configuration
func (r *KubernetesRuntimeReconciler) reconcileWorkloadPlan(ctx context.Context, req ctrl.Request, plan *scorev1b1.WorkloadPlan) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

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

// ReconcileWorkloadExposure handles WorkloadExposure resources and publishes canonical URLs
func (r *KubernetesRuntimeReconciler) ReconcileWorkloadExposure(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// 1. Get WorkloadExposure
	we := &scorev1b1.WorkloadExposure{}
	if err := r.Get(ctx, req.NamespacedName, we); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Skip non-kubernetes runtime classes
	if we.Spec.RuntimeClass != "kubernetes" {
		logger.V(1).Info("Skipping WorkloadExposure with non-kubernetes runtime class",
			"runtimeClass", we.Spec.RuntimeClass)
		return ctrl.Result{}, nil
	}

	logger.Info("Reconciling WorkloadExposure for Kubernetes runtime",
		"workloadExposure", req.NamespacedName,
		"runtimeClass", we.Spec.RuntimeClass)

	// 2. Get referenced Workload
	workload := &scorev1b1.Workload{}
	workloadKey := types.NamespacedName{
		Name: we.Spec.WorkloadRef.Name,
	}

	// Set namespace - use explicit namespace if provided, otherwise use same as WorkloadExposure
	if we.Spec.WorkloadRef.Namespace != nil && *we.Spec.WorkloadRef.Namespace != "" {
		workloadKey.Namespace = *we.Spec.WorkloadRef.Namespace
	} else {
		workloadKey.Namespace = we.Namespace
	}

	if err := r.Get(ctx, workloadKey, workload); err != nil {
		logger.Error(err, "Failed to get referenced Workload")
		r.Recorder.Event(we, corev1.EventTypeWarning, "WorkloadNotFound", err.Error())
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	// Validate UID if specified (strong identity check)
	if we.Spec.WorkloadRef.UID != "" && string(workload.UID) != we.Spec.WorkloadRef.UID {
		logger.V(1).Info("Workload UID mismatch, skipping (may be recreated)",
			"expectedUID", we.Spec.WorkloadRef.UID,
			"actualUID", workload.UID)
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	// 3. Generation guard - skip if observedWorkloadGeneration is behind
	if we.Spec.ObservedWorkloadGeneration < workload.Generation {
		logger.V(1).Info("Skipping WorkloadExposure with outdated generation",
			"observedGeneration", we.Spec.ObservedWorkloadGeneration,
			"currentGeneration", workload.Generation)
		return ctrl.Result{RequeueAfter: time.Second * 30}, nil
	}

	// 4. Generate canonical URL from confirmed primary information (Ingress/LB only)
	exposureURL, err := r.generateCanonicalURL(ctx, workload, we.Namespace)
	if err != nil {
		logger.Error(err, "Failed to generate canonical URL")
		r.Recorder.Event(we, corev1.EventTypeWarning, "URLGenerationFailed", err.Error())
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	if exposureURL == "" || !r.isValidURL(exposureURL) {
		logger.V(1).Info("No valid URL available yet, exposure may be pending", "url", exposureURL)
		return ctrl.Result{RequeueAfter: time.Second * 30}, nil
	}

	// 5. Build desired exposures
	exposureType := "ingress"
	if strings.Contains(exposureURL, "svc.cluster.local") {
		exposureType = "service"
	} else if strings.Contains(exposureURL, ":") && !strings.Contains(exposureURL, "://") {
		exposureType = "loadbalancer"
	}

	desired := []scorev1b1.ExposureEntry{{
		URL:   exposureURL,
		Ready: true,
		Type:  exposureType,
	}}

	// 6. Check if update is needed (avoid flapping) using DeepEqual
	if reflect.DeepEqual(we.Status.Exposures, desired) {
		logger.V(1).Info("Exposures unchanged, no update needed", "url", exposureURL)
		return ctrl.Result{}, nil
	}

	// 7. Update status
	we.Status.Exposures = desired
	if err := r.Status().Update(ctx, we); err != nil {
		logger.Error(err, "Failed to update WorkloadExposure status")
		return ctrl.Result{RequeueAfter: time.Minute}, err
	}

	logger.Info("Successfully updated WorkloadExposure status", "url", exposureURL, "type", exposureType)
	r.Recorder.Eventf(we, corev1.EventTypeNormal, "ExposurePublished",
		"Published %s exposure URL: %s", exposureType, exposureURL)

	return ctrl.Result{}, nil
}

// generateCanonicalURL publishes canonical URL from materialized exposure (confirmed primary info only)
// Only publishes URLs from confirmed sources: Ingress with hostnames or LoadBalancer with external IPs/hostnames.
// Does NOT publish ClusterIP, NodePort, or other unconfirmed endpoints.
func (r *KubernetesRuntimeReconciler) generateCanonicalURL(ctx context.Context, workload *scorev1b1.Workload, namespace string) (string, error) {
	// Skip if no service is defined
	if workload.Spec.Service == nil || len(workload.Spec.Service.Ports) == 0 {
		return "", nil
	}

	workloadName := workload.Name

	// Priority 1: Check for Ingress with confirmed hostnames
	if ingressURL, err := r.getURLFromIngress(ctx, workloadName, namespace); err != nil {
		return "", fmt.Errorf("failed to check ingress: %w", err)
	} else if ingressURL != "" {
		return ingressURL, nil
	}

	// Priority 2: Check for LoadBalancer Service with external IPs/hostnames
	if lbURL, err := r.getURLFromLoadBalancer(ctx, workloadName, namespace); err != nil {
		return "", fmt.Errorf("failed to check load balancer: %w", err)
	} else if lbURL != "" {
		return lbURL, nil
	}

	// Priority 3: For now, do not publish ClusterIP/NodePort URLs
	// as they are not externally accessible or stable
	// TODO: Add NodePort support when external access is confirmed
	return "", nil
}

// getURLFromIngress checks for Ingress resources with confirmed hostnames and returns the first available URL
func (r *KubernetesRuntimeReconciler) getURLFromIngress(ctx context.Context, workloadName, namespace string) (string, error) {
	// List Ingresses that might be associated with this workload
	var ingressList networkingv1.IngressList
	if err := r.List(ctx, &ingressList, client.InNamespace(namespace)); err != nil {
		return "", err
	}

	for _, ingress := range ingressList.Items {
		// Check if this Ingress references our Service
		for _, rule := range ingress.Spec.Rules {
			if rule.Host == "" {
				continue // Skip rules without hostnames
			}

			// Check if this rule has paths pointing to our service
			if rule.HTTP != nil {
				for _, path := range rule.HTTP.Paths {
					if path.Backend.Service != nil && path.Backend.Service.Name == workloadName {
						// Found a matching Ingress rule with hostname
						scheme := "https"
						if len(ingress.Spec.TLS) == 0 {
							scheme = "http"
						}
						exposureURL := fmt.Sprintf("%s://%s", scheme, rule.Host)
						if path.Path != "" && path.Path != "/" {
							exposureURL += path.Path
						}
						return exposureURL, nil
					}
				}
			}
		}
	}

	return "", nil
}

// getURLFromLoadBalancer checks for LoadBalancer Service with external IPs/hostnames
func (r *KubernetesRuntimeReconciler) getURLFromLoadBalancer(ctx context.Context, workloadName, namespace string) (string, error) {
	service := &corev1.Service{}
	serviceKey := types.NamespacedName{
		Name:      workloadName,
		Namespace: namespace,
	}

	if err := r.Get(ctx, serviceKey, service); err != nil {
		if errors.IsNotFound(err) {
			return "", nil
		}
		return "", err
	}

	// Only handle LoadBalancer type services
	if service.Spec.Type != corev1.ServiceTypeLoadBalancer {
		return "", nil
	}

	// Check if LoadBalancer has external IP or hostname
	if len(service.Status.LoadBalancer.Ingress) == 0 {
		return "", nil // LoadBalancer not ready yet
	}

	// Use first available ingress point
	ingress := service.Status.LoadBalancer.Ingress[0]
	var host string
	if ingress.Hostname != "" {
		host = ingress.Hostname
	} else if ingress.IP != "" {
		host = ingress.IP
	} else {
		return "", nil // No usable address
	}

	// Get the first port for URL construction
	if len(service.Spec.Ports) == 0 {
		return "", nil
	}
	port := service.Spec.Ports[0].Port

	// Construct URL (assume HTTP for now, could be enhanced to detect HTTPS)
	exposureURL := fmt.Sprintf("http://%s:%d", host, port)
	return exposureURL, nil
}

// isValidURL validates if the given string is a valid HTTP/HTTPS URL
func (r *KubernetesRuntimeReconciler) isValidURL(urlStr string) bool {
	if urlStr == "" {
		return false
	}

	// Parse URL and validate structure
	u, err := url.Parse(urlStr)
	if err != nil {
		return false
	}

	// Validate scheme is HTTP or HTTPS
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}

	// Validate host is not empty
	if u.Host == "" {
		return false
	}

	return true
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

	// ResourceClaims are no longer needed for placeholder resolution
	// All values are now resolved by the Orchestrator and provided in WorkloadPlan.ResolvedValues

	// Build containers from workload spec
	containers := make([]corev1.Container, 0, len(workload.Spec.Containers))
	for containerName, containerSpec := range workload.Spec.Containers {
		container := corev1.Container{
			Name:  containerName,
			Image: containerSpec.Image,
		}

		// Get resolved environment variables from WorkloadPlan.ResolvedValues
		if plan.Spec.ResolvedValues != nil {
			resolvedEnv, err := r.extractResolvedEnv(plan.Spec.ResolvedValues, containerName)
			if err != nil {
				log.FromContext(ctx).Error(err, "Failed to extract resolved environment variables")
				return nil, fmt.Errorf("failed to extract resolved environment variables: %w", err)
			}

			for envName, envValue := range resolvedEnv {
				container.Env = append(container.Env, corev1.EnvVar{
					Name:  envName,
					Value: envValue,
				})
			}
		} else if containerSpec.Variables != nil {
			// Fallback: use raw values from containerSpec.Variables (no placeholder resolution)
			for key, value := range containerSpec.Variables {
				container.Env = append(container.Env, corev1.EnvVar{
					Name:  key,
					Value: value,
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

// extractResolvedEnv extracts resolved environment variables for a specific container from ResolvedValues
func (r *KubernetesRuntimeReconciler) extractResolvedEnv(resolvedValues *runtime.RawExtension, containerName string) (map[string]string, error) {
	if resolvedValues == nil || resolvedValues.Raw == nil {
		return make(map[string]string), nil
	}

	// Parse the resolved values JSON
	var values map[string]interface{}
	if err := json.Unmarshal(resolvedValues.Raw, &values); err != nil {
		return nil, fmt.Errorf("failed to unmarshal resolved values: %w", err)
	}

	// Navigate to containers.<containerName>.env
	containers, ok := values["containers"].(map[string]interface{})
	if !ok {
		return make(map[string]string), nil
	}

	container, ok := containers[containerName].(map[string]interface{})
	if !ok {
		return make(map[string]string), nil
	}

	env, ok := container["env"].(map[string]interface{})
	if !ok {
		return make(map[string]string), nil
	}

	// Convert to string map
	result := make(map[string]string)
	for key, value := range env {
		if strValue, ok := value.(string); ok {
			result[key] = strValue
		}
		// TODO: Handle valueFrom.secretKeyRef pattern for Secret references
	}

	return result, nil
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

// SetupWorkloadExposureWithManager sets up the WorkloadExposure controller with the Manager
func (r *KubernetesRuntimeReconciler) SetupWorkloadExposureWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&scorev1b1.WorkloadExposure{}).
		Watches(
			&scorev1b1.Workload{},
			handler.EnqueueRequestsFromMapFunc(r.findWorkloadExposuresForWorkload),
		).
		Watches(
			&corev1.Service{},
			handler.EnqueueRequestsFromMapFunc(r.findWorkloadExposuresForService),
		).
		Named("workloadexposure").
		Complete(r)
}

// findWorkloadExposuresForWorkload finds WorkloadExposures that reference the given Workload
func (r *KubernetesRuntimeReconciler) findWorkloadExposuresForWorkload(ctx context.Context, obj client.Object) []ctrl.Request {
	workload := obj.(*scorev1b1.Workload)

	// List all WorkloadExposures in the same namespace
	var exposures scorev1b1.WorkloadExposureList
	if err := r.List(ctx, &exposures, client.InNamespace(workload.Namespace)); err != nil {
		return nil
	}

	var requests []ctrl.Request
	for _, exposure := range exposures.Items {
		// Check if this exposure references the workload
		if exposure.Spec.WorkloadRef.Name == workload.Name {
			// Check namespace match (nil means same namespace as exposure)
			exposureNamespace := workload.Namespace
			if exposure.Spec.WorkloadRef.Namespace != nil && *exposure.Spec.WorkloadRef.Namespace != "" {
				exposureNamespace = *exposure.Spec.WorkloadRef.Namespace
			}

			if exposureNamespace == workload.Namespace {
				requests = append(requests, ctrl.Request{
					NamespacedName: types.NamespacedName{
						Name:      exposure.Name,
						Namespace: exposure.Namespace,
					},
				})
			}
		}
	}

	return requests
}

// findWorkloadExposuresForService finds WorkloadExposures that might be affected by Service changes
func (r *KubernetesRuntimeReconciler) findWorkloadExposuresForService(ctx context.Context, obj client.Object) []ctrl.Request {
	service := obj.(*corev1.Service)

	// Find the corresponding Workload (Service name should match Workload name)
	workload := &scorev1b1.Workload{}
	workloadKey := types.NamespacedName{
		Name:      service.Name,
		Namespace: service.Namespace,
	}

	if err := r.Get(ctx, workloadKey, workload); err != nil {
		// No corresponding workload found
		return nil
	}

	// Use the workload mapping function
	return r.findWorkloadExposuresForWorkload(ctx, workload)
}
