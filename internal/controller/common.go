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

package controller

import (
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"slices"
	"strings"

	common_cm "github.com/openstack-k8s-operators/lib-common/modules/common/configmap"
	common_helper "github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	common_secret "github.com/openstack-k8s-operators/lib-common/modules/common/secret"
	apiv1beta1 "github.com/openstack-k8s-operators/lightspeed-operator/api/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// toPtr returns a pointer to the given value.
func toPtr[T any](v T) *T {
	return &v
}

// getRawClient returns a raw client that is not restricted to WATCH_NAMESPACE.
// This is useful for operations that need to query resources across all namespaces
// cluster wide.
func getRawClient(helper *common_helper.Helper) (client.Client, error) {
	cfg, err := config.GetConfig()
	if err != nil {
		return nil, err
	}

	rawClient, err := client.New(cfg, client.Options{Scheme: helper.GetScheme()})
	if err != nil {
		return nil, err
	}

	return rawClient, nil
}

// generateAppServerSelectorLabels returns a map of labels used as selectors
// for the application server pods.
func generateAppServerSelectorLabels() map[string]string {
	return map[string]string{
		"app.kubernetes.io/component":  "app-server",
		"app.kubernetes.io/managed-by": "openstack-lightspeed-operator",
		"app.kubernetes.io/name":       "openstack-lightspeed-app-server",
		"app.kubernetes.io/part-of":    "openstack-lightspeed",
	}
}

// getConfigMapResourceVersion retrieves the resource version of a ConfigMap.
func getConfigMapResourceVersion(ctx context.Context, h *common_helper.Helper, name string, namespace string) (string, error) {
	rawClient, err := getRawClient(h)
	if err != nil {
		return "", fmt.Errorf("failed to get raw client: %w", err)
	}

	cm := &corev1.ConfigMap{}
	err = rawClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, cm)
	if err != nil {
		return "", fmt.Errorf("failed to get configmap %s: %w", name, err)
	}
	return cm.ResourceVersion, nil
}

// getSecretResourceVersion retrieves the resource version of a Secret.
func getSecretResourceVersion(ctx context.Context, h *common_helper.Helper, name string, namespace string) (string, error) {
	rawClient, err := getRawClient(h)
	if err != nil {
		return "", fmt.Errorf("failed to get raw client: %w", err)
	}

	secret := &corev1.Secret{}
	err = rawClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, secret)
	if err != nil {
		return "", fmt.Errorf("failed to get secret %s: %w", name, err)
	}
	return secret.ResourceVersion, nil
}

// providerNameToEnvVarName converts a provider name to a valid environment variable name.
// It uppercases the string and replaces hyphens and dots with underscores.
func providerNameToEnvVarName(providerName string) string {
	name := strings.ToUpper(providerName)
	name = strings.ReplaceAll(name, "-", "_")
	name = strings.ReplaceAll(name, ".", "_")
	return name
}

// generatePostgresSelectorLabels returns selector labels for Postgres components.
func generatePostgresSelectorLabels() map[string]string {
	return map[string]string{
		"app.kubernetes.io/component":  "postgres-server",
		"app.kubernetes.io/managed-by": "openstack-lightspeed-operator",
		"app.kubernetes.io/name":       "openstack-lightspeed-service-postgres",
		"app.kubernetes.io/part-of":    "openstack-lightspeed",
	}
}

// isDeploymentReady checks whether the provided deployment is ready by verifying
// that the deployment's observed generation matches the current generation and
// all replicas (updated, available, and total) match the desired count.
func isDeploymentReady(deploy *appsv1.Deployment) bool {
	if deploy.Generation > deploy.Status.ObservedGeneration {
		return false
	}

	replicas := int32(1)
	if deploy.Spec.Replicas != nil {
		replicas = *deploy.Spec.Replicas
	}

	return deploy.Status.UpdatedReplicas == replicas &&
		deploy.Status.AvailableReplicas == replicas &&
		deploy.Status.Replicas == replicas
}

// generateOKPSelectorLabels returns selector labels for OKP components.
func generateOKPSelectorLabels() map[string]string {
	return map[string]string{
		"app.kubernetes.io/component":  "okp-server",
		"app.kubernetes.io/managed-by": "openstack-lightspeed-operator",
		"app.kubernetes.io/name":       "openstack-lightspeed-okp-server",
		"app.kubernetes.io/part-of":    "openstack-lightspeed",
	}
}

