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
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	common_helper "github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	apiv1beta1 "github.com/openstack-k8s-operators/lightspeed-operator/api/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
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

// isOKPEnabled returns true if the "okp" feature flag is present in the dev config.
func isOKPEnabled(instance *apiv1beta1.OpenStackLightspeed) bool {
	config, _ := parseDevConfig(instance)
	return slices.Contains(config.FeatureFlags, "okp")
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
