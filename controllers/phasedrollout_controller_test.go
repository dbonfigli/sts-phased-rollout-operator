/*
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
// +kubebuilder:docs-gen:collapse=Apache License

/*
Ideally, we should have one `<kind>_controller_test.go` for each controller scaffolded and called in the `suite_test.go`.
So, let's write our example test for the CronJob controller (`cronjob_controller_test.go.`)
*/

/*
As usual, we start with the necessary imports. We also define some utility variables.
*/
package controllers

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strconv"

	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	stsplusv1alpha1 "github.com/dbonfigli/sts-plus-operator/api/v1alpha1"
)

var _ = Describe("PhasedRollout controller", func() {

	const (
		phasedRollotName         = "test-phased-rollout"
		ns                       = "default"
		STSName                  = "test-sts"
		timeout                  = time.Second * 5
		duration                 = time.Second * 5
		interval                 = time.Millisecond * 250
		checkInitialDelaySeconds = 3
		checkPeriodSeconds       = 3
	)

	phasedRolloutTemplate := stsplusv1alpha1.PhasedRollout{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "sts.plus/v1alpha1",
			Kind:       "PhasedRollout",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      phasedRollotName,
			Namespace: ns,
		},
		Spec: stsplusv1alpha1.PhasedRolloutSpec{
			TargetRef: STSName,
			Check: stsplusv1alpha1.Check{
				InitialDelaySeconds: checkInitialDelaySeconds,
				PeriodSeconds:       checkPeriodSeconds,
				SuccessThreshold:    2,
				Query: stsplusv1alpha1.PrometheusQuery{
					Expr: "up{}",
					URL:  "http://localhost",
				},
			},
		},
	}
	var replicas int32 = 2
	stsTemplate := appsv1.StatefulSet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "StatefulSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      STSName,
			Namespace: ns,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:       &replicas,
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{Type: appsv1.RollingUpdateStatefulSetStrategyType},
			Selector:       &metav1.LabelSelector{MatchLabels: map[string]string{"app": "nginx"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "nginx"}},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "nginx", Image: "registry.k8s.io/nginx-slim:0.8"}}},
			},
		},
	}

	podTemplate := corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Pod",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-sts-0",
			Namespace: ns,
			Labels:    map[string]string{"app": "nginx"},
		},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "nginx", Image: "registry.k8s.io/nginx-slim:0.8"}}},
	}

	Describe("Create the PhasedRollout", func() {
		Context("When the spec does not respects the constraints", func() {
			It("Should not create the PhasedRollout", func() {

				By("Creating the PhasedRollout")
				phasedRollout := phasedRolloutTemplate
				phasedRollout.Name = randomName(phasedRollotName)
				phasedRollout.Spec.Check.InitialDelaySeconds = -1
				Expect(k8sClient.Create(context.Background(), &phasedRollout)).ShouldNot(Succeed())
			})
		})

		Context("When the spec respects the constraints", func() {
			It("Should create the PhasedRollout with status errorSTSNotFound", func() {

				By("Creating the PhasedRollout")
				phasedRollout := phasedRolloutTemplate
				phasedRollout.Name = randomName(phasedRollotName)
				Expect(k8sClient.Create(context.Background(), &phasedRollout)).Should(Succeed())
				Eventually(func() bool {
					var pr stsplusv1alpha1.PhasedRollout
					err := k8sClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: phasedRollout.Name}, &pr)
					return err == nil && pr.Status.Status == stsplusv1alpha1.PhasedRollotErrorSTSNotFound
				}, timeout, interval).Should(BeTrue())
			})
		})

		Context("When the sts has not UpdateStrategy rolling update", func() {
			It("Should create the PhasedRollout with status errorCannotManage", func() {

				By("Creating the sts")
				sts := stsTemplate
				sts.Name = randomName(STSName)
				sts.Spec.UpdateStrategy = appsv1.StatefulSetUpdateStrategy{Type: appsv1.OnDeleteStatefulSetStrategyType}
				Expect(k8sClient.Create(context.Background(), &sts)).Should(Succeed())

				By("Creating the PhasedRollout")
				phasedRollout := phasedRolloutTemplate
				phasedRollout.Name = randomName(phasedRollotName)
				phasedRollout.Spec.TargetRef = sts.Name
				Expect(k8sClient.Create(context.Background(), &phasedRollout)).Should(Succeed())
				Eventually(func() bool {
					var pr stsplusv1alpha1.PhasedRollout
					err := k8sClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: phasedRollout.Name}, &pr)
					return err == nil && pr.Status.Status == stsplusv1alpha1.PhasedRollotErrorCannotManage
				}, timeout, interval).Should(BeTrue())
			})
		})
	})

	Describe("Take ownership of a sts", func() {
		Context("When the sts is not managed by any other PhasedRollout", func() {
			It("Should take ownership of the sts", func() {

				By("Creating the sts")
				sts := stsTemplate
				sts.Name = randomName(STSName)
				Expect(k8sClient.Create(context.Background(), &sts)).Should(Succeed())

				By("Creating the phasedRollout")
				phasedRollout := phasedRolloutTemplate
				phasedRollout.Name = randomName(phasedRollotName)
				phasedRollout.Spec.TargetRef = sts.Name
				Expect(k8sClient.Create(context.Background(), &phasedRollout)).Should(Succeed())

				By("Expecting the sts to report to be managed by the phasedRollout")
				Eventually(func() bool {
					var readSTS appsv1.StatefulSet
					err := k8sClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: sts.Name}, &readSTS)
					return err == nil && readSTS.Annotations != nil && readSTS.Annotations[managedByAnnotation] == phasedRollout.Name
				}, timeout, interval).Should(BeTrue())
			})
		})

		Context("When the sts is managed by another PhasedRollout", func() {
			Context("When another PhasedRollout does not exist", func() {
				It("Should take ownership of the sts", func() {

					By("Creating the sts that reports to be managed by another PhasedRollout")
					sts := stsTemplate
					sts.Name = randomName(STSName)
					sts.Annotations = make(map[string]string)
					sts.Annotations[managedByAnnotation] = "non-existant-phased-rollout"
					Expect(k8sClient.Create(context.Background(), &sts)).Should(Succeed())

					By("Creating the phasedRollout")
					phasedRollout := phasedRolloutTemplate
					phasedRollout.Name = randomName(phasedRollotName)
					phasedRollout.Spec.TargetRef = sts.Name
					Expect(k8sClient.Create(context.Background(), &phasedRollout)).Should(Succeed())

					By("Expecting the sts to report to be managed by the phasedRollout")
					Eventually(func() bool {
						var readSTS appsv1.StatefulSet
						err := k8sClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: sts.Name}, &readSTS)
						return err == nil && readSTS.Annotations != nil && readSTS.Annotations[managedByAnnotation] == phasedRollout.Name
					}, timeout, interval).Should(BeTrue())
				})
			})

			Context("When the sts reports to be managed by another PhasedRollout that indeed exists but it does not report to have this sts as target", func() {
				It("Should take ownership of the sts", func() {

					By("Creating the sts that reports to be managed by another PhasedRollout")
					otherPhasedRolloutName := randomName(phasedRollotName)
					sts := stsTemplate
					sts.Name = randomName(STSName)
					sts.Annotations = make(map[string]string)
					sts.Annotations[managedByAnnotation] = otherPhasedRolloutName
					Expect(k8sClient.Create(context.Background(), &sts)).Should(Succeed())

					By("Creating the other phasedRollout")
					otherPhasedRollout := phasedRolloutTemplate
					otherPhasedRollout.Name = otherPhasedRolloutName
					otherPhasedRollout.Spec.TargetRef = "non-existant-sts"
					Expect(k8sClient.Create(context.Background(), &otherPhasedRollout)).Should(Succeed())

					By("Creating the phasedRollout")
					phasedRollout := phasedRolloutTemplate
					phasedRollout.Name = randomName(phasedRollotName)
					phasedRollout.Spec.TargetRef = sts.Name
					Expect(k8sClient.Create(context.Background(), &phasedRollout)).Should(Succeed())

					By("Expecting the sts to report to be managed by the phasedRollout")
					Eventually(func() bool {
						var readSTS appsv1.StatefulSet
						err := k8sClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: sts.Name}, &readSTS)
						return err == nil && readSTS.Annotations != nil && readSTS.Annotations[managedByAnnotation] == phasedRollout.Name
					}, timeout, interval).Should(BeTrue())
				})
			})

			Context("When another PhasedRollout is properly managing the sts", func() {
				It("Should not take ownership of the sts", func() {

					By("Creating the sts")
					sts := stsTemplate
					sts.Name = randomName(STSName)
					Expect(k8sClient.Create(context.Background(), &sts)).Should(Succeed())

					By("Creating the phasedRollout")
					phasedRollout := phasedRolloutTemplate
					phasedRollout.Name = randomName(phasedRollotName)
					phasedRollout.Spec.TargetRef = sts.Name
					Expect(k8sClient.Create(context.Background(), &phasedRollout)).Should(Succeed())

					By("Expecting the sts reports to be managed by the phasedRollout")
					Eventually(func() bool {
						var readSTS appsv1.StatefulSet
						err := k8sClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: sts.Name}, &readSTS)
						return err == nil && readSTS.Annotations != nil && readSTS.Annotations[managedByAnnotation] == phasedRollout.Name
					}, timeout, interval).Should(BeTrue())

					By("Creating the other phasedRollout")
					otherPhasedRollout := phasedRolloutTemplate
					otherPhasedRollout.Name = randomName(phasedRollotName)
					otherPhasedRollout.Spec.TargetRef = sts.Name
					Expect(k8sClient.Create(context.Background(), &otherPhasedRollout)).Should(Succeed())

					By("Expecting the other phasedRollout to have status \"errorCannotManage\"")
					Eventually(func() bool {
						var pr stsplusv1alpha1.PhasedRollout
						err := k8sClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: otherPhasedRollout.Name}, &pr)
						return err == nil && pr.Status.Status == stsplusv1alpha1.PhasedRollotErrorCannotManage
					}, timeout, interval).Should(BeTrue())
				})
			})
		})
	})

	Describe("Suspend the PhasedRollout", func() {
		Context("When the PhasedRollout.Spec.StandardRollingUpdate == true", func() {
			It("Should resume the standard k8s rolling update", func() {

				By("Creating the sts")
				sts := stsTemplate
				sts.Name = randomName(STSName)
				Expect(k8sClient.Create(context.Background(), &sts)).Should(Succeed())

				By("Creating the PhasedRollout")
				phasedRollout := phasedRolloutTemplate
				phasedRollout.Name = randomName(phasedRollotName)
				phasedRollout.Spec.TargetRef = sts.Name
				Expect(k8sClient.Create(context.Background(), &phasedRollout)).Should(Succeed())

				By("Expecting the sts to report to be managed by the phasedRollout with partition config to block unmanaged rolling updates")
				Eventually(func() bool {
					var readSTS appsv1.StatefulSet
					err := k8sClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: sts.Name}, &readSTS)
					return err == nil &&
						readSTS.Annotations != nil &&
						readSTS.Annotations[managedByAnnotation] == phasedRollout.Name &&
						readSTS.Spec.UpdateStrategy.RollingUpdate != nil &&
						*readSTS.Spec.UpdateStrategy.RollingUpdate.Partition == 2
				}, timeout, interval).Should(BeTrue())

				By("Setting PhasedRollout.Spec.StandardRollingUpdate == true")
				Expect(k8sClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: phasedRollout.Name}, &phasedRollout)).Should(Succeed())
				phasedRollout.Spec.StandardRollingUpdate = true
				Expect(k8sClient.Update(context.Background(), &phasedRollout)).Should(Succeed())

				By("Expecting the phasedRollout to have status \"suspended\"")
				Eventually(func() bool {
					var pr stsplusv1alpha1.PhasedRollout
					err := k8sClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: phasedRollout.Name}, &pr)
					return err == nil && pr.Status.Status == stsplusv1alpha1.PhasedRollotSuspened
				}, timeout, interval).Should(BeTrue())

				By("Expecting the sts to have UpdateStrategy.RollingUpdate.Partition == 0")
				Eventually(func() bool {
					var readSTS appsv1.StatefulSet
					err := k8sClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: sts.Name}, &readSTS)
					return err == nil && (readSTS.Spec.UpdateStrategy.RollingUpdate == nil || readSTS.Spec.UpdateStrategy.RollingUpdate.Partition == nil || *readSTS.Spec.UpdateStrategy.RollingUpdate.Partition == 0)
				}, timeout, interval).Should(BeTrue())
			})
		})
	})

	Describe("Perform phased rollout", func() {

		var fakePrometheusServer *fakePrometheusServer

		BeforeEach(func() {
			fakePrometheusServer = createFakePrometheusServer()
			phasedRolloutTemplate.Spec.Check.Query.URL = fakePrometheusServer.srv.URL
			DeferCleanup(func() {
				fakePrometheusServer.srv.Close()
			})
		})

		Context("When the sts needs to update pods (CurrentRevision != UpdateRevision)", func() {
			It("Should correctly perform a phased rollout", func() {

				By("Creating the sts")
				sts := stsTemplate
				sts.Name = randomName(STSName)
				Expect(k8sClient.Create(context.Background(), &sts)).Should(Succeed())

				By("Expecting the sts to be created")
				Eventually(func() bool {
					var readSTS appsv1.StatefulSet
					err := k8sClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: sts.Name}, &readSTS)
					return err == nil
				}, timeout, interval).Should(BeTrue())

				By("Setting the sts revision")
				sts.Status.CurrentRevision = "web-0000000000"
				sts.Status.UpdateRevision = "web-0000000000"
				sts.Status.Replicas = 2
				sts.Status.ReadyReplicas = 2
				sts.Status.AvailableReplicas = 2
				Expect(k8sClient.Status().Update(context.Background(), &sts)).Should(Succeed())

				By("Creating the sts pods and expecting them to be created")
				for i := 0; i < 2; i++ {
					pod := podTemplate
					pod.Labels["controller-revision-hash"] = "web-0000000000"
					pod.Name = sts.Name + "-" + strconv.Itoa(i)
					Expect(k8sClient.Create(context.Background(), &pod)).Should(Succeed())
					Eventually(func() bool {
						var readPod corev1.Pod
						err := k8sClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: pod.Name}, &readPod)
						return err == nil
					}, timeout, interval).Should(BeTrue())
				}

				By("Creating the PhasedRollout")
				phasedRollout := phasedRolloutTemplate
				phasedRollout.Name = randomName(phasedRollotName)
				phasedRollout.Spec.TargetRef = sts.Name
				Expect(k8sClient.Create(context.Background(), &phasedRollout)).Should(Succeed())

				By("Expecting the phasedRollout to have status \"updated\"")
				Eventually(func() bool {
					var pr stsplusv1alpha1.PhasedRollout
					err := k8sClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: phasedRollout.Name}, &pr)
					return err == nil && pr.Status.Status == stsplusv1alpha1.PhasedRollotUpdated
				}, timeout, interval).Should(BeTrue())

				By("Updating the sts revision to a new one")
				Expect(k8sClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: sts.Name}, &sts)).Should(Succeed())
				sts.Status.UpdateRevision = "web-0000000001"
				Expect(k8sClient.Status().Update(context.Background(), &sts)).Should(Succeed())

				By("Expecting the phasedRollout to have the correct status (status == \"rolling\", RollingPodStatus.Status == \"waitForInitialDelay\", RollingPodStatus.Partition == 2")
				Eventually(func() bool {
					var pr stsplusv1alpha1.PhasedRollout
					err := k8sClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: phasedRollout.Name}, &pr)
					return err == nil &&
						pr.Status.Status == stsplusv1alpha1.PhasedRollotRolling &&
						pr.Status.UpdateRevision == "web-0000000001" &&
						pr.Status.RollingPodStatus.Status == stsplusv1alpha1.RollingPodWaitForInitialDelay &&
						pr.Status.RollingPodStatus.Partition == 2
				}, timeout, interval).Should(BeTrue())

				By("after waitForInitialDelay, status should be \"waitForChecks\"")
				Eventually(func() bool {
					var pr stsplusv1alpha1.PhasedRollout
					err := k8sClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: phasedRollout.Name}, &pr)
					return err == nil &&
						pr.Status.Status == stsplusv1alpha1.PhasedRollotRolling &&
						pr.Status.UpdateRevision == "web-0000000001" &&
						pr.Status.RollingPodStatus.Status == stsplusv1alpha1.RollingPodWaitForChecks &&
						pr.Status.RollingPodStatus.Partition == 2
				}, time.Duration(checkInitialDelaySeconds+timeout)*time.Second, interval).Should(BeTrue())

				// here we are not checking for 2 consecutiveSuccessfulChecks because ConsecutiveSuccessfulChecks == 2 only briefly before moving to the next status
				By("there should be 1 consecutiveSuccessfulChecks")
				Eventually(func() bool {
					var pr stsplusv1alpha1.PhasedRollout
					err := k8sClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: phasedRollout.Name}, &pr)
					return err == nil &&
						pr.Status.Status == stsplusv1alpha1.PhasedRollotRolling &&
						pr.Status.UpdateRevision == "web-0000000001" &&
						pr.Status.RollingPodStatus.Status == stsplusv1alpha1.RollingPodWaitForChecks &&
						pr.Status.RollingPodStatus.ConsecutiveSuccessfulChecks == 1
				}, time.Duration(checkPeriodSeconds*2)*time.Second, interval).Should(BeTrue())

				By("after 2 consecutiveSuccessfulChecks status should be \"waitForPodToBeUpdated\"")
				Eventually(func() bool {
					var pr stsplusv1alpha1.PhasedRollout
					err := k8sClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: phasedRollout.Name}, &pr)
					return err == nil &&
						pr.Status.Status == stsplusv1alpha1.PhasedRollotRolling &&
						pr.Status.UpdateRevision == "web-0000000001" &&
						pr.Status.RollingPodStatus.Status == stsplusv1alpha1.RollingPodWaitForPodToBeUpdated
				}, checkPeriodSeconds*2, interval).Should(BeTrue())

				By("sts.Spec.UpdateStrategy.RollingUpdate.Partition should be decreased to 1")
				Eventually(func() bool {
					var readSTS appsv1.StatefulSet
					err := k8sClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: sts.Name}, &readSTS)
					return err == nil && readSTS.Spec.UpdateStrategy.RollingUpdate != nil && *readSTS.Spec.UpdateStrategy.RollingUpdate.Partition == 1
				}, timeout, interval).Should(BeTrue())

				By("Setting sts AvailableReplicas == 1")
				Expect(k8sClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: sts.Name}, &sts)).Should(Succeed())
				sts.Status.ReadyReplicas = 1
				sts.Status.AvailableReplicas = 1
				Expect(k8sClient.Status().Update(context.Background(), &sts)).Should(Succeed())

				By("Setting pod-1 to updated revision")
				var pod corev1.Pod
				Expect(k8sClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: sts.Name + "-1"}, &pod)).Should(Succeed())
				pod.Labels["controller-revision-hash"] = "web-0000000001"
				Expect(k8sClient.Update(context.Background(), &pod)).Should(Succeed())

				By("status should be status should be \"WaitForAllPodsToBeAvailable\"")
				Eventually(func() bool {
					var pr stsplusv1alpha1.PhasedRollout
					err := k8sClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: phasedRollout.Name}, &pr)
					return err == nil &&
						pr.Status.Status == stsplusv1alpha1.PhasedRollotRolling &&
						pr.Status.RollingPodStatus.Status == stsplusv1alpha1.RollingPodWaitForAllPodsToBeAvailable
				}, timeout, interval).Should(BeTrue())

				By("Setting sts AvailableReplicas == 2")
				Expect(k8sClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: sts.Name}, &sts)).Should(Succeed())
				sts.Status.ReadyReplicas = 2
				sts.Status.AvailableReplicas = 2
				Expect(k8sClient.Status().Update(context.Background(), &sts)).Should(Succeed())

				By("status should be \"RollingPodWaitForInitialDelay\"")
				Eventually(func() bool {
					var pr stsplusv1alpha1.PhasedRollout
					err := k8sClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: phasedRollout.Name}, &pr)
					return err == nil &&
						pr.Status.Status == stsplusv1alpha1.PhasedRollotRolling &&
						pr.Status.RollingPodStatus.Status == stsplusv1alpha1.RollingPodWaitForInitialDelay &&
						pr.Status.RollingPodStatus.Partition == 1
				}, timeout, interval).Should(BeTrue())

				By("status should be \"RollingPodWaitForChecks\"")
				Eventually(func() bool {
					var pr stsplusv1alpha1.PhasedRollout
					err := k8sClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: phasedRollout.Name}, &pr)
					return err == nil &&
						pr.Status.Status == stsplusv1alpha1.PhasedRollotRolling &&
						pr.Status.RollingPodStatus.Status == stsplusv1alpha1.RollingPodWaitForChecks
				}, (checkInitialDelaySeconds+timeout)*time.Second, interval).Should(BeTrue())

				By("on prometheus errors status should be \"prometheusError\"")
				fakePrometheusServer.shouldReturnError = true
				Eventually(func() bool {
					var pr stsplusv1alpha1.PhasedRollout
					err := k8sClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: phasedRollout.Name}, &pr)
					return err == nil &&
						pr.Status.Status == stsplusv1alpha1.PhasedRollotRolling &&
						pr.Status.RollingPodStatus.Status == stsplusv1alpha1.RollingPodPrometheusError
				}, time.Duration(checkPeriodSeconds*2)*time.Second, interval).Should(BeTrue())

				By("on prometheus with data, failed checks should increase")
				fakePrometheusServer.shouldReturnError = false
				fakePrometheusServer.shouldReturnData = true
				Eventually(func() bool {
					var pr stsplusv1alpha1.PhasedRollout
					err := k8sClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: phasedRollout.Name}, &pr)
					return err == nil &&
						pr.Status.Status == stsplusv1alpha1.PhasedRollotRolling &&
						pr.Status.RollingPodStatus.Status == stsplusv1alpha1.RollingPodWaitForChecks &&
						pr.Status.RollingPodStatus.ConsecutiveSuccessfulChecks == 0 &&
						pr.Status.RollingPodStatus.ConsecutiveFailedChecks > 0 &&
						pr.Status.RollingPodStatus.TotalFailedChecks > 0
				}, time.Duration(checkPeriodSeconds*2)*time.Second, interval).Should(BeTrue())

				By("on multiple successful checks, sts partition should decrease (to 0)")
				fakePrometheusServer.shouldReturnError = false
				fakePrometheusServer.shouldReturnData = false
				Eventually(func() bool {
					var readSTS appsv1.StatefulSet
					err := k8sClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: sts.Name}, &readSTS)
					return err == nil && readSTS.Spec.UpdateStrategy.RollingUpdate != nil && *readSTS.Spec.UpdateStrategy.RollingUpdate.Partition == 0
				}, time.Duration(checkPeriodSeconds*3)*time.Second, interval).Should(BeTrue())

			})
		})
	})
})