// parseDevConfig unmarshals the Dev RawExtension into a DevSpec.
// Returns a zero-value DevSpec and an error on malformed input.
func parseDevConfig(instance *apiv1beta1.OpenStackLightspeed) (apiv1beta1.DevSpec, error) {
	var config apiv1beta1.DevSpec
	if len(instance.Spec.Dev.Raw) > 0 {
		if err := json.Unmarshal(instance.Spec.Dev.Raw, &config); err != nil {
			return config, err
		}
	}
	return config, nil
}

// isRHOSMCPEnabled returns true if the "rhos_mcps" feature flag is present in the dev config.
func isRHOSMCPEnabled(instance *apiv1beta1.OpenStackLightspeed) (bool, error) {
	config, err := parseDevConfig(instance)
	if err != nil {
		return false, err
	}
	return slices.Contains(config.FeatureFlags, "rhos_mcps"), nil
}

// getOKPChunkFilterQuery returns the chunk filter query from the dev config, or a version-aware default.
func getOKPChunkFilterQuery(ctx context.Context, h *common_helper.Helper, instance *apiv1beta1.OpenStackLightspeed) string {
	config, _ := parseDevConfig(instance)
	if config.OKPChunkFilterQuery != "" {
		return config.OKPChunkFilterQuery
	}

	logger := h.GetLogger()

	ocpVersion, err := DetectOCPVersion(ctx, h)
	if err != nil {
		logger.Error(err, "Failed to detect OCP version, using default", "default", OKPDefaultOCPVersion)
		ocpVersion = OKPDefaultOCPVersion
	}

	osVersion := detectRHOSOVersion(ocpVersion, logger)

	return fmt.Sprintf(OKPChunkFilterQueryFmt, osVersion, ocpVersion)
}

// getDeployment retrieves deployment from the cluster
func getDeployment(ctx context.Context, h *common_helper.Helper, name string, namespace string) (*appsv1.Deployment, error) {
	deployment := &appsv1.Deployment{}
	err := h.GetClient().Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, deployment)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			return &appsv1.Deployment{}, errors.New("deployment not found")
		}
		return &appsv1.Deployment{}, fmt.Errorf("failed to get deployment %s: %w", name, err)
	}

	return deployment, nil
}

// generateRandomString generates a random alphanumeric string of the given length containing at
// least one lowercase letter, one uppercase letter, and one digit. Minimum required secretLength
// is 16.
func generateRandomString(secretLength int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	const charsetLen = len(charset)
	const minLength = 16
	const maxIterations = 1000

	if secretLength < minLength {
		return "", fmt.Errorf("secret length must be at least %d", minLength)
	}

	for range maxIterations {

		randPassword := make([]byte, secretLength)
		var hasLowerCase, hasUpperCase, hasNumber bool

		for passwdCharIdx := range secretLength {
			idx, err := rand.Int(rand.Reader, big.NewInt(int64(charsetLen)))
			if err != nil {
				return "", fmt.Errorf("failed to generate secret: %w", err)
			}

			passwdChar := charset[idx.Int64()]
			if passwdChar >= 'a' && passwdChar <= 'z' {
				hasLowerCase = true
			} else if passwdChar >= 'A' && passwdChar <= 'Z' {
				hasUpperCase = true
			} else if passwdChar >= '0' && passwdChar <= '9' {
				hasNumber = true
			}

			randPassword[passwdCharIdx] = passwdChar
		}

		if hasLowerCase && hasUpperCase && hasNumber {
			return string(randPassword), nil
		}
	}

	return "", fmt.Errorf("failed to generate secret: exceeded iterations - %d", maxIterations)
}

// GetCRDName returns the name of the CustomResourceDefinition (CRD) for a given
// GroupVersionKind (GVK). The CRD name is constructed as "<Kind>s.<Group>" string.
func GetCRDName(gvk schema.GroupVersionKind) string {
	return fmt.Sprintf("%ss.%s", strings.ToLower(gvk.Kind), gvk.Group)
}

// IsCRDEstablished checks if a CRD exists and is in "Established" state (ready for use).
// Returns (true, nil) if the CRD exists and is established, (false, nil) if it doesn't exist,
// and (false, error) for other errors.
func IsCRDEstablished(ctx context.Context, helper *common_helper.Helper, gvk schema.GroupVersionKind) (bool, error) {
	crdName := GetCRDName(gvk)
	crd := &apiextensionsv1.CustomResourceDefinition{}
	err := helper.GetClient().Get(ctx, client.ObjectKey{Name: crdName}, crd)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}

	for _, cond := range crd.Status.Conditions {
		if cond.Type == apiextensionsv1.Established && cond.Status == apiextensionsv1.ConditionTrue {
			return true, nil
		}
	}

	return false, nil
}

