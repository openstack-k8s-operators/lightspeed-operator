/*
Copyright 2026.

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

package v1beta1

import (
	"context"
	"errors"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	lightspeedv1beta1 "github.com/openstack-k8s-operators/lightspeed-operator/api/v1beta1"
)

func TestValidateCreate(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := lightspeedv1beta1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	instance := &lightspeedv1beta1.OpenStackLightspeed{
		ObjectMeta: metav1.ObjectMeta{Name: "new", Namespace: "lightspeed"},
	}
	existing := &lightspeedv1beta1.OpenStackLightspeed{
		ObjectMeta: metav1.ObjectMeta{Name: "existing", Namespace: instance.Namespace},
	}
	otherNamespace := existing.DeepCopy()
	otherNamespace.Namespace = "other"
	terminating := existing.DeepCopy()
	now := metav1.Now()
	terminating.DeletionTimestamp = &now
	terminating.Finalizers = []string{"lightspeed.openstack.org/finalizer"}

	for _, tt := range []struct {
		name       string
		objects    []client.Object
		obj        runtime.Object
		listErr    error
		checkErr   func(error) bool
		message    string
		causeField string
	}{
		{name: "first instance", obj: instance},
		{name: "different namespace", obj: instance, objects: []client.Object{otherNamespace}},
		{name: "duplicate", obj: instance, objects: []client.Object{existing}, checkErr: apierrors.IsInvalid, message: `metadata.namespace: Forbidden: only one OpenStackLightspeed instance per namespace is allowed; "existing" already exists`, causeField: "metadata.namespace"},
		{name: "terminating instance still blocks creation", obj: instance, objects: []client.Object{terminating}, checkErr: apierrors.IsInvalid, causeField: "metadata.namespace"},
		{name: "list failure denies creation", obj: instance, listErr: errors.New("API unavailable"), checkErr: apierrors.IsInternalError, message: "API unavailable"},
		{name: "unexpected object", obj: &corev1.Secret{}, checkErr: func(err error) bool { return err != nil }, message: "expected an OpenStackLightspeed object"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			reader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tt.objects...).WithInterceptorFuncs(interceptor.Funcs{
				List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
					if tt.listErr != nil {
						return tt.listErr
					}
					return c.List(ctx, list, opts...)
				},
			}).Build()
			validator := &OpenStackLightspeedCustomValidator{Reader: reader}
			_, err := validator.ValidateCreate(context.Background(), tt.obj)
			if tt.checkErr == nil {
				if err != nil {
					t.Fatalf("expected creation to be allowed: %v", err)
				}
			} else if !tt.checkErr(err) {
				t.Fatalf("unexpected validation error: %v", err)
			}
			if tt.message != "" && (err == nil || !strings.Contains(err.Error(), tt.message)) {
				t.Fatalf("expected error containing %q, got %v", tt.message, err)
			}
			if tt.causeField != "" {
				cause, ok := apierrors.StatusCause(err, metav1.CauseTypeForbidden)
				if !ok || cause.Field != tt.causeField {
					t.Fatalf("expected forbidden field %q, got %+v", tt.causeField, cause)
				}
			}
		})
	}
}

func TestUpdatesAndDeletesDoNotRequireLookup(t *testing.T) {
	validator := &OpenStackLightspeedCustomValidator{}
	instance := &lightspeedv1beta1.OpenStackLightspeed{}
	if _, err := validator.ValidateUpdate(context.Background(), instance, instance); err != nil {
		t.Fatalf("expected update to be allowed: %v", err)
	}
	if _, err := validator.ValidateDelete(context.Background(), instance); err != nil {
		t.Fatalf("expected deletion to be allowed: %v", err)
	}
}
