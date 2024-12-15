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
	"context"
	"fmt"
	"net/url"

	stsplusv1alpha1 "github.com/dbonfigli/sts-phased-rollout-operator/api/v1alpha1"
	"github.com/prometheus/prometheus/promql/parser"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// nolint:unused
// log is for logging in this package.
var phasedrolloutlog = logf.Log.WithName("phasedrollout-resource")

// SetupPhasedRolloutWebhookWithManager registers the webhook for PhasedRollout in the manager.
func SetupPhasedRolloutWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&stsplusv1alpha1.PhasedRollout{}).
		WithValidator(&PhasedRolloutCustomValidator{}).
		WithDefaulter(&PhasedRolloutCustomDefaulter{}).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-sts-plus-v1alpha1-phasedrollout,mutating=true,failurePolicy=ignore,sideEffects=None,groups=sts.plus,resources=phasedrollouts,verbs=create;update,versions=v1alpha1,name=mphasedrollout-v1alpha1.kb.io,admissionReviewVersions=v1

// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type PhasedRolloutCustomDefaulter struct{}

var _ webhook.CustomDefaulter = &PhasedRolloutCustomDefaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind PhasedRollout.
func (d *PhasedRolloutCustomDefaulter) Default(ctx context.Context, obj runtime.Object) error {
	phasedrollout, ok := obj.(*stsplusv1alpha1.PhasedRollout)

	if !ok {
		return fmt.Errorf("expected an PhasedRollout object but got %T", obj)
	}
	phasedrolloutlog.Info("Defaulting for PhasedRollout", "name", phasedrollout.GetName())

	if phasedrollout.Spec.Check.InitialDelaySeconds == 0 {
		phasedrollout.Spec.Check.InitialDelaySeconds = 60
	}
	if phasedrollout.Spec.Check.PeriodSeconds == 0 {
		phasedrollout.Spec.Check.PeriodSeconds = 60
	}
	if phasedrollout.Spec.Check.SuccessThreshold == 0 {
		phasedrollout.Spec.Check.SuccessThreshold = 3
	}

	return nil
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-sts-plus-v1alpha1-phasedrollout,mutating=false,failurePolicy=ignore,sideEffects=None,groups=sts.plus,resources=phasedrollouts,verbs=create;update,versions=v1alpha1,name=vphasedrollout-v1alpha1.kb.io,admissionReviewVersions=v1

// PhasedRolloutCustomValidator struct is responsible for validating the PhasedRollout resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type PhasedRolloutCustomValidator struct{}

var _ webhook.CustomValidator = &PhasedRolloutCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type PhasedRollout.
func (v *PhasedRolloutCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	phasedrollout, ok := obj.(*stsplusv1alpha1.PhasedRollout)
	if !ok {
		return nil, fmt.Errorf("expected a PhasedRollout object but got %T", obj)
	}
	phasedrolloutlog.Info("Validation for PhasedRollout upon creation", "name", phasedrollout.GetName())

	return nil, validatePhasedRollout(phasedrollout)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type PhasedRollout.
func (v *PhasedRolloutCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	phasedrollout, ok := newObj.(*stsplusv1alpha1.PhasedRollout)
	if !ok {
		return nil, fmt.Errorf("expected a PhasedRollout object for the newObj but got %T", newObj)
	}
	phasedrolloutlog.Info("Validation for PhasedRollout upon update", "name", phasedrollout.GetName())

	// TODO(user): fill in your validation logic upon object update.

	return nil, validatePhasedRollout(phasedrollout)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type PhasedRollout.
func (v *PhasedRolloutCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	phasedrollout, ok := obj.(*stsplusv1alpha1.PhasedRollout)
	if !ok {
		return nil, fmt.Errorf("expected a PhasedRollout object but got %T", obj)
	}
	phasedrolloutlog.Info("Validation for PhasedRollout upon deletion", "name", phasedrollout.GetName())

	return nil, nil
}

func validatePhasedRollout(r *stsplusv1alpha1.PhasedRollout) error {
	var allErrs field.ErrorList
	if err := validatePromQLExpr(r); err != nil {
		allErrs = append(allErrs, err)
	}
	if err := validatePrometheusUrl(r); err != nil {
		allErrs = append(allErrs, err)
	}
	if len(allErrs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(
		schema.GroupKind{Group: "sts.plus", Kind: "PhasedRollout"},
		r.Name, allErrs)
}

func validatePromQLExpr(r *stsplusv1alpha1.PhasedRollout) *field.Error {
	expr := r.Spec.Check.Query.Expr
	_, err := parser.ParseExpr(expr)
	if err != nil {
		return field.Invalid(field.NewPath("spec").Child("check").Child("query").Child("expr"), expr, "error parsing promQL expr: "+err.Error())
	}
	return nil
}

func validatePrometheusUrl(r *stsplusv1alpha1.PhasedRollout) *field.Error {
	promURL := r.Spec.Check.Query.URL
	_, err := url.ParseRequestURI(promURL)
	if err != nil {
		return field.Invalid(field.NewPath("spec").Child("check").Child("query").Child("url"), promURL, err.Error())
	}
	return nil
}
