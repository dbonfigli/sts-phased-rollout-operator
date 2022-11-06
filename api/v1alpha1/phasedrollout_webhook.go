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
	"net/url"

	"github.com/prometheus/prometheus/promql/parser"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// log is for logging in this package.
var phasedrolloutlog = logf.Log.WithName("phasedrollout-resource")

func (r *PhasedRollout) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

//+kubebuilder:webhook:path=/mutate-sts-plus-v1alpha1-phasedrollout,mutating=true,failurePolicy=fail,sideEffects=None,groups=sts.plus,resources=phasedrollouts,verbs=create;update,versions=v1alpha1,name=mphasedrollout.kb.io,admissionReviewVersions={v1,v1beta1}

var _ webhook.Defaulter = &PhasedRollout{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *PhasedRollout) Default() {
	phasedrolloutlog.Info("default", "name", r.Name)

	if r.Spec.Check.InitialDelaySeconds == 0 {
		r.Spec.Check.InitialDelaySeconds = 60
	}
	if r.Spec.Check.PeriodSeconds == 0 {
		r.Spec.Check.PeriodSeconds = 60
	}
	if r.Spec.Check.MaxTries == 0 {
		r.Spec.Check.MaxTries = 10
	}
	if r.Spec.Check.SuccessThreshold == 0 {
		r.Spec.Check.SuccessThreshold = 3
	}
	if r.Spec.Check.FailureThreshold == 0 {
		r.Spec.Check.FailureThreshold = 3
	}
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:path=/validate-sts-plus-v1alpha1-phasedrollout,mutating=false,failurePolicy=fail,sideEffects=None,groups=sts.plus,resources=phasedrollouts,verbs=create;update,versions=v1alpha1,name=vphasedrollout.kb.io,admissionReviewVersions={v1,v1beta1}

var _ webhook.Validator = &PhasedRollout{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *PhasedRollout) ValidateCreate() error {
	phasedrolloutlog.Info("validate create", "name", r.Name)

	return r.validatePhasedRollout()
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *PhasedRollout) ValidateUpdate(old runtime.Object) error {
	phasedrolloutlog.Info("validate update", "name", r.Name)

	return r.validatePhasedRollout()
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *PhasedRollout) ValidateDelete() error {
	phasedrolloutlog.Info("validate delete", "name", r.Name)

	return nil
}

func (r *PhasedRollout) validatePhasedRollout() error {
	var allErrs field.ErrorList
	if err := r.validatePromQLExpr(); err != nil {
		allErrs = append(allErrs, err)
	}
	if err := r.validatePrometheusUrl(); err != nil {
		allErrs = append(allErrs, err)
	}
	if len(allErrs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(
		schema.GroupKind{Group: "sts.plus", Kind: "PhasedRollout"},
		r.Name, allErrs)
}

func (r *PhasedRollout) validatePromQLExpr() *field.Error {
	expr := r.Spec.Check.Query.Expr
	_, err := parser.ParseExpr(expr)
	if err != nil {
		return field.Invalid(field.NewPath("spec").Child("check").Child("query").Child("expr"), expr, err.Error())
	}
	return nil
}

func (r *PhasedRollout) validatePrometheusUrl() *field.Error {
	promURL := r.Spec.Check.Query.URL
	_, err := url.ParseRequestURI(promURL)
	if err != nil {
		return field.Invalid(field.NewPath("spec").Child("check").Child("query").Child("url"), promURL, err.Error())
	}
	return nil
}