type fakePrometheusServer struct {
	shouldReturnData  bool
	shouldReturnError bool
	srv               *httptest.Server
}

func createFakePrometheusServer() *fakePrometheusServer {
	f := &fakePrometheusServer{
		shouldReturnData:  false,
		shouldReturnError: false,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if f.shouldReturnError {
			w.WriteHeader(500)
		} else if f.shouldReturnData {
			fmt.Fprintf(w, `
				{
					"status": "success",
					"data": {
						"resultType": "vector",
						"result": []
					}
				}`)
		} else {
			fmt.Fprintf(w, `
				{
					"status": "success",
					"data": {
						"resultType": "vector",
						"result": [
							{
								"metric": {
									"__name__": "up",
									"container": "example-app",
									"endpoint": "web",
									"instance": "10.244.0.21:8080",
									"job": "example-app",
									"namespace": "default",
									"pod": "example-app-5f86f88d98-bzgv5",
									"service": "example-app"
								},
								"value": [
									1674906750.443,
									"1"
								]
							}
						]
					}
				}`)
		}
	}))
	f.srv = srv
	return f
}

func randomName(prefix string) string {
	letters := []rune("abcdefghijklmnopqrstuvwxyz")
	suffix := make([]rune, 8)
	for i := 0; i < 8; i++ {
		suffix[i] = letters[rand.Intn(len(letters))]
	}
	return prefix + "-" + string(suffix)
}
