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
	"bytes"
	"context"
	"errors"
	"fmt"
	"text/template"

	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	common_helper "github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	apiv1beta1 "github.com/openstack-k8s-operators/lightspeed-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	uns "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const (
	// CloudsYAMLConfigMapName is the name of the ConfigMap containing clouds.yaml.
	CloudsYAMLConfigMapName string = "openstack-config"

	// SecureYAMLSecretName is the name of the Secret containing secure.yaml.
	SecureYAMLSecretName string = "openstack-config-secret" // #nosec G101 -- Kubernetes Secret resource name, not a credential

	// CombinedCABundleSecretName is the name of the Secret containing the TLS CA bundle.
	CombinedCABundleSecretName string = "combined-ca-bundle" // #nosec G101 -- Kubernetes Secret resource name, not a credential

	// MCPConfigYAMLConfigMapName is the name of the ConfigMap containing config.yaml for the MCP server.
	MCPConfigYAMLConfigMapName string = "mcp-config"

	// MCPServerPort is the port on which the MCP server listens.
	MCPServerPort = 8080
)

// ---------------------------------------------------------------------------
// Builders
// ---------------------------------------------------------------------------

// mcpServerConfigTmpl is the parsed MCP server config template.
// Parsed once at package init from the //go:embed string mcpServerConfigTemplate.
var mcpServerConfigTmpl = template.Must(template.New("mcp-config").Parse(mcpServerConfigTemplate))

// mcpServerConfigParams holds the template parameters for the MCP server config.
type mcpServerConfigParams struct {
	OpenStackEnabled bool
	OpenShiftEnabled bool
}

// buildMCPServerConfigData renders the MCP server config template with the
// enabled flags for each platform section.
func buildMCPServerConfigData(openStackReady bool) (string, error) {
	var buf bytes.Buffer
	err := mcpServerConfigTmpl.Execute(&buf, mcpServerConfigParams{
		OpenStackEnabled: openStackReady,
		OpenShiftEnabled: true,
	})
	if err != nil {
		return "", fmt.Errorf("failed to render MCP server config template: %w", err)
	}

	return buf.String(), nil
}

// BuildMCPServerConfigMap creates the ConfigMap for the MCP server configuration.
func BuildMCPServerConfigMap(
	instance *apiv1beta1.OpenStackLightspeed,
	openStackReady bool,
) (corev1.ConfigMap, error) {
	configData, err := buildMCPServerConfigData(openStackReady)
	if err != nil {
		return corev1.ConfigMap{}, err
	}

	configMap := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      MCPConfigYAMLConfigMapName,
			Namespace: instance.Namespace,
		},
		Data: map[string]string{
			"config.yaml": configData,
		},
	}

	return configMap, nil
}

// GetMCPServerURL returns the internal cluster URL for the MCP server sidecar.
func GetMCPServerURL() string {
	return fmt.Sprintf("http://127.0.0.1:%d", MCPServerPort)
}

// ---------------------------------------------------------------------------
// Reconciliation
// ---------------------------------------------------------------------------

// ReconcileMCPServerTask reconciles the MCP server as a ReconcileFunc.
func (r *OpenStackLightspeedReconciler) ReconcileMCPServerTask(h *common_helper.Helper, ctx context.Context, instance *apiv1beta1.OpenStackLightspeed) error {
	rhosMCPEnabled, err := isRHOSMCPEnabled(instance)
	if err != nil {
		return fmt.Errorf("failed to parse dev config: %w", err)
	}
	if rhosMCPEnabled {
		openStackReady, mcpErr := r.ReconcileMCPServer(ctx, h, instance)
		if mcpErr != nil {
			instance.Status.Conditions.Set(condition.FalseCondition(
				apiv1beta1.OpenStackLightspeedMCPServerReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				apiv1beta1.DeploymentCheckFailedMessage,
				mcpErr.Error(),
			))
			return mcpErr
		}
		instance.Status.OpenStackReady = openStackReady
	} else {
		if err := r.cleanupMCPResources(ctx, h, instance); err != nil {
			return err
		}
		instance.Status.OpenStackReady = false
		instance.Status.Conditions.MarkTrue(
			apiv1beta1.OpenStackLightspeedMCPServerReadyCondition,
			apiv1beta1.OpenStackLightspeedMCPServerDisabledMessage,
		)
	}
	return nil
}

