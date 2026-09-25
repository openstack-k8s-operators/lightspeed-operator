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
	"sync/atomic"
	"testing"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
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

func newSingletonScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 to scheme: %v", err)
	}
	if err := apiv1beta1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add apiv1beta1 to scheme: %v", err)
	}
	return scheme
}

func newLightspeed(name, namespace string, created time.Time) *apiv1beta1.OpenStackLightspeed {
	return &apiv1beta1.OpenStackLightspeed{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         namespace,
			UID:               types.UID(namespace + "/" + name),
			CreationTimestamp: metav1.NewTime(created),
		},
	}
}

func TestTakesPrecedence(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	older := newLightspeed("b-older", "ns", base)
	newer := newLightspeed("a-newer", "ns", base.Add(time.Minute))

	if !takesPrecedence(older, newer) {
		t.Errorf("expected older instance to take precedence over newer one")
	}
	if takesPrecedence(newer, older) {
		t.Errorf("expected newer instance to NOT take precedence over older one")
	}

	// Equal timestamps: the lexicographically smaller name wins the tie.
	sameA := newLightspeed("aaa", "ns", base)
	sameB := newLightspeed("bbb", "ns", base)
	if !takesPrecedence(sameA, sameB) {
		t.Errorf("expected name tie-break: aaa should take precedence over bbb")
	}
	if takesPrecedence(sameB, sameA) {
		t.Errorf("expected name tie-break: bbb should NOT take precedence over aaa")
	}
}

func TestIsPrimaryInstance(t *testing.T) {
	scheme := newSingletonScheme(t)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	primary := newLightspeed("primary", "ns", base)
	duplicate := newLightspeed("duplicate", "ns", base.Add(time.Minute))
	// An instance in a different namespace must not affect the decision.
	otherNS := newLightspeed("primary", "other-ns", base.Add(-time.Hour))

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(primary, duplicate, otherNS).
		Build()

	r := &OpenStackLightspeedReconciler{Client: fakeClient, Scheme: scheme}
	ctx := context.Background()

	isPrimary, err := r.isPrimaryInstance(ctx, primary)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isPrimary {
		t.Errorf("expected the oldest instance to be primary")
	}

	isPrimary, err = r.isPrimaryInstance(ctx, duplicate)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if isPrimary {
		t.Errorf("expected the newer instance to be a duplicate, not primary")
	}
}

func TestIsPrimaryInstance_SingleInstance(t *testing.T) {
	scheme := newSingletonScheme(t)
	only := newLightspeed("only", "ns", time.Now())

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(only).
		Build()

	r := &OpenStackLightspeedReconciler{Client: fakeClient, Scheme: scheme}

	isPrimary, err := r.isPrimaryInstance(context.Background(), only)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isPrimary {
		t.Errorf("expected a lone instance to be primary")
	}
}
