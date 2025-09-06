/*
Copyright 2025.

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

package resolver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	scorev1b1 "github.com/cappyzawa/score-orchestrator/api/v1b1"
)

const (
	// StaticURIType is the resource type handled by this resolver
	StaticURIType = "static-uri"
)

// StaticURIController reconciles ResourceBinding objects with type "static-uri".
type StaticURIController struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// RBAC permissions are defined separately in config/rbac/role_static_uri_resolver.yaml
// This controller uses a separate ClusterRole to enforce single-writer principle

// Reconcile implements the reconciliation logic for static-uri ResourceBindings.
func (r *StaticURIController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the ResourceBinding
	rb := &scorev1b1.ResourceBinding{}
	if err := r.Get(ctx, req.NamespacedName, rb); err != nil {
		if errors.IsNotFound(err) {
			logger.V(1).Info("ResourceBinding not found, may have been deleted")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Only handle static-uri type
	if rb.Spec.Type != StaticURIType {
		logger.V(1).Info("ResourceBinding type is not static-uri, skipping", "type", rb.Spec.Type)
		return ctrl.Result{}, nil
	}

	return r.reconcileStaticURI(ctx, rb)
}

func (r *StaticURIController) reconcileStaticURI(ctx context.Context, rb *scorev1b1.ResourceBinding) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Create a copy for patching
	before := rb.DeepCopy()

	// Extract URI from params
	uri, missing, err := r.extractURI(rb)
	if err != nil {
		// JSON parse failure ⇒ Warning + short requeue
		logger.V(1).Info("Failed to parse params JSON", "error", err.Error())
		r.Recorder.Eventf(rb, "Warning", "ParamsInvalid", "Failed to parse params")
		return ctrl.Result{RequeueAfter: 3 * time.Second}, nil
	}

	if missing || strings.TrimSpace(uri) == "" {
		// Check if already in Failed/SpecInvalid state (idempotent)
		if r.isAlreadyInFailedState(rb) {
			logger.V(1).Info("ResourceBinding already in failed state")
			return ctrl.Result{}, nil
		}
		// Input invalid ⇒ Failed/SpecInvalid state
		return r.updateToFailedState(ctx, rb, before, "Required param 'uri' is missing.")
	}

	// Check if status is already in desired Bound state (idempotent)
	if r.isAlreadyInBoundState(rb, uri) {
		logger.V(1).Info("ResourceBinding already in bound state")
		return ctrl.Result{}, nil
	}

	// Update status to Bound state
	return r.updateToBoundState(ctx, rb, before, uri)
}

// extractURI extracts the URI from ResourceBinding params.
// Returns: (uri, missing, error)
// - error: JSON parse errors
// - missing: true if 'uri' param is missing or not a string
// - uri: the extracted URI value (may be empty)
func (r *StaticURIController) extractURI(rb *scorev1b1.ResourceBinding) (string, bool, error) {
	if rb.Spec.Params == nil {
		return "", true, nil
	}

	var params map[string]interface{}
	if err := json.Unmarshal(rb.Spec.Params.Raw, &params); err != nil {
		return "", false, fmt.Errorf("failed to unmarshal params: %w", err)
	}

	uriValue, exists := params["uri"]
	if !exists {
		return "", true, nil
	}

	uri, ok := uriValue.(string)
	if !ok {
		return "", true, nil
	}

	return uri, false, nil
}

// updateToFailedState updates the ResourceBinding to Failed/SpecInvalid state.
func (r *StaticURIController) updateToFailedState(ctx context.Context, rb *scorev1b1.ResourceBinding, before *scorev1b1.ResourceBinding, message string) (ctrl.Result, error) {
	now := metav1.NewTime(time.Now())
	rb.Status.Phase = scorev1b1.ResourceBindingPhaseFailed
	rb.Status.Reason = "SpecInvalid"
	rb.Status.Message = message
	rb.Status.OutputsAvailable = false
	rb.Status.Outputs = scorev1b1.ResourceBindingOutputs{}
	rb.Status.ObservedGeneration = rb.Generation
	rb.Status.LastTransitionTime = &now

	// Patch the status
	if err := r.Status().Patch(ctx, rb, client.MergeFrom(before)); err != nil {
		if errors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, err
	}

	r.Recorder.Eventf(rb, "Warning", "SpecInvalid", "Required param 'uri' is missing.")
	return ctrl.Result{}, nil
}

// updateToBoundState updates the ResourceBinding to Bound/Succeeded state.
func (r *StaticURIController) updateToBoundState(ctx context.Context, rb *scorev1b1.ResourceBinding, before *scorev1b1.ResourceBinding, uri string) (ctrl.Result, error) {
	now := metav1.NewTime(time.Now())
	rb.Status.Phase = scorev1b1.ResourceBindingPhaseBound
	rb.Status.Reason = "Succeeded"
	rb.Status.Message = "Outputs are available."
	rb.Status.OutputsAvailable = true
	rb.Status.Outputs = scorev1b1.ResourceBindingOutputs{
		URI: &uri,
	}
	rb.Status.ObservedGeneration = rb.Generation
	rb.Status.LastTransitionTime = &now

	// Patch the status
	if err := r.Status().Patch(ctx, rb, client.MergeFrom(before)); err != nil {
		if errors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, err
	}

	r.Recorder.Eventf(rb, "Normal", "Bound", "Published uri=%s.", uri)
	return ctrl.Result{}, nil
}

// isAlreadyInFailedState checks if the ResourceBinding is already in the Failed/SpecInvalid state.
func (r *StaticURIController) isAlreadyInFailedState(rb *scorev1b1.ResourceBinding) bool {
	return rb.Status.Phase == scorev1b1.ResourceBindingPhaseFailed &&
		rb.Status.Reason == "SpecInvalid" &&
		!rb.Status.OutputsAvailable &&
		rb.Status.ObservedGeneration == rb.Generation
}

// isAlreadyInBoundState checks if the ResourceBinding is already in the Bound state with the expected URI.
func (r *StaticURIController) isAlreadyInBoundState(rb *scorev1b1.ResourceBinding, expectedURI string) bool {
	return rb.Status.Phase == scorev1b1.ResourceBindingPhaseBound &&
		rb.Status.Reason == "Succeeded" &&
		rb.Status.OutputsAvailable &&
		rb.Status.Outputs.URI != nil &&
		*rb.Status.Outputs.URI == expectedURI &&
		rb.Status.ObservedGeneration == rb.Generation
}

// SetupWithManager sets up the controller with the Manager.
func (r *StaticURIController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&scorev1b1.ResourceBinding{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 1,
		}).
		WithEventFilter(predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool {
				rb, ok := e.Object.(*scorev1b1.ResourceBinding)
				return ok && rb.Spec.Type == StaticURIType
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				oldRB, okOld := e.ObjectOld.(*scorev1b1.ResourceBinding)
				newRB, okNew := e.ObjectNew.(*scorev1b1.ResourceBinding)
				if !okOld || !okNew || newRB.Spec.Type != StaticURIType {
					return false
				}
				// Reconcile on Generation change or if not Bound yet
				return oldRB.GetGeneration() != newRB.GetGeneration() ||
					newRB.Status.Phase != scorev1b1.ResourceBindingPhaseBound
			},
			DeleteFunc:  func(event.DeleteEvent) bool { return false },
			GenericFunc: func(event.GenericEvent) bool { return false },
		}).
		Complete(r)
}
