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
	PhasedRollotErrorCannotManage = "errorCannotManage"
	PhasedRollotErrorSTSNotFound  = "errorSTSNotFound"
	PhasedRollotRolling           = "rolling"
	PhasedRollotUpdated           = "updated"
	PhasedRollotSuspened          = "suspended"

	RollingPodWaitForPodToBeUpdated       = "waitForPodToBeUpdated"
	RollingPodWaitForAllPodsToBeAvailable = "WaitForAllPodsToBeAvailable"
	RollingPodWaitForInitialDelay         = "waitForInitialDelay"
	RollingPodWaitForChecks               = "waitForChecks"
	RollingPodPrometheusError             = "prometheusError"
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
	Status           string            `json:"status,omitempty"` // error / rolling / updated
	Message          string            `json:"message,omitempty"`
	UpdateRevision   string            `json:"updateRevision,omitempty"`
	RolloutStartTime string            `json:"rolloutStartTime,omitempty"`
	RolloutEndTime   string            `json:"rolloutEndTime,omitempty"`
	RollingPodStatus *RollingPodStatus `json:"rollingPodStatus,omitempty"`
}

type RollingPodStatus struct {
	Status                      string `json:"status,omitempty"` // waitToBeRolled / waitToBeReady / initialDelay / rolling
	Partition                   int32  `json:"partition"`
	AnalisysStartTime           string `json:"analisysStartTime,omitempty"`
	LastCheckTime               string `json:"lastCheckTime,omitempty"`
	ConsecutiveSuccessfulChecks int32  `json:"consecutiveSuccessfulChecks"`
	ConsecutiveFailedChecks     int32  `json:"consecutiveFailedChecks"`
	TotalFailedChecks           int32  `json:"totalFailedChecks"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

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

	// Details on the prmetheus query to perform as check
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
