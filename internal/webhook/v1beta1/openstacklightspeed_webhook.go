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

// Package v1beta1 implements admission validation for OpenStackLightspeed.
package v1beta1

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	lightspeedv1beta1 "github.com/openstack-k8s-operators/lightspeed-operator/api/v1beta1"
)

// SetupOpenStackLightspeedWebhookWithManager registers the validating webhook.
func SetupOpenStackLightspeedWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&lightspeedv1beta1.OpenStackLightspeed{}).
		WithValidator(&OpenStackLightspeedCustomValidator{Reader: mgr.GetAPIReader()}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-lightspeed-openstack-org-v1beta1-openstacklightspeed,mutating=false,failurePolicy=fail,sideEffects=None,groups=lightspeed.openstack.org,resources=openstacklightspeeds,verbs=create,versions=v1beta1,name=vopenstacklightspeed-v1beta1.kb.io,admissionReviewVersions=v1

// OpenStackLightspeedCustomValidator rejects additional instances in a namespace.
type OpenStackLightspeedCustomValidator struct {
	Reader client.Reader
}

var _ webhook.CustomValidator = &OpenStackLightspeedCustomValidator{}

// ValidateCreate allows creation only when the namespace has no existing instance.
func (v *OpenStackLightspeedCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	instance, ok := obj.(*lightspeedv1beta1.OpenStackLightspeed)
	if !ok {
		return nil, fmt.Errorf("expected an OpenStackLightspeed object but got %T", obj)
	}

	// Read directly from the API server to avoid cache lag. Concurrent creates
	// can still pass this check, so the controller retains its duplicate guard.
	var instances lightspeedv1beta1.OpenStackLightspeedList
	if err := v.Reader.List(ctx, &instances, client.InNamespace(instance.Namespace)); err != nil {
		return nil, apierrors.NewInternalError(fmt.Errorf("failed to check existing OpenStackLightspeed instances: %w", err))
	}
	if len(instances.Items) > 0 {
		errs := field.ErrorList{
			field.Forbidden(
				field.NewPath("metadata", "namespace"),
				fmt.Sprintf("only one OpenStackLightspeed instance per namespace is allowed; %q already exists", instances.Items[0].Name),
			),
		}
		return nil, apierrors.NewInvalid(
			lightspeedv1beta1.GroupVersion.WithKind("OpenStackLightspeed").GroupKind(),
			instance.Name,
			errs,
		)
	}
	return nil, nil
}

// ValidateUpdate allows updates so existing instances can still be managed.
func (v *OpenStackLightspeedCustomValidator) ValidateUpdate(_ context.Context, _, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

// ValidateDelete allows deletion so existing instances can always be removed.
func (v *OpenStackLightspeedCustomValidator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}