// ReconcileMCPServer performs the reconciliation of the MCP server.
// The MCP server runs as a sidecar in the LCore pod. The OpenStack MCP tools
// are only configured in lightspeed-stack when OpenStackControlPlane exists and is Ready.
// Returns whether OpenStackControlPlane is ready (for config generation).
func (r *OpenStackLightspeedReconciler) ReconcileMCPServer(
	ctx context.Context,
	helper *common_helper.Helper,
	instance *apiv1beta1.OpenStackLightspeed,
) (openStackReady bool, err error) {
	crdReady, err := IsDynamicCRDWatched(r.DynamicWatchCRD, OpenStackControlPlaneGVK())
	if err != nil {
		return false, err
	}

	if !crdReady {
		helper.GetLogger().Info("OpenStackControlPlane CRD not available, deploying MCP server without OpenStack resources")
		return false, r.reconcileMCPServerDeploy(ctx, helper, instance, false)
	}

	// The watch is registered, but the CRD may have been removed since.
	crdEstablished, err := IsCRDEstablished(ctx, helper, OpenStackControlPlaneGVK())
	if err != nil {
		return false, err
	}
	if !crdEstablished {
		helper.GetLogger().Info("OpenStackControlPlane CRD was removed, deploying MCP server without OpenStack resources")
		return false, r.reconcileMCPServerDeploy(ctx, helper, instance, false)
	}

	openStackControlPlaneList, err := r.listOpenStackControlPlanes(ctx, helper)
	if err != nil {
		return false, err
	}

	switch l := len(openStackControlPlaneList.Items); l {
	case 0:
		helper.GetLogger().Info("No OpenStackControlPlane found, deploying MCP server without OpenStack resources")
		return false, r.reconcileMCPServerDeploy(ctx, helper, instance, false)

	case 1:
		oscp := &openStackControlPlaneList.Items[0]
		return r.reconcileMCPServerWithOpenStack(ctx, helper, instance, oscp)

	default:
		return false, errors.New("more than one OpenStackControlPlane found")
	}
}

// listOpenStackControlPlanes lists all OpenStackControlPlane instances.
// It first tries the cached client. If the cache returns 0 items (which
// may happen when the cache ByObject config does not cover the namespace
// where OpenStackControlPlane lives), it falls back to a direct API call.
func (r *OpenStackLightspeedReconciler) listOpenStackControlPlanes(
	ctx context.Context,
	helper *common_helper.Helper,
) (*uns.UnstructuredList, error) {
	openStackControlPlaneList := &uns.UnstructuredList{}
	openStackControlPlaneList.SetGroupVersionKind(OpenStackControlPlaneGVK().GroupVersion().WithKind("OpenStackControlPlaneList"))

	err := r.List(ctx, openStackControlPlaneList)
	if err != nil && !k8s_errors.IsNotFound(err) {
		return nil, err
	}

	if len(openStackControlPlaneList.Items) > 0 {
		return openStackControlPlaneList, nil
	}

	rawClient, err := getRawClient(helper)
	if err != nil {
		return nil, fmt.Errorf("failed to get raw client for OpenStackControlPlane fallback list: %w", err)
	}

	fallbackList := &uns.UnstructuredList{}
	fallbackList.SetGroupVersionKind(OpenStackControlPlaneGVK().GroupVersion().WithKind("OpenStackControlPlaneList"))
	if err := rawClient.List(ctx, fallbackList); err != nil {
		if k8s_errors.IsNotFound(err) {
			return fallbackList, nil
		}
		return nil, err
	}

	if len(fallbackList.Items) > 0 {
		helper.GetLogger().Info(
			fmt.Sprintf("OpenStackControlPlane not found in cache but found %d via direct API call (cache may not cover the namespace)", len(fallbackList.Items)),
		)
	}

	return fallbackList, nil
}

// reconcileMCPServerWithOpenStack copies OpenStack resources and reconciles the MCP config.
func (r *OpenStackLightspeedReconciler) reconcileMCPServerWithOpenStack(
	ctx context.Context,
	helper *common_helper.Helper,
	instance *apiv1beta1.OpenStackLightspeed,
	oscp *uns.Unstructured,
) (bool, error) {
	log := helper.GetLogger()

	fields, err := extractOSCPFields(helper, oscp)
	if err != nil {
		log.Info(fmt.Sprintf("OpenStackControlPlane field check failed with error: %v", err))
		return false, err
	}
	if fields == nil {
		log.Info("OpenStackControlPlane fields not ready yet, deploying MCP without OpenStack resources")
		return false, r.reconcileMCPServerDeploy(ctx, helper, instance, false)
	}

	_, err = copyObjectsToOpenStackLightspeedNamespace(ctx, helper, instance, oscp, fields)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			log.Info(fmt.Sprintf("OpenStack resource not found (%v), deploying MCP without OpenStack resources", err))
			return false, r.reconcileMCPServerDeploy(ctx, helper, instance, false)
		}
		return false, err
	}

	if err := r.reconcileMCPServerDeploy(ctx, helper, instance, true); err != nil {
		return false, err
	}

	log.Info("MCP server reconciled with OpenStack resources")
	return true, nil
}

// reconcileMCPServerDeploy ensures the MCP server ConfigMap exists.
func (r *OpenStackLightspeedReconciler) reconcileMCPServerDeploy(
	ctx context.Context,
	helper *common_helper.Helper,
	instance *apiv1beta1.OpenStackLightspeed,
	openStackReady bool,
) error {
	configYAMLConfigMap := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      MCPConfigYAMLConfigMapName,
			Namespace: instance.Namespace,
		},
	}
	_, err := controllerutil.CreateOrPatch(ctx, helper.GetClient(), &configYAMLConfigMap, func() error {
		built, err := BuildMCPServerConfigMap(instance, openStackReady)
		if err != nil {
			return err
		}
		configYAMLConfigMap.Data = built.Data
		return controllerutil.SetControllerReference(helper.GetBeforeObject(), &configYAMLConfigMap, helper.GetScheme())
	})
	return err
}

