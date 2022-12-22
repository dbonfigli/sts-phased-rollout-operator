/*
Copyright 2022.

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

package controllers

import (
	"context"
	//"encoding/json"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	//"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	stsplusv1alpha1 "github.com/dbonfigli/sts-plus-operator/api/v1alpha1"
)

const managedByAnnotation = "sts.plus/phasedRollout"

// PhasedRolloutReconciler reconciles a PhasedRollout object
type PhasedRolloutReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=sts.plus,resources=phasedrollouts,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=sts.plus,resources=phasedrollouts/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=sts.plus,resources=phasedrollouts/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;update;watch;patch
//+kubebuilder:rbac:groups=apps,resources=statefulsets/status,verbs=get

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the PhasedRollout object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.13.0/pkg/reconcile
func (r *PhasedRolloutReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.V(10).Info("starting reconciliation", "name", req.Name)

	// get target sts
	// check if pods need to roll
	// perform promql expr
	// roll node

	// get phasedRollout
	var phasedRollout stsplusv1alpha1.PhasedRollout
	if err := r.Get(ctx, req.NamespacedName, &phasedRollout); err != nil {
		log.Error(err, "unable to fetch PhasedRollout")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// get sts
	var sts appsv1.StatefulSet
	mustUpdateSts := false
	stsNamespacedName := client.ObjectKey{
		Namespace: req.Namespace,
		Name:      phasedRollout.Spec.TargetRef,
	}
	if err := r.Get(ctx, stsNamespacedName, &sts); err != nil {
		log.Error(err, "unable to fetch STS")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// check if can manage sts
	updateStrategy := sts.Spec.UpdateStrategy.Type
	if updateStrategy != appsv1.RollingUpdateStatefulSetStrategyType {
		log.Info("sts has not RollingUpdate as UpdateStrategy, cannot manage it", "stsName", sts.Name, "UpdateStrategy", updateStrategy)
		return ctrl.Result{}, nil
	}
	if sts.Annotations == nil {
		sts.Annotations = make(map[string]string)
	}
	reportedPhasedRolloutName, ok := sts.Annotations[managedByAnnotation]
	if ok {
		// check if another phasedRollout exists for this sts and if it is properly configured
		if reportedPhasedRolloutName != phasedRollout.Name {
			var otherPhasedRollout stsplusv1alpha1.PhasedRollout
			otherPhasedRolloutName := client.ObjectKey{
				Namespace: req.Namespace,
				Name:      reportedPhasedRolloutName,
			}
			if err := r.Get(ctx, otherPhasedRolloutName, &otherPhasedRollout); err != nil { //TODO check if the error is not found
				log.Info("sts reports to be managed by another phasedRollout, but this other phasedRollout does not exist, will overwrite it", "stsName", sts.Name, "reportedPhasedRolloutName", reportedPhasedRolloutName)
				sts.Annotations[managedByAnnotation] = phasedRollout.Name
				mustUpdateSts = true
			} else if otherPhasedRollout.Spec.TargetRef != sts.Name {
				log.Info("sts reports to be managed by another phasedRollout, but this other phasedRollout does not report to have this sts as target, will overwrite it", "stsName", sts.Name, "reportedPhasedRolloutName", reportedPhasedRolloutName, "reportedPhasedRolloutTargetRef", otherPhasedRollout.Spec.TargetRef)
				sts.Annotations[managedByAnnotation] = phasedRollout.Name
				mustUpdateSts = true
			} else {
				log.Info("sts is managed by another phasedRollout", "stsName", sts.Name, "reportedPhasedRolloutName", reportedPhasedRolloutName)
				return ctrl.Result{}, nil
			}
		}
	} else {
		log.Info("sts not managed by any other phasedRollot, ok to manage", "stsName", sts.Name)
		sts.Annotations[managedByAnnotation] = phasedRollout.Name
		mustUpdateSts = true
	}

	// update partition
	if sts.Status.CurrentRevision == sts.Status.UpdateRevision {
		// no need to rolling pod
		if sts.Spec.Replicas == nil {
			log.Info("Spec.Replicas not found in sts, cannot cdheck if Spec.UpdateStrategy.RollingUpdate.Partition must be updated", "stsName", sts.Name)
			return ctrl.Result{}, nil
		}
		if sts.Spec.UpdateStrategy.RollingUpdate == nil {
			sts.Spec.UpdateStrategy.RollingUpdate = &appsv1.RollingUpdateStatefulSetStrategy{}
		}
		if sts.Spec.UpdateStrategy.RollingUpdate.Partition == nil || *sts.Spec.UpdateStrategy.RollingUpdate.Partition != *sts.Spec.Replicas {
			partition := *sts.Spec.Replicas
			sts.Spec.UpdateStrategy.RollingUpdate.Partition = &partition
			mustUpdateSts = true
		}
	} else {
		// must check promql and do rollout
		log.Info("must do rollout")
	}

	if mustUpdateSts {
		if err := r.Update(ctx, &sts); err != nil {
			log.Error(err, "unable to update STS")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func mapSTSToPhasedRollout(o client.Object) []reconcile.Request {
	log := log.FromContext(context.Background())
	phasedRolloutName, ok := o.GetAnnotations()[managedByAnnotation]
	if ok {
		log.V(10).Info("found annotation in sts for mapped request", "stsName", o.GetName(), "phasedRolloutName", phasedRolloutName)
		return []reconcile.Request{
			{
				NamespacedName: types.NamespacedName{
					Namespace: o.GetNamespace(),
					Name:      phasedRolloutName,
				},
			},
		}
	}

	// TODO look for all phasedRollouts
	// if not there
	log.V(10).Info("no annotation in sts for mapped request and no phasedRollout found for this sts", "stsName", o.GetName())
	return []reconcile.Request{}

}

// SetupWithManager sets up the controller with the Manager.
func (r *PhasedRolloutReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&stsplusv1alpha1.PhasedRollout{}).
		Watches(&source.Kind{Type: &appsv1.StatefulSet{}}, handler.EnqueueRequestsFromMapFunc(mapSTSToPhasedRollout)).
		Complete(r)
}
