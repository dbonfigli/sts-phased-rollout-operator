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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	stsplusv1alpha1 "github.com/dbonfigli/sts-phased-rollout-operator/api/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("PhasedRollout Webhook", func() {
	var (
		obj       *stsplusv1alpha1.PhasedRollout
		oldObj    *stsplusv1alpha1.PhasedRollout
		validator PhasedRolloutCustomValidator
		defaulter PhasedRolloutCustomDefaulter
	)

	BeforeEach(func() {
		obj = &stsplusv1alpha1.PhasedRollout{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "sts.plus/v1alpha1",
				Kind:       "PhasedRollout",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "o1",
				Namespace: "ns",
			},
			Spec: stsplusv1alpha1.PhasedRolloutSpec{
				Check: stsplusv1alpha1.Check{
					Query: stsplusv1alpha1.PrometheusQuery{
						Expr: "up",
						URL:  "http://prometheus",
					},
				},
			}}
		oldObj = &stsplusv1alpha1.PhasedRollout{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "sts.plus/v1alpha1",
				Kind:       "PhasedRollout",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "o1",
				Namespace: "ns",
			},
			Spec: stsplusv1alpha1.PhasedRolloutSpec{
				Check: stsplusv1alpha1.Check{
					Query: stsplusv1alpha1.PrometheusQuery{
						Expr: "up",
						URL:  "http://prometheus",
					},
				},
			}}
		validator = PhasedRolloutCustomValidator{}
		Expect(validator).NotTo(BeNil(), "Expected validator to be initialized")
		defaulter = PhasedRolloutCustomDefaulter{}
		Expect(defaulter).NotTo(BeNil(), "Expected defaulter to be initialized")
		Expect(oldObj).NotTo(BeNil(), "Expected oldObj to be initialized")
		Expect(obj).NotTo(BeNil(), "Expected obj to be initialized")
	})

	AfterEach(func() {
	})

	Context("When creating PhasedRollout under Defaulting Webhook", func() {
		It("Should apply defaults when a required field is empty", func() {
			By("not passing default values")
			obj.Spec.Check.InitialDelaySeconds = 0
			obj.Spec.Check.PeriodSeconds = 0
			obj.Spec.Check.SuccessThreshold = 0
			By("calling the Default method to apply defaults")
			defaulter.Default(ctx, obj)
			By("checking that the default values are set")
			Expect(obj.Spec.Check.InitialDelaySeconds).To(Equal(int32(60)))
			Expect(obj.Spec.Check.PeriodSeconds).To(Equal(int32(60)))
			Expect(obj.Spec.Check.SuccessThreshold).To(Equal(int32(3)))
		})
	})

	Context("When creating or updating PhasedRollout under Validating Webhook", func() {
		It("Should deny creation if promql expression is not valid", func() {
			By("simulating an invalid creation scenario")
			obj.Spec.Check.Query.Expr = "invalid_promql{"
			_, error := validator.ValidateCreate(ctx, obj)
			Expect(error).ToNot(BeNil())
		})

		It("Should allow creation if promql expression is valid", func() {
			By("simulating an invalid creation scenario")
			obj.Spec.Check.Query.Expr = "http_requests_total{job=~\".*server\"}"
			_, error := validator.ValidateCreate(ctx, obj)
			Expect(error).To(BeNil())
		})

		It("Should deny update if url is invalid", func() {
			By("simulating an invalid update scenario")
			obj.Spec.Check.Query.URL = "http://\\"
			_, error := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(error).ToNot(BeNil())
		})

		It("Should allow update if url is valid", func() {
			By("simulating an invalid update scenario")
			obj.Spec.Check.Query.URL = "http://sevice:456"
			_, error := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(error).To(BeNil())
		})
	})

})
