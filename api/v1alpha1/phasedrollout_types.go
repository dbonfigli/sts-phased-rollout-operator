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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ConditionStatusUnknown = "Unknown"
	ConditionStatusTrue    = "True"
	ConditionStatusFalse   = "False"

	PhasedRolloutConditionReady = "Ready"

	PhasedRolloutErrorCannotManage = "ErrorCannotManage"
	PhasedRolloutErrorSTSNotFound  = "ErrorSTSNotFound"
	PhasedRolloutRolling           = "Rolling"
	PhasedRolloutUpdated           = "Updated"
	PhasedRolloutSuspened          = "Suspended"

	RollingPodWaitForPodToBeUpdated       = "WaitForPodToBeUpdated"
	RollingPodWaitForAllPodsToBeAvailable = "WaitForAllPodsToBeAvailable"
	RollingPodWaitForInitialDelay         = "WaitForInitialDelay"
	RollingPodWaitForChecks               = "WaitForChecks"
	RollingPodPrometheusError             = "PrometheusError"
)

// PhasedRolloutSpec defines the desired state of PhasedRollout
type PhasedRolloutSpec struct {
	// TargetRef references a target resource, i.e. the name of the statefulset this PhasedRollout should manage
	TargetRef string `json:"targetRef"`
	// Check defines the validation process of a rollout
	Check Check `json:"check"`
	// StandardRollingUpdate stops the phased rollout mechanism and resume the standard RollingUpdate strategy
	// +optional
	StandardRollingUpdate bool `json:"standardRollingUpdate"`
}

// PhasedRolloutStatus defines the observed state of PhasedRollout
type PhasedRolloutStatus struct {
	Conditions       []metav1.Condition `json:"conditions,omitempty"`
	UpdateRevision   string             `json:"updateRevision,omitempty"`
	RolloutStartTime string             `json:"rolloutStartTime,omitempty"`
	RolloutEndTime   string             `json:"rolloutEndTime,omitempty"`
	RollingPodStatus *RollingPodStatus  `json:"rollingPodStatus,omitempty"`
}

type RollingPodStatus struct {
	Status                      string `json:"status,omitempty"`
	Partition                   int32  `json:"partition"`
	AnalisysStartTime           string `json:"analisysStartTime,omitempty"`
	LastCheckTime               string `json:"lastCheckTime,omitempty"`
	ConsecutiveSuccessfulChecks int32  `json:"consecutiveSuccessfulChecks"`
	ConsecutiveFailedChecks     int32  `json:"consecutiveFailedChecks"`
	TotalFailedChecks           int32  `json:"totalFailedChecks"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="target-ref",type="string",JSONPath=".spec.targetRef",description="target statefulset name"
//+kubebuilder:printcolumn:name="ready",type="string",JSONPath=".status.conditions[?(@.type == 'Ready')].status"
//+kubebuilder:printcolumn:name="reason",type="string",JSONPath=".status.conditions[?(@.type == 'Ready')].reason"
//+kubebuilder:printcolumn:name="message",type="string",JSONPath=".status.conditions[?(@.type == 'Ready')].message"
//+kubebuilder:printcolumn:name="rollout-start-time",type="date",JSONPath=".status.rolloutStartTime"
//+kubebuilder:printcolumn:name="rollout-end-time",type="date",JSONPath=".status.rolloutEndTime"
//+kubebuilder:printcolumn:name="age",type="date",JSONPath=".metadata.creationTimestamp"

// PhasedRollout is the Schema for the phasedrollouts API
type PhasedRollout struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PhasedRolloutSpec   `json:"spec,omitempty"`
	Status PhasedRolloutStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// PhasedRolloutList contains a list of PhasedRollout
type PhasedRolloutList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PhasedRollout `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PhasedRollout{}, &PhasedRolloutList{})
}

// Check is used to describe how the check should be done
type Check struct {
	//+kubebuilder:validation:Minimum=0

	// Number of seconds to wait before performing cheks after rollout step, after rolled pods are available. This is usefult to set to wait for metrics to settle down. Default is 60 seconds, minimum is 0.
	// +optional
	InitialDelaySeconds int32 `json:"initialDelaySeconds"`

	//+kubebuilder:validation:Minimum=0

	// How often (in seconds) to perform the check. Default is 60 seconds, minimum is 0.
	// +optional
	PeriodSeconds int32 `json:"periodSeconds"`

	//+kubebuilder:validation:Minimum=1

	// Number of consecutive success checks to consider the rollout step good. Default is 3, minimum is 1.
	// +optional
	SuccessThreshold int32 `json:"successThreshold"`

	// Details on the prmetheus query to perform as check, semantic similar to a prometheus alert: no data means success, if the query returns data it means failure
	Query PrometheusQuery `json:"query"`
}

// PrometheusQuery describes how to perform the prometheus query
type PrometheusQuery struct {

	//+kubebuilder:validation:MinLength=1

	// Prometheus expression for the check
	Expr string `json:"expr"`

	//+kubebuilder:validation:MinLength=1

	// URL of prometheus endpoint
	URL string `json:"url"`

	// true will skip tls checks
	// +optional
	InsecureSkipVerify bool `json:"insecureSkipVerify"`

	// Secret reference containing the prometheus credentials for basic authentication or bearer token authentication.
	// The data in the secret can optionally have:
	// username: username to use for basic authentication
	// password: password to use for basic authentication
	// token: token for bearer token authentication
	// +optional
	SecretRef string `json:"secretRef,omitempty"`
}

// GetConditionReady returns the status condition of type "ready" or creates one if it does not exsist
// the returned condition is the actualy entry in the PhasedRollout, not a copy
func (p *PhasedRollout) GetConditionReady() *metav1.Condition {
	for i := range p.Status.Conditions {
		if p.Status.Conditions[i].Type == PhasedRolloutConditionReady {
			return &p.Status.Conditions[i]
		}
	}
	if p.Status.Conditions == nil {
		p.Status.Conditions = make([]metav1.Condition, 0)
	}
	p.Status.Conditions = append(p.Status.Conditions, metav1.Condition{
		Type:   PhasedRolloutConditionReady,
		Status: ConditionStatusUnknown,
	})
	return &p.Status.Conditions[len(p.Status.Conditions)-1]
}
