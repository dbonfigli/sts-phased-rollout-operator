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
	"strconv"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	stsplusv1alpha1 "github.com/dbonfigli/sts-plus-operator/api/v1alpha1"
	"github.com/dbonfigli/sts-plus-operator/pkg/prometheus"
)

const managedByAnnotation = "sts.plus/phasedRollout"
const finalizerAnnotation = "sts.plus/finalizer"

// PhasedRolloutReconciler reconciles a PhasedRollout object.
type PhasedRolloutReconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	Recorder         record.EventRecorder
	RetryWaitSeconds int
}

//+kubebuilder:rbac:groups=sts.plus,resources=phasedrollouts,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=sts.plus,resources=phasedrollouts/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=sts.plus,resources=phasedrollouts/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;update;watch;patch
//+kubebuilder:rbac:groups=apps,resources=statefulsets/status,verbs=get
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch

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
			// normal if the custom resource has been deleted
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to get phasedRollout")
		return ctrl.Result{}, err
	}

	// add finalizer if the phasedRollout has not been marked for deletion
	if phasedRollout.ObjectMeta.DeletionTimestamp.IsZero() && !controllerutil.ContainsFinalizer(&phasedRollout, finalizerAnnotation) {
		log.V(10).Info("adding finalizer")
		controllerutil.AddFinalizer(&phasedRollout, finalizerAnnotation)
		return ctrl.Result{}, r.Update(ctx, &phasedRollout)
	}

	// get sts targeting this phasedRollout
	var sts appsv1.StatefulSet
	stsNamespacedName := client.ObjectKey{
		Namespace: req.Namespace,
		Name:      phasedRollout.Spec.TargetRef,
	}
	if err := r.Get(ctx, stsNamespacedName, &sts); err != nil {
		if apierrs.IsNotFound(err) {
			// if the phasedRollout has been marked for deletion and has a finalizer, remove it, it is ready to be deleted
			if !phasedRollout.ObjectMeta.DeletionTimestamp.IsZero() && controllerutil.ContainsFinalizer(&phasedRollout, finalizerAnnotation) {
				log.V(10).Info("removing finalizer, sts is not found", "stsName", phasedRollout.Spec.TargetRef)
				controllerutil.RemoveFinalizer(&phasedRollout, finalizerAnnotation)
				return ctrl.Result{}, r.Update(ctx, &phasedRollout)
			}
			log.V(10).Info("sts no found, will retry after a backoff", "stsName", sts.Name)
			if phasedRollout.Status.Status != stsplusv1alpha1.PhasedRollotErrorSTSNotFound {
				phasedRollout.Status.Status = stsplusv1alpha1.PhasedRollotErrorSTSNotFound
				phasedRollout.Status.Message = "target sts " + phasedRollout.Spec.TargetRef + "not found in namespace " + req.Namespace
				return ctrl.Result{RequeueAfter: time.Duration(r.RetryWaitSeconds) * time.Second}, r.Status().Update(ctx, &phasedRollout)
			}
			return ctrl.Result{RequeueAfter: time.Duration(r.RetryWaitSeconds) * time.Second}, nil
		}
		log.Error(err, "unable to get sts", "stsName", sts.Name)
		return ctrl.Result{}, err
	}

	// if the phasedRollout has been marked for deletion, clean up the sts removing the partition config that was added
	if !phasedRollout.ObjectMeta.DeletionTimestamp.IsZero() && controllerutil.ContainsFinalizer(&phasedRollout, finalizerAnnotation) {
		if sts.Annotations[managedByAnnotation] == phasedRollout.Name {
			log.V(10).Info("cleaning up the sts before removing the phasedRollout", "stsName", sts.Name)
			delete(sts.Annotations, managedByAnnotation)
			if sts.Spec.UpdateStrategy.RollingUpdate != nil && sts.Spec.UpdateStrategy.RollingUpdate.Partition != nil && *sts.Spec.UpdateStrategy.RollingUpdate.Partition != 0 {
				if sts.Spec.UpdateStrategy.RollingUpdate.MaxUnavailable == nil {
					sts.Spec.UpdateStrategy.RollingUpdate = nil
				} else {
					sts.Spec.UpdateStrategy.RollingUpdate.Partition = nil
				}
			}
			return ctrl.Result{}, r.Update(ctx, &sts)
		}
		log.V(10).Info("removing finalizer, sts has no partition config to clean up", "stsName", sts.Name)
		controllerutil.RemoveFinalizer(&phasedRollout, finalizerAnnotation)
		return ctrl.Result{}, r.Update(ctx, &phasedRollout)
	}

	// check if we should manage the sts
	okToManage, needReconciliation, err := r.manageSTS(ctx, &sts, &phasedRollout)
	if err != nil {
		return ctrl.Result{}, err
	}
	if needReconciliation {
		return ctrl.Result{}, nil
	}
	if !okToManage {
		return ctrl.Result{RequeueAfter: time.Duration(r.RetryWaitSeconds) * time.Second}, nil
	}

	// check if the UpdateStrategy is RollingUpdate
	if sts.Spec.UpdateStrategy.Type != appsv1.RollingUpdateStatefulSetStrategyType {
		if phasedRollout.Status.Status != stsplusv1alpha1.PhasedRollotErrorCannotManage {
			message := "sts has not RollingUpdate as UpdateStrategy, cannot manage it"
			log.Info(message, "stsName", sts.Name, "UpdateStrategy", sts.Spec.UpdateStrategy.Type)
			r.Recorder.Eventf(&phasedRollout, "Warning", "CannotManage", message)
			phasedRollout.Status.Status = stsplusv1alpha1.PhasedRollotErrorCannotManage
			phasedRollout.Status.Message = message
			return ctrl.Result{}, r.Status().Update(ctx, &phasedRollout)
		}
		return ctrl.Result{}, nil
	}

	// check if we need to suspend the phased rollout
	if phasedRollout.Spec.StandardRollingUpdate {
		log.V(10).Info("phased rollout is suspended (StandardRollingUpdate = true)")
		if sts.Spec.UpdateStrategy.RollingUpdate != nil && sts.Spec.UpdateStrategy.RollingUpdate.Partition != nil && *sts.Spec.UpdateStrategy.RollingUpdate.Partition != 0 {
			log.V(10).Info("removing the sts RollingUpdate partition config because of StandardRollingUpdate = true", "stsName", sts.Name)
			sts.Spec.UpdateStrategy.RollingUpdate.Partition = nil
			return ctrl.Result{}, r.Update(ctx, &sts)
		}
		if phasedRollout.Status.Status != stsplusv1alpha1.PhasedRollotSuspened {
			log.Info("setting phasedRollout state", "state", stsplusv1alpha1.PhasedRollotSuspened)
			phasedRollout.Status.Status = stsplusv1alpha1.PhasedRollotSuspened
			phasedRollout.Status.Message = "phased rollout mechanism suspended because of StandardRollingUpdate = true"
			return ctrl.Result{}, r.Status().Update(ctx, &phasedRollout)
		}
		return ctrl.Result{}, nil
	}

	// if no updates needed, set partition to prevent unmanaged future rollouts
	if sts.Status.CurrentRevision == sts.Status.UpdateRevision {
		if phasedRollout.Status.Status != stsplusv1alpha1.PhasedRollotUpdated {
			log.Info("setting phasedRollout state", "state", stsplusv1alpha1.PhasedRollotUpdated)
			// if there was an ongoing phased rollout, then it has been completed, set RolloutEndTime and remove RollingPodStatus
			if phasedRollout.Status.Status == stsplusv1alpha1.PhasedRollotRolling {
				r.Recorder.Eventf(&phasedRollout, "Normal", "RolloutCompleted", "the phased rollout is completed")
				phasedRollout.Status.RolloutEndTime = time.Now().Format(time.RFC3339)
			}
			phasedRollout.Status.Status = stsplusv1alpha1.PhasedRollotUpdated
			phasedRollout.Status.Message = "all pods updated to the current revision"
			phasedRollout.Status.RollingPodStatus = nil
			return ctrl.Result{}, r.Status().Update(ctx, &phasedRollout)
		}
		mustReconcile, err := r.preventUncontrolledRollouts(ctx, &sts)
		if err != nil {
			return ctrl.Result{}, err
		}
		if mustReconcile {
			return ctrl.Result{}, nil
		}
	} else {
		// need to perform a rollout
		return r.rollout(ctx, &sts, &phasedRollout)
	}

	return ctrl.Result{}, nil
}

