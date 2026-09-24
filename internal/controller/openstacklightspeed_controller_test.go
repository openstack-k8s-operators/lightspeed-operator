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
	"sync/atomic"
	"testing"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
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
							LLMEndpoint:     "https://example.com/llm",
							LLMEndpointType: OpenAIProviderName,
							ModelName:       "test-model",
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
				DynamicWatchCRD: DynamicWatchCRD{
					OpenStackControlPlaneGVK():         new(atomic.Bool),
					KeystoneApplicationCredentialGVK(): new(atomic.Bool),
				},
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			// TODO(user): Add more specific assertions depending on your controller's reconciliation logic.
			// Example: If you expect a certain status condition after reconciliation, verify it here.
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