// oscpFields holds the validated fields extracted from an OpenStackControlPlane.
type oscpFields struct {
	configSecret       string
	configMap          string
	caBundleSecretName string
}

// extractOSCPFields extracts and validates the required fields from an OpenStackControlPlane.
// Returns (nil, nil) when the status TLS field is not yet populated (waiting for readiness).
func extractOSCPFields(
	helper *common_helper.Helper,
	oscp *uns.Unstructured,
) (*oscpFields, error) {
	configSecret, found, err := uns.NestedString(oscp.Object, "spec", "openstackclient", "template", "openStackConfigSecret")
	if err != nil || !found || configSecret == "" {
		return nil, fmt.Errorf("OpenStackClient.Template.OpenStackConfigSecret is missing value")
	}

	configMap, found, err := uns.NestedString(oscp.Object, "spec", "openstackclient", "template", "openStackConfigMap")
	if err != nil || !found || configMap == "" {
		return nil, fmt.Errorf("OpenStackControlPlane.OpenStackClient.Template.OpenStackConfigMap is missing value")
	}

	caBundleSecretName, found, err := uns.NestedString(oscp.Object, "status", "tls", "caBundleSecretName")
	if err != nil || !found || caBundleSecretName == "" {
		helper.GetLogger().Info("Waiting for OpenStackControlPlane.Status.TLS.CaBundleSecretName value")
		return nil, nil
	}

	return &oscpFields{
		configSecret:       configSecret,
		configMap:          configMap,
		caBundleSecretName: caBundleSecretName,
	}, nil
}

// ---------------------------------------------------------------------------
// Deletion
// ---------------------------------------------------------------------------

// cleanupMCPResources removes MCP server resources when the rhos_mcps feature
// flag is disabled.
func (r *OpenStackLightspeedReconciler) cleanupMCPResources(
	ctx context.Context,
	helper *common_helper.Helper,
	instance *apiv1beta1.OpenStackLightspeed,
) error {
	log := helper.GetLogger()
	ns := instance.Namespace

	mcpCM := &corev1.ConfigMap{}
	mcpCM.Name = MCPConfigYAMLConfigMapName
	mcpCM.Namespace = ns
	if err := helper.GetClient().Delete(ctx, mcpCM); err != nil && !k8s_errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete MCP config ConfigMap: %w", err)
	}

	cloudsCM := &corev1.ConfigMap{}
	cloudsCM.Name = CloudsYAMLConfigMapName
	cloudsCM.Namespace = ns
	if err := helper.GetClient().Delete(ctx, cloudsCM); err != nil && !k8s_errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete openstack-config ConfigMap: %w", err)
	}

	secureSec := &corev1.Secret{}
	secureSec.Name = SecureYAMLSecretName
	secureSec.Namespace = ns
	if err := helper.GetClient().Delete(ctx, secureSec); err != nil && !k8s_errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete openstack-config-secret Secret: %w", err)
	}

	caSec := &corev1.Secret{}
	caSec.Name = CombinedCABundleSecretName
	caSec.Namespace = ns
	if err := helper.GetClient().Delete(ctx, caSec); err != nil && !k8s_errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete combined-ca-bundle Secret: %w", err)
	}

	log.Info("RHOS MCP resources cleaned up")
	return nil
}

// copyObjectsToOpenStackLightspeedNamespace copies the required ConfigMaps and Secrets
// from the OpenStackControlPlane's namespace to the OpenStack Lightspeed namespace.
func copyObjectsToOpenStackLightspeedNamespace(
	ctx context.Context,
	helper *common_helper.Helper,
	instance *apiv1beta1.OpenStackLightspeed,
	oscp *uns.Unstructured,
	fields *oscpFields,
) (map[string]client.Object, error) {
	objectsToCopy := map[string]client.Object{
		SecureYAMLSecretName: &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fields.configSecret,
				Namespace: oscp.GetNamespace(),
			},
		},
		CloudsYAMLConfigMapName: &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fields.configMap,
				Namespace: oscp.GetNamespace(),
			},
		},
		CombinedCABundleSecretName: &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fields.caBundleSecretName,
				Namespace: oscp.GetNamespace(),
			},
		},
	}

	copiedObjects := make(map[string]client.Object)

	for resourceName, sourceObject := range objectsToCopy {
		targetObject := sourceObject.DeepCopyObject().(client.Object)
		targetObject.SetNamespace(instance.Namespace)
		targetObject.SetName(resourceName)

		copied, err := CopyResource(ctx, helper, sourceObject, targetObject, instance, helper.GetScheme())
		if err != nil {
			if k8s_errors.IsNotFound(err) {
				helper.GetLogger().Info(
					fmt.Sprintf("Resource %s not found in namespace %s, waiting for it to be created",
						sourceObject.GetName(), sourceObject.GetNamespace()),
				)
			}
			return nil, err
		}
		if copied == nil {
			return nil, errors.New("the internal representation of the copied object is nil")
		}
		copiedObjects[resourceName] = copied
	}

	return copiedObjects, nil
}
