/*
Copyright 2025.

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

package controller

import (
	"context"
	"strings"
	"testing"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1beta1 "github.com/openstack-k8s-operators/lightspeed-operator/api/v1beta1"
)

func TestReconcileStatusCoreDeployments(t *testing.T) {
	for _, missing := range []string{"", PostgresDeploymentName, OKPDeploymentName, LCoreDeploymentName} {
		name := missing
		if name == "" {
			name = "all deployments ready"
		}
		t.Run(name, func(t *testing.T) {
			h := newTestHelper(t)
			if err := appsv1.AddToScheme(h.GetScheme()); err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			for _, deploymentName := range []string{PostgresDeploymentName, OKPDeploymentName, LCoreDeploymentName} {
				if deploymentName == missing {
					continue
				}
				deployment := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{Name: deploymentName, Namespace: "test-ns"},
					Spec:       appsv1.DeploymentSpec{Replicas: toPtr(int32(1))},
				}
				if err := h.GetClient().Create(ctx, deployment); err != nil {
					t.Fatal(err)
				}
				deployment.Status = appsv1.DeploymentStatus{
					ObservedGeneration: deployment.Generation,
					Replicas:           1, UpdatedReplicas: 1, AvailableReplicas: 1,
				}
				if err := h.GetClient().Status().Update(ctx, deployment); err != nil {
					t.Fatal(err)
				}
			}
			instance := &apiv1beta1.OpenStackLightspeed{
				ObjectMeta: metav1.ObjectMeta{Name: "test-instance", Namespace: "test-ns"},
			}
			r := &OpenStackLightspeedReconciler{Client: h.GetClient()}
			result, err := r.reconcileStatus(ctx, h, instance)
			if err != nil {
				t.Fatal(err)
			}
			wantReady := missing == ""
			if got := instance.Status.Conditions.IsTrue(apiv1beta1.OpenStackLightspeedReadyCondition); got != wantReady {
				t.Fatalf("ready = %t, want %t", got, wantReady)
			}
			if wantReady && !result.IsZero() {
				t.Fatalf("ready deployment should not requeue: %v", result)
			}
			if !wantReady && result.RequeueAfter != ResourceCreationTimeout {
				t.Fatalf("missing deployment should requeue: %v", result)
			}
		})
	}
}

var _ = ginkgo.Describe("OpenStackLightspeed Controller", func() {
	ginkgo.Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		openstacklightspeed := &apiv1beta1.OpenStackLightspeed{}

		ginkgo.BeforeEach(func() {
			ginkgo.By("creating the custom resource for the Kind OpenStackLightspeed")
			err := k8sClient.Get(ctx, typeNamespacedName, openstacklightspeed)
			if err != nil && errors.IsNotFound(err) {
				resource := &apiv1beta1.OpenStackLightspeed{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: apiv1beta1.OpenStackLightspeedSpec{
						OpenStackLightspeedCore: apiv1beta1.OpenStackLightspeedCore{
							DefaultModel: "test-model",
							Models: []apiv1beta1.OpenStackLightspeedModelSpec{{
								Name:            "test-model",
								LLMEndpoint:     "https://example.com/llm",
								LLMEndpointType: OpenAIProviderName,
								LLMCredentials:  "test-llm-credentials",
								ModelName:       "test-model",
							}},
						},
					},
				}
				gomega.Expect(k8sClient.Create(ctx, resource)).To(gomega.Succeed())
			}
		})

		ginkgo.AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &apiv1beta1.OpenStackLightspeed{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Cleanup the specific resource instance OpenStackLightspeed")
			gomega.Expect(k8sClient.Delete(ctx, resource)).To(gomega.Succeed())
		})
		ginkgo.It("should successfully reconcile the resource", func() {
			ginkgo.By("Reconciling the created resource")
			controllerReconciler := &OpenStackLightspeedReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			// TODO(user): Add more specific assertions depending on your controller's reconciliation logic.
			// Example: If you expect a certain status condition after reconciliation, verify it here.
		})
		ginkgo.It("should validate the Responses request size limit", func() {
			for _, size := range []int{-1, 0, 1, 262144} {
				resource := &apiv1beta1.OpenStackLightspeed{}
				gomega.Expect(k8sClient.Get(ctx, typeNamespacedName, resource)).To(gomega.Succeed())
				resource.Spec.LCore = &apiv1beta1.LCoreSpec{ResponsesRequestMaxSize: toPtr(size)}
				err := k8sClient.Update(ctx, resource)
				if size < 1 {
					gomega.Expect(errors.IsInvalid(err)).To(gomega.BeTrue(), "limit %d should be rejected: %v", size, err)
				} else {
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					stored := &apiv1beta1.OpenStackLightspeed{}
					gomega.Expect(k8sClient.Get(ctx, typeNamespacedName, stored)).To(gomega.Succeed())
					gomega.Expect(stored.Spec.LCore.ResponsesRequestMaxSize).To(gomega.Equal(toPtr(size)))
				}
			}
			resource := &apiv1beta1.OpenStackLightspeed{}
			gomega.Expect(k8sClient.Get(ctx, typeNamespacedName, resource)).To(gomega.Succeed())
			resource.Spec.LCore.ResponsesRequestMaxSize = nil
			gomega.Expect(k8sClient.Update(ctx, resource)).To(gomega.Succeed())
			stored := &apiv1beta1.OpenStackLightspeed{}
			gomega.Expect(k8sClient.Get(ctx, typeNamespacedName, stored)).To(gomega.Succeed())
			gomega.Expect(stored.Spec.LCore.ResponsesRequestMaxSize).To(gomega.BeNil())
		})
	})
})

func TestReconcileInstanceCount(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := apiv1beta1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add apiv1beta1 to scheme: %v", err)
	}
	first := &apiv1beta1.OpenStackLightspeed{
		ObjectMeta: metav1.ObjectMeta{Name: "first", Namespace: "ns"},
	}
	second := first.DeepCopy()
	second.Name = "second"
	otherNamespace := first.DeepCopy()
	otherNamespace.Namespace = "other-ns"
	listErr := errors.NewServiceUnavailable("list failed")

	for _, tt := range []struct {
		name      string
		instances []*apiv1beta1.OpenStackLightspeed
		request   string
		listErr   error
		wantError string
		reconcile bool
	}{
		{name: "no instances", request: first.Name},
		{name: "single instance", instances: []*apiv1beta1.OpenStackLightspeed{first}, request: first.Name, reconcile: true},
		{name: "other namespace is ignored", instances: []*apiv1beta1.OpenStackLightspeed{first, otherNamespace}, request: first.Name, reconcile: true},
		{name: "deleted request is ignored", instances: []*apiv1beta1.OpenStackLightspeed{first}, request: "deleted"},
		{name: "multiple instances block first", instances: []*apiv1beta1.OpenStackLightspeed{first, second}, request: first.Name, wantError: "only one OpenStackLightspeed instance per namespace is allowed"},
		{name: "multiple instances block second", instances: []*apiv1beta1.OpenStackLightspeed{first, second}, request: second.Name, wantError: "only one OpenStackLightspeed instance per namespace is allowed"},
		{name: "list error is returned", instances: []*apiv1beta1.OpenStackLightspeed{first}, request: first.Name, listErr: listErr, wantError: listErr.Error()},
	} {
		t.Run(tt.name, func(t *testing.T) {
			builder := fake.NewClientBuilder().WithScheme(scheme).
				WithStatusSubresource(&apiv1beta1.OpenStackLightspeed{})
			for _, instance := range tt.instances {
				builder.WithObjects(instance.DeepCopy())
			}
			fakeClient := builder.WithInterceptorFuncs(interceptor.Funcs{
				List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
					if tt.listErr != nil {
						return tt.listErr
					}
					return c.List(ctx, list, opts...)
				},
			}).Build()
			r := &OpenStackLightspeedReconciler{Client: fakeClient, Scheme: scheme}
			ctx := context.Background()
			result, err := r.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: tt.request, Namespace: first.Namespace},
			})
			if tt.wantError == "" {
				if err != nil {
					t.Fatalf("unexpected reconcile error: %v", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("expected error containing %q, got %v", tt.wantError, err)
			}
			if result != (reconcile.Result{}) {
				t.Fatalf("unexpected requeue: %+v", result)
			}
			for _, instance := range tt.instances {
				actual := &apiv1beta1.OpenStackLightspeed{}
				if err := fakeClient.Get(ctx, client.ObjectKeyFromObject(instance), actual); err != nil {
					t.Fatal(err)
				}
				wantReconciled := tt.reconcile && instance.Namespace == first.Namespace && instance.Name == tt.request
				if (len(actual.Finalizers) > 0) != wantReconciled || (len(actual.Status.Conditions) > 0) != wantReconciled {
					t.Errorf("unexpected reconciliation of %s/%s: finalizers=%v, conditions=%v",
						actual.Namespace, actual.Name, actual.Finalizers, actual.Status.Conditions)
				}
			}
		})
	}
}