// preventUncontrolledRollouts set sts.Spec.UpdateStrategy.RollingUpdate.Partition to a value so that arbitrary rollouts are disabled.
// Returns:
// the first return value is true we need to do a reconciliation after an apply of a change;
// the second return value is an error if any.
func (r *PhasedRolloutReconciler) preventUncontrolledRollouts(ctx context.Context, sts *appsv1.StatefulSet) (bool, error) {
	log := log.FromContext(ctx)

	if sts.Spec.Replicas == nil {
		var one int32 = 1
		sts.Spec.Replicas = &one
	}
	if sts.Spec.UpdateStrategy.RollingUpdate == nil {
		sts.Spec.UpdateStrategy.RollingUpdate = &appsv1.RollingUpdateStatefulSetStrategy{}
	}
	if sts.Spec.UpdateStrategy.RollingUpdate.Partition == nil || *sts.Spec.UpdateStrategy.RollingUpdate.Partition != *sts.Spec.Replicas {
		partition := *sts.Spec.Replicas
		sts.Spec.UpdateStrategy.RollingUpdate.Partition = &partition

		log.V(10).Info("updating sts.Spec.UpdateStrategy.RollingUpdate.Partition to sts.Spec.Replicas to prevent uncontrolled rollouts", "stsName", sts.Name, "partition", partition)
		err := r.Update(ctx, sts)
		return true, err
	}
	return false, nil
}

