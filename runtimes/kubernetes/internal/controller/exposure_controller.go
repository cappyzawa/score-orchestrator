package controller

import (
	"context"
	"fmt"
	"net/url"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	scorev1b1 "github.com/cappyzawa/score-orchestrator/api/v1b1"
)

// KubernetesRuntimeExposureReconciler reconciles a WorkloadExposure object for Kubernetes runtime
type KubernetesRuntimeExposureReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=score.dev,resources=workloadexposures,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=score.dev,resources=workloadexposures/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=score.dev,resources=workloadexposures/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *KubernetesRuntimeExposureReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("controller", "workloadexposure", "namespacedName", req.NamespacedName)

	// Fetch the WorkloadExposure instance
	workloadExposure := &scorev1b1.WorkloadExposure{}
	if err := r.Get(ctx, req.NamespacedName, workloadExposure); err != nil {
		if apierrors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Unable to fetch WorkloadExposure")
		return ctrl.Result{}, err
	}

	// Only handle Kubernetes runtime
	if workloadExposure.Spec.RuntimeClass != kubernetesRuntimeClass {
		logger.Info("Skipping non-Kubernetes runtime", "runtimeClass", workloadExposure.Spec.RuntimeClass)
		return ctrl.Result{}, nil
	}

	// Get all Services in the namespace with the workload label
	services := &corev1.ServiceList{}
	workloadName := workloadExposure.Name
	listOpts := []client.ListOption{
		client.InNamespace(req.Namespace),
		client.MatchingLabels{
			"score.dev/workload": workloadName,
		},
	}
	if err := r.List(ctx, services, listOpts...); err != nil {
		logger.Error(err, "Failed to list Services")
		return ctrl.Result{}, err
	}

	// Find the primary service and generate URL
	var exposureURL string
	for _, service := range services.Items {
		if serviceURL, err := r.getURLFromService(&service); err != nil {
			logger.Error(err, "Failed to get URL from service", "serviceName", service.Name)
		} else if serviceURL != "" {
			exposureURL = serviceURL
			break // Use the first valid URL found
		}
	}

	// Update WorkloadExposure status with the URL
	exposureStatus := scorev1b1.WorkloadExposureStatus{
		Exposures: []scorev1b1.ExposureEntry{
			{
				URL:   exposureURL,
				Ready: exposureURL != "",
			},
		},
	}

	if !reflect.DeepEqual(workloadExposure.Status, exposureStatus) {
		workloadExposure.Status = exposureStatus
		if err := r.Status().Update(ctx, workloadExposure); err != nil {
			logger.Error(err, "Failed to update WorkloadExposure status")
			return ctrl.Result{}, err
		}
		logger.Info("Updated WorkloadExposure status", "url", exposureURL)
	}

	return ctrl.Result{}, nil
}

// getURLFromService generates a URL from the given Service
func (r *KubernetesRuntimeExposureReconciler) getURLFromService(service *corev1.Service) (string, error) {
	switch service.Spec.Type {
	case corev1.ServiceTypeLoadBalancer:
		return r.getURLFromLoadBalancer(service)
	case corev1.ServiceTypeNodePort:
		return r.getURLFromNodePort(service)
	case corev1.ServiceTypeClusterIP, "":
		// Handle both explicit ClusterIP and empty type (default ClusterIP)
		return r.getURLFromClusterIP(service)
	default:
		return "", nil
	}
}

// getURLFromLoadBalancer gets URL from LoadBalancer service
func (r *KubernetesRuntimeExposureReconciler) getURLFromLoadBalancer(service *corev1.Service) (string, error) {
	if len(service.Status.LoadBalancer.Ingress) == 0 {
		return "", nil
	}

	ingress := service.Status.LoadBalancer.Ingress[0]
	host := ingress.IP
	if host == "" {
		host = ingress.Hostname
	}
	if host == "" {
		return "", nil
	}

	if len(service.Spec.Ports) == 0 {
		return "", nil
	}

	port := service.Spec.Ports[0].Port
	serviceURL := fmt.Sprintf("http://%s:%d", host, port)

	if r.isValidURL(serviceURL) {
		return serviceURL, nil
	}

	return "", nil
}

// getURLFromNodePort gets URL from NodePort service
func (r *KubernetesRuntimeExposureReconciler) getURLFromNodePort(service *corev1.Service) (string, error) {
	if len(service.Spec.Ports) == 0 {
		return "", nil
	}

	nodePort := service.Spec.Ports[0].NodePort
	if nodePort == 0 {
		return "", nil
	}

	serviceURL := fmt.Sprintf("http://localhost:%d", nodePort)

	if r.isValidURL(serviceURL) {
		return serviceURL, nil
	}

	return "", nil
}

// getURLFromClusterIP gets URL from ClusterIP service
func (r *KubernetesRuntimeExposureReconciler) getURLFromClusterIP(service *corev1.Service) (string, error) {
	// Handle ClusterIP type services (including default/empty type)
	if service.Spec.Type != "" && service.Spec.Type != corev1.ServiceTypeClusterIP {
		return "", nil
	}

	if len(service.Spec.Ports) == 0 {
		return "", nil
	}

	port := service.Spec.Ports[0].Port
	serviceURL := fmt.Sprintf("http://localhost:%d", port)

	if r.isValidURL(serviceURL) {
		return serviceURL, nil
	}

	return "", nil
}

// isValidURL checks if the given URL string is valid
func (r *KubernetesRuntimeExposureReconciler) isValidURL(urlStr string) bool {
	if urlStr == "" {
		return false
	}

	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return false
	}

	return parsedURL.Scheme != "" && parsedURL.Host != ""
}

// SetupWithManager sets up the controller with the Manager.
func (r *KubernetesRuntimeExposureReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&scorev1b1.WorkloadExposure{}).
		Watches(
			&corev1.Service{},
			handler.EnqueueRequestsFromMapFunc(r.findWorkloadExposuresForService),
		).
		Complete(r)
}

// findWorkloadExposuresForService maps Service events to WorkloadExposure reconciliation requests
func (r *KubernetesRuntimeExposureReconciler) findWorkloadExposuresForService(ctx context.Context, obj client.Object) []reconcile.Request {
	service := obj.(*corev1.Service)

	// Check if this service has the workload label
	workloadName, exists := service.Labels["score.dev/workload"]
	if !exists {
		return nil
	}

	// Find WorkloadExposure with the same name
	workloadExposure := &scorev1b1.WorkloadExposure{}
	namespacedName := types.NamespacedName{
		Name:      workloadName,
		Namespace: service.Namespace,
	}

	if err := r.Get(ctx, namespacedName, workloadExposure); err != nil {
		return nil
	}

	// Only reconcile if this is a Kubernetes runtime WorkloadExposure
	if workloadExposure.Spec.RuntimeClass != kubernetesRuntimeClass {
		return nil
	}

	return []reconcile.Request{
		{NamespacedName: namespacedName},
	}
}
