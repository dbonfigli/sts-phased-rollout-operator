/*
Copyright 2024.

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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	PhasedRolloutConditionReady   = "Ready"
	PhasedRolloutConditionUpdated = "Updated"

	PhasedRolloutErrorCannotManage = "ErrorCannotManage"
	PhasedRolloutErrorSTSNotFound  = "ErrorSTSNotFound"
	PhasedRolloutSuspened          = "Suspended"
	PhasedRolloutReady             = "Ready"

	PhasedRolloutRolling = "Rolling"
	PhasedRolloutUpdated = "Updated"

	RollingPodWaitForPodToBeUpdated       = "WaitForPodToBeUpdated"
	RollingPodWaitForAllPodsToBeAvailable = "WaitForAllPodsToBeAvailable"
	RollingPodWaitForInitialDelay         = "WaitForInitialDelay"
	RollingPodWaitForChecks               = "WaitForChecks"
	RollingPodPrometheusError             = "PrometheusError"
)

// PhasedRolloutSpec defines the desired state of PhasedRollout.
type PhasedRolloutSpec struct {
	// TargetRef references a target resource, i.e. the name of the statefulset this PhasedRollout should manage.
	TargetRef string `json:"targetRef"`
	// Check defines the validation process of a rollout.
	Check Check `json:"check"`
	// StandardRollingUpdate, if true, stops the phased rollout mechanism and resume the standard RollingUpdate strategy.
	// Default is false.
	// +optional
	StandardRollingUpdate bool `json:"standardRollingUpdate"`
}

// PhasedRolloutStatus defines the observed state of PhasedRollout.
type PhasedRolloutStatus struct {
	// List of status conditions to indicate the status this PhasedRollout. Known condition types are `Ready` and `Updated`.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// Phase is a simple, high-level summary of the status of the PhasedRollout. The conditions array contains more detail about the PhasedRollout status.
	Phase string `json:"phase,omitempty"`
	// UpdateRevision is the revision of the sts as seen in the `sts.status.updateRevision` field.
	UpdateRevision string `json:"updateRevision,omitempty"`
	// RolloutStartTime is the time when the latest rollout started.
	RolloutStartTime metav1.Time `json:"rolloutStartTime,omitempty"`
	// RolloutStartTime is the time when the latest rollout ended.
	RolloutEndTime metav1.Time `json:"rolloutEndTime,omitempty"`
	// RollingPodStatus contains information regarding the rollout of a pod.
	RollingPodStatus *RollingPodStatus `json:"rollingPodStatus,omitempty"`
}

type RollingPodStatus struct {
	// Status contains a brief description of the phase where the ongoing rollout is during the update process for a pod.
	Status string `json:"status,omitempty"`
	// Partition is the last seen `sts.spec.updateStrategy.rollingUpdate.partition` value.
	Partition int32 `json:"partition"`
	// AnalisysStartTime is the time when the analysis started before updating a new pod.
	AnalisysStartTime metav1.Time `json:"analisysStartTime,omitempty"`
	// LastCheckTime is the time when the last check was performed before updating a new pod.
	LastCheckTime metav1.Time `json:"lastCheckTime,omitempty"`
	// ConsecutiveSuccessfulChecks is the number of consecutive successful checks performed up until now during the analysis before rolling the next pod.
	ConsecutiveSuccessfulChecks int32 `json:"consecutiveSuccessfulChecks"`
	// ConsecutiveFailedChecks is the number of consecutive failed checks performed up until now during the analysis before rolling the next pod.
	ConsecutiveFailedChecks int32 `json:"consecutiveFailedChecks"`
	// TotalFailedChecks is the total number of failed checks performed up until now during the analysis before rolling the next pod.
	TotalFailedChecks int32 `json:"totalFailedChecks"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="target-ref",type="string",JSONPath=".spec.targetRef",description="target statefulset name"
//+kubebuilder:printcolumn:name="phase",type="string",JSONPath=".status.phase"
//+kubebuilder:printcolumn:name="partition",type="string",JSONPath=".status.rollingPodStatus.partition"
//+kubebuilder:printcolumn:name="rolling-pod-status",type="string",JSONPath=".status.rollingPodStatus.status"
//+kubebuilder:printcolumn:name="rollout-start-time",type="date",JSONPath=".status.rolloutStartTime"
//+kubebuilder:printcolumn:name="rollout-end-time",type="date",JSONPath=".status.rolloutEndTime"
//+kubebuilder:printcolumn:name="age",type="date",JSONPath=".metadata.creationTimestamp"

// PhasedRollout is the Schema for the PhasedRollouts API.
type PhasedRollout struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PhasedRolloutSpec   `json:"spec,omitempty"`
	Status PhasedRolloutStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// PhasedRolloutList contains a list of PhasedRollout.
type PhasedRolloutList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PhasedRollout `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PhasedRollout{}, &PhasedRolloutList{})
}

// Check is used to describe how the check should be done.
type Check struct {
	//+kubebuilder:validation:Minimum=0

	// InitialDelaySeconds is the number of seconds to wait before performing checks after a rollout step, after all pods in the sts are available. This is useful to set in order to wait for prometheus metrics to settle down. Default is 60 seconds, minimum is 0.
	// +optional
	InitialDelaySeconds int32 `json:"initialDelaySeconds"`

	//+kubebuilder:validation:Minimum=0

	// PeriodSeconds defines how often to perform the checks. Default is 60 seconds, minimum is 0.
	// +optional
	PeriodSeconds int32 `json:"periodSeconds"`

	//+kubebuilder:validation:Minimum=1

	// SuccessThreshold is the number of consecutive successful checks that must be reached before letting the rollout proceed. Default is 3, minimum is 1.
	// +optional
	SuccessThreshold int32 `json:"successThreshold"`

	// Query contains the details to perform prometheus queries.
	Query PrometheusQuery `json:"query"`
}

// PrometheusQuery describes how to perform the prometheus query.
type PrometheusQuery struct {

	//+kubebuilder:validation:MinLength=1

	// Prometheus expression for the check. The semantic similar to a prometheus alert, if data is returned then the check is considered successful, if no data is returned the check is considered failed.
	Expr string `json:"expr"`

	//+kubebuilder:validation:MinLength=1

	// URL of prometheus endpoint.
	URL string `json:"url"`

	// InsecureSkipVerify, if true, will skip tls validation.
	// +optional
	InsecureSkipVerify bool `json:"insecureSkipVerify"`

	// SecretRef is the name of a secret in the same namespace of this PhasedRollout containing the prometheus credentials for basic authentication or bearer token authentication.
	// The keys in the secret can optionally be `username` and `password` (to use for basic authentication) or `token` (for bearer token authentication)
	// +optional
	SecretRef string `json:"secretRef,omitempty"`
}

// GetCondition returns the status condition of defined type or creates one if it does not exist.
// The returned condition is the actualy entry in the PhasedRollout, not a copy.
func (p *PhasedRollout) GetCondition(conditionType string) *metav1.Condition {
	for i := range p.Status.Conditions {
		if p.Status.Conditions[i].Type == conditionType {
			return &p.Status.Conditions[i]
		}
	}
	if p.Status.Conditions == nil {
		p.Status.Conditions = make([]metav1.Condition, 0)
	}
	p.Status.Conditions = append(p.Status.Conditions, metav1.Condition{
		Type:   conditionType,
		Status: metav1.ConditionUnknown,
	})
	return &p.Status.Conditions[len(p.Status.Conditions)-1]
}

// SetCondition sets the status condition of defined type or creates one if it does not exist.
func (p *PhasedRollout) SetCondition(conditionType string, status metav1.ConditionStatus, reason, message string) {
	var c *metav1.Condition
	if p.Status.Conditions == nil {
		p.Status.Conditions = make([]metav1.Condition, 0)
	}
	for i := range p.Status.Conditions {
		if p.Status.Conditions[i].Type == conditionType {
			c = &p.Status.Conditions[i]
		}
	}
	if c == nil {
		p.Status.Conditions = append(p.Status.Conditions, metav1.Condition{Type: conditionType})
		c = &p.Status.Conditions[len(p.Status.Conditions)-1]
	}
	if c.Status != status {
		c.LastTransitionTime = metav1.Now()
	}
	c.Status = status
	c.Reason = reason
	c.Message = message
	c.ObservedGeneration = p.GetGeneration()
}