// OpenStackControlPlaneGVK returns the GroupVersionKind for OpenStackControlPlane.
func OpenStackControlPlaneGVK() schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   OpenStackControlPlaneGroup,
		Version: OpenStackControlPlaneVersion,
		Kind:    OpenStackControlPlaneKind,
	}
}

// KeystoneApplicationCredentialGVK returns the GroupVersionKind for KeystoneApplicationCredential.
func KeystoneApplicationCredentialGVK() schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   KeystoneApplicationCredentialGroup,
		Version: KeystoneApplicationCredentialVersion,
		Kind:    KeystoneApplicationCredentialKind,
	}
}

// IsDynamicCRDWatched reports whether a controller-runtime watch is currently
// registered for the given GVK. The flag is reset to false when a CRD is
// removed and its broken informer is cleaned up. This does NOT indicate
// whether the CRD currently exists — use IsCRDEstablished for that.
func IsDynamicCRDWatched(
	dynamicWatchCRD DynamicWatchCRD,
	gvk schema.GroupVersionKind,
) (bool, error) {
	seen, exists := dynamicWatchCRD[gvk]
	if !exists {
		return false, fmt.Errorf("GVK %v not found in DynamicWatchCRD map", gvk)
	}
	return seen.Load(), nil
}

// SetChecksumAnnotation sets or updates the checksum annotation on the provided object.
func SetChecksumAnnotation(object client.Object, checksum string) {
	annotations := object.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations[OpenStackLightspeedChecksumAnnotation] = checksum
	object.SetAnnotations(annotations)
}

// GetChecksumAnnotation retrieves the checksum annotation from the given object.
// If the annotation is not found, it returns an empty string.
func GetChecksumAnnotation(object client.Object) string {
	annotations := object.GetAnnotations()
	if annotations == nil {
		return ""
	}
	checksum, ok := annotations[OpenStackLightspeedChecksumAnnotation]
	if !ok {
		return ""
	}
	return checksum
}

// CopyResource copies a resource (Secret or ConfigMap) from one namespace to another,
// setting a controller reference on the copy and computing checksums.
func CopyResource(
	ctx context.Context,
	helper *common_helper.Helper,
	sourceObject client.Object,
	targetObject client.Object,
	owner client.Object,
	scheme *runtime.Scheme,
) (client.Object, error) {
	var copyObject client.Object
	var err error

	switch source := sourceObject.(type) {
	case *corev1.Secret:
		fetched, fetchErr := helper.GetKClient().CoreV1().Secrets(source.GetNamespace()).Get(ctx, source.GetName(), metav1.GetOptions{})
		if fetchErr != nil {
			return nil, fetchErr
		}

		copySecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      targetObject.GetName(),
				Namespace: targetObject.GetNamespace(),
			},
		}

		_, err = controllerutil.CreateOrPatch(ctx, helper.GetClient(), copySecret, func() error {
			copySecret.Data = fetched.Data
			copySecret.StringData = fetched.StringData
			copySecret.Type = fetched.Type
			if err := controllerutil.SetControllerReference(owner, copySecret, scheme); err != nil {
				return err
			}

			checksum, err := common_secret.Hash(copySecret)
			if err != nil {
				return err
			}
			SetChecksumAnnotation(copySecret, checksum)
			return nil
		})

		copyObject = copySecret
	case *corev1.ConfigMap:
		fetched, fetchErr := helper.GetKClient().CoreV1().ConfigMaps(source.GetNamespace()).Get(ctx, source.GetName(), metav1.GetOptions{})
		if fetchErr != nil {
			return nil, fetchErr
		}

		copyConfigMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      targetObject.GetName(),
				Namespace: targetObject.GetNamespace(),
			},
		}

		_, err = controllerutil.CreateOrPatch(ctx, helper.GetClient(), copyConfigMap, func() error {
			copyConfigMap.Data = fetched.Data
			copyConfigMap.BinaryData = fetched.BinaryData
			if err := controllerutil.SetControllerReference(owner, copyConfigMap, scheme); err != nil {
				return err
			}

			checksum, err := common_cm.Hash(copyConfigMap)
			if err != nil {
				return err
			}
			SetChecksumAnnotation(copyConfigMap, checksum)
			return nil
		})

		copyObject = copyConfigMap
	default:
		return nil, errors.New("cannot copy resource (invalid type)")
	}

	if err != nil {
		return nil, err
	}

	return copyObject, nil
}
