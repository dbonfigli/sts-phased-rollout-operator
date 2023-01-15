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
	"time"

	appsv1 "k8s.io/api/apps/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
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

//remember to track events
//remember to update state
//clean on delete phasedRollout
//backup used for clean up

// get target sts
// check if pods need to roll
// perform promql expr
// roll node

func (r *PhasedRolloutReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.V(10).Info("starting reconciliation")
	defer func() {
		log.V(10).Info("ending reconciliation")
	}()

	// get phasedRollout
	var phasedRollout stsplusv1alpha1.PhasedRollout
	if err := r.Get(ctx, req.NamespacedName, &phasedRollout); err != nil {
		if apierrs.IsNotFound(err) {
			// normal if the CR has been deleted
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch phasedRollout")
		return ctrl.Result{}, err
	}

	// get sts targeting this phasedRollout
	var sts appsv1.StatefulSet
	stsNamespacedName := client.ObjectKey{
		Namespace: req.Namespace,
		Name:      phasedRollout.Spec.TargetRef,
	}
	if err := r.Get(ctx, stsNamespacedName, &sts); err != nil {
		if apierrs.IsNotFound(err) {
			log.V(10).Info("sts no found, will retry in 30 seconds", "stsName", sts.Name)
			if phasedRollout.Status.Status != stsplusv1alpha1.PhasedRollotErrorSTSNotFound {
				phasedRollout.Status.Status = stsplusv1alpha1.PhasedRollotErrorSTSNotFound
				phasedRollout.Status.Message = "target sts " + phasedRollout.Spec.TargetRef + "not found in namespace " + req.Namespace
				return ctrl.Result{RequeueAfter: 30 * time.Second}, r.Status().Update(ctx, &phasedRollout)
			}
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		log.Error(err, "unable to fetch sts")
		return ctrl.Result{}, err
	}

	// check if we should manage the sts
	okToManage, needReconciliation, err := r.manageSTS(ctx, &sts, &phasedRollout)
	if err != nil {
		return ctrl.Result{}, err
	}
	if needReconciliation {
		return ctrl.Result{Requeue: true}, nil
	}
	if !okToManage {
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// check if the UpdateStrategy is RollingUpdate
	if sts.Spec.UpdateStrategy.Type != appsv1.RollingUpdateStatefulSetStrategyType {
		if phasedRollout.Status.Status != stsplusv1alpha1.PhasedRollotErrorCannotManage {
			message := "sts has not RollingUpdate as UpdateStrategy, cannot manage it"
			log.Info(message, "stsName", sts.Name, "UpdateStrategy", sts.Spec.UpdateStrategy.Type)
			phasedRollout.Status.Status = stsplusv1alpha1.PhasedRollotErrorCannotManage
			phasedRollout.Status.Message = message
			return ctrl.Result{}, r.Status().Update(ctx, &phasedRollout)
		}
		return ctrl.Result{}, nil
	}

	// check if we need to suspend the phased rollout
	if phasedRollout.Spec.StandardRollingUpdate {
		log.V(10).Info("phased rollout is suspended (StandardRollingUpdate = true)")
		if sts.Spec.UpdateStrategy.RollingUpdate != nil &&
			sts.Spec.UpdateStrategy.RollingUpdate.Partition != nil &&
			*sts.Spec.UpdateStrategy.RollingUpdate.Partition != 0 {
			log.V(10).Info("removing the sts RollingUpdate partition config because of StandardRollingUpdate = true", "stsName", sts.Name)
			sts.Spec.UpdateStrategy.RollingUpdate.Partition = nil
			return ctrl.Result{Requeue: true}, r.Update(ctx, &sts)
		}
		if phasedRollout.Status.Status != stsplusv1alpha1.PhasedRollotSuspened {
			log.V(10).Info("setting phasedRollout state", "state", stsplusv1alpha1.PhasedRollotSuspened)
			phasedRollout.Status.Status = stsplusv1alpha1.PhasedRollotSuspened
			phasedRollout.Status.Message = "phased rollout mechanism suspended because of StandardRollingUpdate = true"
			return ctrl.Result{}, r.Status().Update(ctx, &phasedRollout)
		}
		return ctrl.Result{}, nil
	}

	// if no updates needed set partition to prevent unmanaged future rollouts
	if sts.Status.CurrentRevision == sts.Status.UpdateRevision {
		if phasedRollout.Status.Status != stsplusv1alpha1.PhasedRollotUpdated {
			log.V(10).Info("setting phasedRollout state", "state", stsplusv1alpha1.PhasedRollotUpdated)
			phasedRollout.Status.Status = stsplusv1alpha1.PhasedRollotUpdated
			phasedRollout.Status.Message = "all pods updated to the current revision"
			return ctrl.Result{Requeue: true}, r.Status().Update(ctx, &phasedRollout)
		}

		if sts.Spec.Replicas == nil {
			var one int32 = 1
			sts.Spec.Replicas = &one
		}
		if sts.Spec.UpdateStrategy.RollingUpdate == nil {
			sts.Spec.UpdateStrategy.RollingUpdate = &appsv1.RollingUpdateStatefulSetStrategy{}
		}
		if sts.Spec.UpdateStrategy.RollingUpdate.Partition == nil ||
			*sts.Spec.UpdateStrategy.RollingUpdate.Partition != *sts.Spec.Replicas {
			partition := *sts.Spec.Replicas
			sts.Spec.UpdateStrategy.RollingUpdate.Partition = &partition
			log.V(10).Info("updating partition to prevent unmanaged rollouts", "stsName", sts.Name, "partition", partition)
			return ctrl.Result{}, r.Update(ctx, &sts)
		}
	}

	// must check promql and do rollout
	log.Info("must do rollout")

	return ctrl.Result{}, nil
}

// manageSTS set the necessary fields in the sts if it should be managed by this phased rollout
// returns:
// the first return value is true if it is ok to manage the sts
// the second return value is true we need to do a reconciliation after an apply of a change in the sts or phasedRollout
// the third return value is an error if any
func (r *PhasedRolloutReconciler) manageSTS(ctx context.Context, sts *appsv1.StatefulSet, phasedRollout *stsplusv1alpha1.PhasedRollout) (bool, bool, error) {
	log := log.FromContext(ctx)

	if sts.Annotations == nil {
		sts.Annotations = make(map[string]string)
	}

	reportedPhasedRolloutName, ok := sts.Annotations[managedByAnnotation]
	if !ok {
		// sts not managed by any phasedRollout, take control it
		log.V(10).Info("sts not managed by any other phasedRollot, ok to manage", "stsName", sts.Name)
		sts.Annotations[managedByAnnotation] = phasedRollout.Name
		return true, true, r.Update(ctx, sts)
	}

	if reportedPhasedRolloutName == phasedRollout.Name {
		// this sts is already managed by this phasedRollout
		return true, false, nil
	}

	// this sts seems to be managed by another phasedRollout

	var otherPhasedRollout stsplusv1alpha1.PhasedRollout
	otherPhasedRolloutName := client.ObjectKey{
		Namespace: sts.Namespace,
		Name:      reportedPhasedRolloutName,
	}
	if err := r.Get(ctx, otherPhasedRolloutName, &otherPhasedRollout); err != nil {
		if !apierrs.IsNotFound(err) {
			log.Error(err, "unable to fetch phasedRollout")
			return false, false, err
		}
		log.V(10).Info("sts reports to be managed by another phasedRollout, but this other phasedRollout does not exist, will overwrite it", "stsName", sts.Name, "reportedPhasedRolloutName", reportedPhasedRolloutName)
		sts.Annotations[managedByAnnotation] = phasedRollout.Name
		return true, true, r.Update(ctx, sts)
	}

	if otherPhasedRollout.Spec.TargetRef != sts.Name {
		log.V(10).Info("sts reports to be managed by another phasedRollout, but this other phasedRollout does not report to have this sts as target, will overwrite it", "stsName", sts.Name, "reportedPhasedRolloutName", reportedPhasedRolloutName, "reportedPhasedRolloutTargetRef", otherPhasedRollout.Spec.TargetRef)
		sts.Annotations[managedByAnnotation] = phasedRollout.Name
		return true, true, r.Update(ctx, sts)
	}

	// this sts seems to be legitimately managed by another phasedRollout
	if phasedRollout.Status.Status != stsplusv1alpha1.PhasedRollotErrorCannotManage {
		log.Info("sts is managed by another phasedRollout", "stsName", sts.Name, "reportedPhasedRolloutName", reportedPhasedRolloutName)
		phasedRollout.Status.Status = stsplusv1alpha1.PhasedRollotErrorCannotManage
		phasedRollout.Status.Message = "sts is managed by another phasedRollout"
		return false, true, r.Status().Update(ctx, phasedRollout)
	}
	return false, false, nil

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

	// TODO what to do here? look for all phasedRollouts if not there?
	log.V(10).Info("no annotation in sts for mapped request", "stsName", o.GetName(), "annotation", managedByAnnotation)
	return []reconcile.Request{}

}

// SetupWithManager sets up the controller with the Manager.
func (r *PhasedRolloutReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&stsplusv1alpha1.PhasedRollout{}).
		Watches(&source.Kind{Type: &appsv1.StatefulSet{}},
			handler.EnqueueRequestsFromMapFunc(mapSTSToPhasedRollout),
		).
		Complete(r)
}
