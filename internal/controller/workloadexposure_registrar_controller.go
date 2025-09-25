/*
Copyright 2024 The Score Orchestrator Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"

	scorev1b1 "github.com/cappyzawa/score-orchestrator/api/v1b1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// WorkloadExposureRegistrar watches Workload and ensures a spec-only WorkloadExposure exists.
// +kubebuilder:rbac:groups=score.dev,resources=workloads,verbs=get;list;watch
// +kubebuilder:rbac:groups=score.dev,resources=workloadexposures,verbs=create;get;list;watch;update;patch;delete
// +kubebuilder:rbac:groups=score.dev,resources=workloadexposures/status,verbs=get;watch
type WorkloadExposureRegistrar struct {
	client.Client
	Scheme       *runtime.Scheme
	Recorder     record.EventRecorder
	RuntimeClass string
}

// Reconcile registers WorkloadExposure resources (spec-only) for Workloads.
func (r *WorkloadExposureRegistrar) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx)

	// 1) Fetch Workload
	var wl scorev1b1.Workload
	if err := r.Get(ctx, req.NamespacedName, &wl); err != nil {
		if apierrors.IsNotFound(err) {
			// Workload was deleted, let GC handle WorkloadExposure cleanup
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch Workload")
		return ctrl.Result{}, err
	}

	// avoid creating/updating exposure during workload deletion; let ownerRef GC handle cleanup
	if wl.DeletionTimestamp != nil {
		log.V(1).Info("Workload is being deleted, skipping WorkloadExposure operations", "workload", req.NamespacedName)
		return ctrl.Result{}, nil
	}

	// 2) Desired WorkloadExposure (spec-only)
	desired := scorev1b1.WorkloadExposure{
		ObjectMeta: metav1.ObjectMeta{
			Name:      wl.Name,
			Namespace: wl.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(&wl, scorev1b1.GroupVersion.WithKind("Workload")),
			},
		},
		Spec: scorev1b1.WorkloadExposureSpec{
			WorkloadRef: scorev1b1.WorkloadExposureWorkloadRef{
				Name:      wl.Name,
				Namespace: ptr.To(wl.Namespace),
				UID:       string(wl.UID),
			},
			RuntimeClass:               r.RuntimeClass,
			ObservedWorkloadGeneration: wl.Generation,
		},
	}

	// 3) Create or Patch (spec-only)
	var we scorev1b1.WorkloadExposure
	err := r.Get(ctx, types.NamespacedName{Name: wl.Name, Namespace: wl.Namespace}, &we)
	switch {
	case apierrors.IsNotFound(err):
		// Create new WorkloadExposure
		if err := r.Create(ctx, &desired); err != nil {
			log.Error(err, "failed to create WorkloadExposure")
			return ctrl.Result{}, err
		}
		r.Recorder.Eventf(&wl, corev1.EventTypeNormal, "ExposureRegistered", "WorkloadExposure %s created with runtimeClass=%s", desired.Name, r.RuntimeClass)
		log.Info("WorkloadExposure created", "workload", req.NamespacedName)
		return ctrl.Result{}, nil
	case err != nil:
		log.Error(err, "unable to fetch WorkloadExposure")
		return ctrl.Result{}, err
	default:
		// Patch spec if changed (do not touch status)
		if updateReason := r.getUpdateReason(&we, &desired); updateReason != "" {
			patch := client.MergeFrom(we.DeepCopy())
			// Only replace Spec; keep metadata/status
			we.Spec = desired.Spec
			if err := r.Patch(ctx, &we, patch); err != nil {
				log.Error(err, "failed to patch WorkloadExposure")
				return ctrl.Result{}, err
			}
			r.Recorder.Eventf(&wl, corev1.EventTypeNormal, "ExposureUpdated", "WorkloadExposure %s spec updated: %s", we.Name, updateReason)
			log.Info("WorkloadExposure spec updated", "workload", req.NamespacedName, "reason", updateReason)
		}
		return ctrl.Result{}, nil
	}
}

// getUpdateReason checks if the WorkloadExposure spec needs to be updated and returns the reason.
// Returns empty string if no update is needed.
func (r *WorkloadExposureRegistrar) getUpdateReason(current, desired *scorev1b1.WorkloadExposure) string {
	var reasons []string

	if current.Spec.ObservedWorkloadGeneration != desired.Spec.ObservedWorkloadGeneration {
		reasons = append(reasons, "generation")
	}
	if current.Spec.RuntimeClass != desired.Spec.RuntimeClass {
		reasons = append(reasons, "runtimeClass")
	}
	if current.Spec.WorkloadRef.Name != desired.Spec.WorkloadRef.Name {
		reasons = append(reasons, "workloadRef.name")
	}
	if current.Spec.WorkloadRef.UID != desired.Spec.WorkloadRef.UID {
		reasons = append(reasons, "workloadRef.uid")
	}
	if (current.Spec.WorkloadRef.Namespace == nil && desired.Spec.WorkloadRef.Namespace != nil) ||
		(current.Spec.WorkloadRef.Namespace != nil && desired.Spec.WorkloadRef.Namespace == nil) ||
		(current.Spec.WorkloadRef.Namespace != nil && desired.Spec.WorkloadRef.Namespace != nil &&
			*current.Spec.WorkloadRef.Namespace != *desired.Spec.WorkloadRef.Namespace) {
		reasons = append(reasons, "workloadRef.namespace")
	}

	if len(reasons) == 0 {
		return ""
	}
	return "changed " + reasons[0] + func() string {
		if len(reasons) > 1 {
			return " (and " + fmt.Sprintf("%d", len(reasons)-1) + " more)"
		}
		return ""
	}()
}

// SetupWithManager sets up the controller with the Manager.
func (r *WorkloadExposureRegistrar) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&scorev1b1.Workload{}).
		Owns(&scorev1b1.WorkloadExposure{}). // for GC awareness
		// Watch WorkloadPlan to trigger Workload reconciliation when Plans are created
		Watches(&scorev1b1.WorkloadPlan{},
			handler.EnqueueRequestForOwner(mgr.GetScheme(), mgr.GetRESTMapper(), &scorev1b1.Workload{}, handler.OnlyControllerOwner())).
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		Named("workload-exposure-registrar").
		Complete(r)
}