// rollout handles the reconciliation during updates.

// During a rollout, the reconciliation generally cyclses through `phasedRollout.Status.RollingPodStatus.Status` statuses this way:
//  1. RollingPodWaitForPodToBeUpdated (wait for the pod to the right of the partition to have the current sts revision);
//  2. RollingPodWaitForAllPodsToBeAvailable (wait for the sts to have all pods available);
//  3. RollingPodWaitForInitialDelay (wait InitialDelay before starting checks, this help collect proper prometheus metrics before consulting them in checks);
//  4. RollingPodWaitForChecks (or PrometheusError) (perform prometheus checks):
//     a. repeat checks until all checks passed decrease sts partition;
//     b. if sts partition == 0 wait for the sts to be updated;
//  5. update phasedRollout.Status.RollingPodStatus.Partition to match sts.partition and set RollingPodWaitForPodToBeUpdated, cycle back at step 1.
func (r *PhasedRolloutReconciler) rollout(ctx context.Context, sts *appsv1.StatefulSet, phasedRollout *stsplusv1alpha1.PhasedRollout) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	log.V(10).Info("there is an ongoing rollout")

	// update phasedRollout status to "rolling"
	if phasedRollout.Status.Status != stsplusv1alpha1.PhasedRollotRolling {
		log.Info("setting phasedRollout state", "state", stsplusv1alpha1.PhasedRollotRolling)
		r.Recorder.Eventf(phasedRollout, "Normal", "RolloutStarted", "the phased rollout is starting")
		phasedRollout.Status.Status = stsplusv1alpha1.PhasedRollotRolling
		phasedRollout.Status.Message = "phased rollout started"
		phasedRollout.Status.UpdateRevision = sts.Status.UpdateRevision
		phasedRollout.Status.RolloutStartTime = time.Now().Format(time.RFC3339)
		phasedRollout.Status.RolloutEndTime = ""
		phasedRollout.Status.RollingPodStatus = nil
		return ctrl.Result{}, r.Status().Update(ctx, phasedRollout)
	}

	// check if the revision has changed during the rollout, if so the phased rollout must restart the rolling update process from the beginning because there is a new revision to deploy
	if phasedRollout.Status.UpdateRevision != sts.Status.UpdateRevision {
		log.Info("sts updateRevision changed during the rollout, must restart the phased rollout", "stsName", sts.Name)
		mustReconcile, err := r.preventUncontrolledRollouts(ctx, sts)
		if err != nil {
			return ctrl.Result{}, err
		}
		if mustReconcile {
			return ctrl.Result{}, nil
		}
		phasedRollout.Status.UpdateRevision = sts.Status.UpdateRevision
		phasedRollout.Status.RolloutStartTime = time.Now().Format(time.RFC3339)
		phasedRollout.Status.RolloutEndTime = ""
		phasedRollout.Status.RollingPodStatus = nil
		return ctrl.Result{}, r.Status().Update(ctx, phasedRollout)
	}

	// if partition == 0 there is nothing to do, basically we wait for the status of the phased rollout to become "updated"
	if sts.Spec.UpdateStrategy.RollingUpdate == nil || sts.Spec.UpdateStrategy.RollingUpdate.Partition == nil || *sts.Spec.UpdateStrategy.RollingUpdate.Partition == 0 {
		log.V(10).Info("sts partition is now 0, the phased rollot is over", "stsName", sts.Name)
		return ctrl.Result{RequeueAfter: time.Duration(r.RetryWaitSeconds) * time.Second}, nil
	}

	// if there is no reported status for the current pod rolling or if there was a change in sts partition, update the status: we now must wait for the next pod to be rolled
	if phasedRollout.Status.RollingPodStatus == nil || phasedRollout.Status.RollingPodStatus.Partition != *sts.Spec.UpdateStrategy.RollingUpdate.Partition {
		phasedRollout.Status.RollingPodStatus = &stsplusv1alpha1.RollingPodStatus{
			Status:    stsplusv1alpha1.RollingPodWaitForPodToBeUpdated,
			Partition: *sts.Spec.UpdateStrategy.RollingUpdate.Partition,
		}
		log.Info("sts partition has changed, update phased rollout status", "stsName", sts.Name, "rollingPodStatus", stsplusv1alpha1.RollingPodWaitForPodToBeUpdated)
		return ctrl.Result{}, r.Status().Update(ctx, phasedRollout)
	}

	// if status is RollingPodWaitForPodToBeUpdated, wait for the pod to the right of the partition to be updated
	if phasedRollout.Status.RollingPodStatus.Status == stsplusv1alpha1.RollingPodWaitForPodToBeUpdated {
		if sts.Spec.Replicas == nil {
			var one int32 = 1
			sts.Spec.Replicas = &one
		}
		if phasedRollout.Status.RollingPodStatus.Partition >= *sts.Spec.Replicas {
			// no pods to the right of the partition, set status to the next step
			phasedRollout.Status.RollingPodStatus.Status = stsplusv1alpha1.RollingPodWaitForAllPodsToBeAvailable
			phasedRollout.Status.RollingPodStatus.AnalisysStartTime = ""
			phasedRollout.Status.RollingPodStatus.LastCheckTime = ""
			phasedRollout.Status.RollingPodStatus.ConsecutiveSuccessfulChecks = 0
			phasedRollout.Status.RollingPodStatus.ConsecutiveFailedChecks = 0
			phasedRollout.Status.RollingPodStatus.TotalFailedChecks = 0
			return ctrl.Result{}, r.Status().Update(ctx, phasedRollout)
		}

		// get pod to the right of the partition
		podName := sts.Name + "-" + strconv.Itoa(int(phasedRollout.Status.RollingPodStatus.Partition))
		var pod corev1.Pod
		podNamespacedName := client.ObjectKey{
			Namespace: sts.Namespace,
			Name:      podName,
		}
		if err := r.Get(ctx, podNamespacedName, &pod); err != nil {
			if apierrs.IsNotFound(err) {
				log.V(10).Info("pod no found, will retry after a backoff", "pod", podName)
				return ctrl.Result{RequeueAfter: time.Duration(r.RetryWaitSeconds) * time.Second}, nil
			}
			log.Error(err, "unable to get pod", "pod", podName)
			return ctrl.Result{}, err
		}
		revision, ok := pod.Labels["controller-revision-hash"]
		if !ok {
			log.V(10).Info("controller-revision-hash label not found for pod, will retry after a backoff", "pod", podName)
			return ctrl.Result{RequeueAfter: time.Duration(r.RetryWaitSeconds) * time.Second}, nil
		}
		if revision != sts.Status.UpdateRevision {
			log.V(10).Info("pod is not updated to sts UpdateRevision, will retry after a backoff", "pod", podName)
			return ctrl.Result{RequeueAfter: time.Duration(r.RetryWaitSeconds) * time.Second}, nil
		}
		log.Info("pod is updated to sts UpdateRevision, setting RollingPodStatus for next step", "pod", podName, "rollingPodStatus", stsplusv1alpha1.RollingPodWaitForAllPodsToBeAvailable)
		phasedRollout.Status.RollingPodStatus.Status = stsplusv1alpha1.RollingPodWaitForAllPodsToBeAvailable
		phasedRollout.Status.RollingPodStatus.AnalisysStartTime = ""
		phasedRollout.Status.RollingPodStatus.LastCheckTime = ""
		phasedRollout.Status.RollingPodStatus.ConsecutiveSuccessfulChecks = 0
		phasedRollout.Status.RollingPodStatus.ConsecutiveFailedChecks = 0
		phasedRollout.Status.RollingPodStatus.TotalFailedChecks = 0
		return ctrl.Result{}, r.Status().Update(ctx, phasedRollout)
	}

	// if status is RollingPodWaitForAllPodsToBeAvailable wait for all pods to be available
	if phasedRollout.Status.RollingPodStatus.Status == stsplusv1alpha1.RollingPodWaitForAllPodsToBeAvailable {
		if sts.Spec.Replicas == nil {
			var one int32 = 1
			sts.Spec.Replicas = &one
		}
		if sts.Status.AvailableReplicas != *sts.Spec.Replicas {
			log.V(10).Info("some pods in sts are not available, will retry after a backoff", "stsName", sts.Name)
			return ctrl.Result{RequeueAfter: time.Duration(r.RetryWaitSeconds) * time.Second}, nil // TODO can we avoid the requeue and just watch the status? is this delayed requeue needed at all since the status of the sts will change and we will see a reconciliation?
		}
		log.Info("all pods available for the sts, setting RollingPodStatus for next step", "stsName", sts.Name, "RollingPodStatus", stsplusv1alpha1.RollingPodWaitForInitialDelay)
		phasedRollout.Status.RollingPodStatus.Status = stsplusv1alpha1.RollingPodWaitForInitialDelay
		phasedRollout.Status.RollingPodStatus.AnalisysStartTime = time.Now().Format(time.RFC3339)
		phasedRollout.Status.RollingPodStatus.LastCheckTime = ""
		phasedRollout.Status.RollingPodStatus.ConsecutiveSuccessfulChecks = 0
		phasedRollout.Status.RollingPodStatus.ConsecutiveFailedChecks = 0
		phasedRollout.Status.RollingPodStatus.TotalFailedChecks = 0
		return ctrl.Result{}, r.Status().Update(ctx, phasedRollout)
	}

	// if status is RollingPodWaitForInitialDelay wait initialDelay before starting checks
	if phasedRollout.Status.RollingPodStatus.Status == stsplusv1alpha1.RollingPodWaitForInitialDelay {
		analisysStartTime, err := time.Parse(time.RFC3339, phasedRollout.Status.RollingPodStatus.AnalisysStartTime)
		if err != nil {
			log.Error(err, "unable to parse phasedRollout.Status.RollingPodStatus.AnalisysStartTime")
			// go back to a good status
			phasedRollout.Status.RollingPodStatus.Status = stsplusv1alpha1.RollingPodWaitForAllPodsToBeAvailable
			return ctrl.Result{RequeueAfter: time.Duration(r.RetryWaitSeconds) * time.Second}, r.Status().Update(ctx, phasedRollout)
		}
		initialDelayEndTime := analisysStartTime.Add(time.Second * time.Duration(phasedRollout.Spec.Check.InitialDelaySeconds))
		if time.Now().After(initialDelayEndTime) {
			log.Info("we are past the initial delay to roll a pod, setting RollingPodStatus for next step", "stsName", sts.Name, "RollingPodStatus", stsplusv1alpha1.RollingPodWaitForChecks)
			phasedRollout.Status.RollingPodStatus.Status = stsplusv1alpha1.RollingPodWaitForChecks
			phasedRollout.Status.RollingPodStatus.LastCheckTime = ""
			phasedRollout.Status.RollingPodStatus.ConsecutiveSuccessfulChecks = 0
			phasedRollout.Status.RollingPodStatus.ConsecutiveFailedChecks = 0
			phasedRollout.Status.RollingPodStatus.TotalFailedChecks = 0
			return ctrl.Result{}, r.Status().Update(ctx, phasedRollout)
		}
		log.V(10).Info("initial delay before rolling next pod is not completed, will retry after the delay is past", "stsName", sts.Name)
		return ctrl.Result{RequeueAfter: time.Until(initialDelayEndTime)}, nil
	}

	// if status is RollingPodWaitForChecks wait for checks to be ok
	if phasedRollout.Status.RollingPodStatus.Status == stsplusv1alpha1.RollingPodWaitForChecks ||
		phasedRollout.Status.RollingPodStatus.Status == stsplusv1alpha1.RollingPodPrometheusError {

		if phasedRollout.Status.RollingPodStatus.ConsecutiveSuccessfulChecks >= phasedRollout.Spec.Check.SuccessThreshold {
			// all checks passed, decrease partition
			if sts.Spec.UpdateStrategy.RollingUpdate == nil || sts.Spec.UpdateStrategy.RollingUpdate.Partition == nil || *sts.Spec.UpdateStrategy.RollingUpdate.Partition == 0 {
				log.Info("sts.Spec.UpdateStrategy.RollingUpdate.Partition is not defined or 0 during a rolling update, this should never happen", "stsName", sts.Name)
				return ctrl.Result{RequeueAfter: time.Duration(r.RetryWaitSeconds) * time.Second}, nil
			}
			log.Info("all checks passed, decreasing partition", "stsName", sts.Name)
			newPartitionValue := *sts.Spec.UpdateStrategy.RollingUpdate.Partition - 1
			sts.Spec.UpdateStrategy.RollingUpdate.Partition = &newPartitionValue
			return ctrl.Result{}, r.Update(ctx, sts)

		}

		if phasedRollout.Status.RollingPodStatus.LastCheckTime != "" {
			lastCheckTime, err := time.Parse(time.RFC3339, phasedRollout.Status.RollingPodStatus.LastCheckTime)
			if err != nil {
				log.Error(err, "unable to parse phasedRollout.Status.RollingPodStatus.LastCheckTime")
				// go back to a good status
				phasedRollout.Status.RollingPodStatus.Status = stsplusv1alpha1.RollingPodWaitForAllPodsToBeAvailable
				return ctrl.Result{}, r.Status().Update(ctx, phasedRollout)
			}
			nextCheckTime := lastCheckTime.Add(time.Second * time.Duration(phasedRollout.Spec.Check.PeriodSeconds))
			if !time.Now().After(nextCheckTime) {
				log.V(10).Info("we must still wait before performing a new check")
				return ctrl.Result{RequeueAfter: time.Until(nextCheckTime)}, nil
			}
		}

		// prepare prometheus client
		username := ""
		password := ""
		token := ""
		secretRef := phasedRollout.Spec.Check.Query.SecretRef
		if secretRef != "" {
			// get secret for prometheus credentials
			var secret corev1.Secret
			podNamespacedName := client.ObjectKey{
				Namespace: sts.Namespace,
				Name:      secretRef,
			}
			if err := r.Get(ctx, podNamespacedName, &secret); err != nil {
				if apierrs.IsNotFound(err) {
					log.Info("secret no found, will retry after a backoff", "secret", secretRef)
					return ctrl.Result{RequeueAfter: time.Duration(r.RetryWaitSeconds) * time.Second}, nil
				}
				log.Error(err, "unable to get secret", "secret", secretRef)
				return ctrl.Result{}, err
			}
			usernameByte := secret.Data["username"]
			username = string(usernameByte)

			passwordByte := secret.Data["password"]
			password = string(passwordByte)

			tokenByte := secret.Data["token"]
			token = string(tokenByte)
		}
		promClient, err := prometheus.NewPrometheusClient(phasedRollout.Spec.Check.Query.URL, phasedRollout.Spec.Check.Query.InsecureSkipVerify, username, password, token)
		if err != nil {
			log.Error(err, "error setting up prometheus client")
			phasedRollout.Status.RollingPodStatus.Status = stsplusv1alpha1.RollingPodPrometheusError
			return ctrl.Result{RequeueAfter: time.Duration(r.RetryWaitSeconds) * time.Second}, r.Status().Update(ctx, phasedRollout) //TODO insteand of retrying we should watch for a change of config
		}

		// perform prometheus check
		checkResult, err := promClient.RunQuery(phasedRollout.Spec.Check.Query.Expr)
		if err != nil {
			log.Error(err, "error querying prometheus")
			phasedRollout.Status.RollingPodStatus.Status = stsplusv1alpha1.RollingPodPrometheusError
			return ctrl.Result{RequeueAfter: time.Duration(r.RetryWaitSeconds) * time.Second}, r.Status().Update(ctx, phasedRollout) //TODO instead of waiting an arbitrary time we should do retries with backoff
		}
		phasedRollout.Status.RollingPodStatus.Status = stsplusv1alpha1.RollingPodWaitForChecks
		phasedRollout.Status.RollingPodStatus.LastCheckTime = time.Now().Format(time.RFC3339)
		if checkResult {
			phasedRollout.Status.RollingPodStatus.ConsecutiveSuccessfulChecks += 1
			phasedRollout.Status.RollingPodStatus.ConsecutiveFailedChecks = 0
		} else {
			phasedRollout.Status.RollingPodStatus.ConsecutiveSuccessfulChecks = 0
			phasedRollout.Status.RollingPodStatus.ConsecutiveFailedChecks += 1
			phasedRollout.Status.RollingPodStatus.TotalFailedChecks += 1
		}
		log.Info("check performed, will requeue for next check", "wasSuccessful", checkResult, "consecutiveSuccessfulChecks", phasedRollout.Status.RollingPodStatus.ConsecutiveSuccessfulChecks, "consecutiveFailedChecks", phasedRollout.Status.RollingPodStatus.ConsecutiveFailedChecks, "totalFailedChecks", phasedRollout.Status.RollingPodStatus.TotalFailedChecks)
		return ctrl.Result{RequeueAfter: time.Duration(phasedRollout.Spec.Check.PeriodSeconds) * time.Second}, r.Status().Update(ctx, phasedRollout)

	}

	log.Info("phasedRollout.Status.Status not recognized, this should never happen", "status", phasedRollout.Status.Status)
	return ctrl.Result{}, nil
}

// manageSTS sets the necessary fields in the sts if it should be managed by this phased rollout.
// Returns:
// the first return value is true if it is ok to manage the sts;
// the second return value is true we need to do a reconciliation after an apply of a change in the sts or phasedRollout;
// the third return value is an error if any.
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
			log.Error(err, "unable to get phasedRollout")
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
