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

// PhasedRolloutSpec defines the desired state of PhasedRollout
type PhasedRolloutSpec struct {
	// TargetRef references a target resource
	TargetRef string `json:"targetRef"`
	// Check defines the validation process of a rollout
	Check Check `json:"check"`
	// SkipCheck perform the rollout without performing checks
	// +optional
	SkipCheck bool `json:"skipCheck"`
	// Whether to rollback the rollout in case of failure of the checks
	// +optional
	Rollback bool `json:"rollback"`
}

// PhasedRolloutStatus defines the observed state of PhasedRollout
type PhasedRolloutStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
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
	//+kubebuilder:validation:Minimum=30

	// Number of seconds to wait before performing cheks after rollout step, after rolled pods are ready. Default is 60 seconds, minimum is 30.
	// +optional
	InitialDelaySeconds int `json:"initialDelaySeconds"`

	//+kubebuilder:validation:Minimum=30

	// How often (in seconds) to perform the check. Default is 60 seconds, minimum is 30.
	// +optional
	PeriodSeconds int `json:"periodSeconds"`

	//+kubebuilder:validation:Minimum=1

	// Max number of tries before the rollout step is marked as failed. Default is 10.
	// +optional
	MaxTries int `json:"maxTries"`

	//+kubebuilder:validation:Minimum=1

	// Number of consecutive success checks to consider the rollout step good. Default is 3.
	// +optional
	SuccessThreshold int `json:"successThreshold"`

	//+kubebuilder:validation:Minimum=1

	// Number of consecutive failed checks to consider the rollout step failed. Default is 3.
	// +optional
	FailureThreshold int `json:"failureThreshold"`

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

	// Secret reference containing the prometheus credentials
	// +optional
	SecretRef string `json:"secretRef,omitempty"`
}
