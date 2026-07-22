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
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"text/template"

	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	common_helper "github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	common_secret "github.com/openstack-k8s-operators/lib-common/modules/common/secret"
	openstack_lib "github.com/openstack-k8s-operators/lib-common/modules/openstack"
	apiv1beta1 "github.com/openstack-k8s-operators/lightspeed-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	uns "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/yaml"
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

// ---------------------------------------------------------------------------
// Cloud config parsing
// ---------------------------------------------------------------------------

type cloudsYAML struct {
	Clouds map[string]cloudYAMLEntry `json:"clouds"`
}

type cloudYAMLEntry struct {
	Auth       cloudYAMLAuth `json:"auth"`
	RegionName string        `json:"region_name"`
}

type cloudYAMLAuth struct {
	AuthURL  string `json:"auth_url"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// parseCloudConfig reads the openstackclient's clouds.yaml and secure.yaml
// from the OSCP namespace and returns the merged config for the first cloud.
func parseCloudConfig(
	ctx context.Context,
	helper *common_helper.Helper,
	oscp *uns.Unstructured,
) (*cloudYAMLEntry, error) {
	configMapName, _, _ := uns.NestedString(oscp.Object, "spec", "openstackclient", "template", "openStackConfigMap")
	configSecretName, _, _ := uns.NestedString(oscp.Object, "spec", "openstackclient", "template", "openStackConfigSecret")
	oscpNS := oscp.GetNamespace()

	kclient := helper.GetKClient()

	cm, err := kclient.CoreV1().ConfigMaps(oscpNS).Get(ctx, configMapName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to read clouds.yaml configmap %s/%s: %w", oscpNS, configMapName, err)
	}

	var clouds cloudsYAML
	if err := yaml.Unmarshal([]byte(cm.Data["clouds.yaml"]), &clouds); err != nil {
		return nil, fmt.Errorf("failed to parse clouds.yaml: %w", err)
	}

	sec, err := kclient.CoreV1().Secrets(oscpNS).Get(ctx, configSecretName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to read secure.yaml secret %s/%s: %w", oscpNS, configSecretName, err)
	}

	var secClouds cloudsYAML
	if err := yaml.Unmarshal(sec.Data["secure.yaml"], &secClouds); err != nil {
		return nil, fmt.Errorf("failed to parse secure.yaml: %w", err)
	}

	var name string
	switch len(clouds.Clouds) {
	case 0:
		return nil, errors.New("no cloud entry found in clouds.yaml")
	case 1:
		for name = range clouds.Clouds {
		}
	default:
		if _, ok := clouds.Clouds["default"]; ok {
			name = "default"
		} else {
			names := make([]string, 0, len(clouds.Clouds))
			for n := range clouds.Clouds {
				names = append(names, n)
			}
			sort.Strings(names)
			return nil, fmt.Errorf("clouds.yaml has multiple entries (%s) and none is named \"default\"", strings.Join(names, ", "))
		}
	}

	entry := clouds.Clouds[name]
	if secEntry, ok := secClouds.Clouds[name]; ok {
		if secEntry.Auth.Password != "" {
			entry.Auth.Password = secEntry.Auth.Password
		}
	}
	return &entry, nil
}

// ---------------------------------------------------------------------------
// OpenStack client
// ---------------------------------------------------------------------------

func getOpenStackClient(
	ctx context.Context,
	helper *common_helper.Helper,
	oscp *uns.Unstructured,
	caPEM []byte,
) (*openstack_lib.OpenStack, *cloudYAMLEntry, error) {
	log := helper.GetLogger()

	cloudCfg, err := parseCloudConfig(ctx, helper, oscp)
	if err != nil {
		return nil, nil, err
	}

	// If caPEM was not provided, read it from the OSCP CA bundle secret.
	if caPEM == nil {
		caBundleSecretName, _, _ := uns.NestedString(oscp.Object, "status", "tls", "caBundleSecretName")
		if caBundleSecretName != "" {
			caSecret, err := helper.GetKClient().CoreV1().Secrets(oscp.GetNamespace()).Get(ctx, caBundleSecretName, metav1.GetOptions{})
			if err != nil {
				return nil, nil, fmt.Errorf("failed to read CA bundle secret: %w", err)
			}
			caPEM = caSecret.Data["tls-ca-bundle.pem"]
		}
	}

	var tlsCfg *openstack_lib.TLSConfig
	if len(caPEM) > 0 {
		tlsCfg = &openstack_lib.TLSConfig{CACerts: []string{string(caPEM)}}
	}

	osClient, err := openstack_lib.NewOpenStack(log, openstack_lib.AuthOpts{
		AuthURL:    cloudCfg.Auth.AuthURL,
		Username:   cloudCfg.Auth.Username,
		Password:   cloudCfg.Auth.Password,
		TenantName: "admin",
		DomainName: "Default",
		Region:     cloudCfg.RegionName,
		TLS:        tlsCfg,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to authenticate with keystone: %w", err)
	}

	return osClient, cloudCfg, nil
}

// ---------------------------------------------------------------------------
// Service user management
// ---------------------------------------------------------------------------

func ensurePasswordSecret(
	ctx context.Context,
	helper *common_helper.Helper,
	namespace string,
) (string, error) {
	kclient := helper.GetKClient()

	sec, err := kclient.CoreV1().Secrets(namespace).Get(ctx, LightspeedPasswordSecretName, metav1.GetOptions{})
	if err == nil {
		return string(sec.Data[LightspeedPasswordSecretKey]), nil
	}
	if !k8s_errors.IsNotFound(err) {
		return "", err
	}

	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate password: %w", err)
	}
	password := hex.EncodeToString(b)

	newSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      LightspeedPasswordSecretName,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			LightspeedPasswordSecretKey: []byte(password),
		},
	}
	_, err = kclient.CoreV1().Secrets(namespace).Create(ctx, newSecret, metav1.CreateOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to create password secret: %w", err)
	}

	return password, nil
}

func ensureServiceUser(
	ctx context.Context,
	helper *common_helper.Helper,
	osClient *openstack_lib.OpenStack,
	oscpNamespace string,
) error {
	log := helper.GetLogger()

	password, err := ensurePasswordSecret(ctx, helper, oscpNamespace)
	if err != nil {
		return err
	}

	userID, err := osClient.CreateUser(log, openstack_lib.User{
		Name:     LightspeedServiceUserName,
		Password: password,
		DomainID: LightspeedServiceUserDomain,
	})
	if err != nil {
		return fmt.Errorf("failed to create keystone user: %w", err)
	}

	serviceProject, err := osClient.GetProject(log, "service", LightspeedServiceUserDomain)
	if err != nil {
		return fmt.Errorf("failed to get service project: %w", err)
	}

	if err := osClient.AssignUserRole(log, "admin", userID, serviceProject.ID); err != nil {
		return fmt.Errorf("failed to assign admin role: %w", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Application credential management
// ---------------------------------------------------------------------------

func ensureApplicationCredential(
	ctx context.Context,
	helper *common_helper.Helper,
	namespace string,
) error {
	rawClient, err := getRawClient(helper)
	if err != nil {
		return fmt.Errorf("failed to get raw client: %w", err)
	}

	acCR := &uns.Unstructured{}
	acCR.SetGroupVersionKind(KeystoneApplicationCredentialGVK())
	acCR.SetName(LightspeedACCRName)
	acCR.SetNamespace(namespace)

	err = rawClient.Get(ctx, types.NamespacedName{Name: LightspeedACCRName, Namespace: namespace}, acCR)
	if err == nil {
		return nil
	}
	if !k8s_errors.IsNotFound(err) {
		return err
	}

	acCR.SetAnnotations(map[string]string{
		"keystone.openstack.org/edpm-service": "false",
	})
	if err := uns.SetNestedField(acCR.Object, LightspeedServiceUserName, "spec", "userName"); err != nil {
		return err
	}
	if err := uns.SetNestedField(acCR.Object, LightspeedPasswordSecretName, "spec", "secret"); err != nil {
		return err
	}
	if err := uns.SetNestedField(acCR.Object, LightspeedPasswordSecretKey, "spec", "passwordSelector"); err != nil {
		return err
	}
	if err := uns.SetNestedStringSlice(acCR.Object, []string{"admin"}, "spec", "roles"); err != nil {
		return err
	}

	helper.GetLogger().Info("Creating KeystoneApplicationCredential CR", "namespace", namespace)
	return rawClient.Create(ctx, acCR)
}

// reconcileACSecret reads the AC secret name from the CR status, manages
// finalizers for rotation safety, and returns the credential values.
func reconcileACSecret(
	ctx context.Context,
	helper *common_helper.Helper,
	instance *apiv1beta1.OpenStackLightspeed,
	oscpNamespace string,
) (acID, acSecret string, ready bool, err error) {
	rawClient, err := getRawClient(helper)
	if err != nil {
		return "", "", false, err
	}

	acCR := &uns.Unstructured{}
	acCR.SetGroupVersionKind(KeystoneApplicationCredentialGVK())
	if err := rawClient.Get(ctx, types.NamespacedName{Name: LightspeedACCRName, Namespace: oscpNamespace}, acCR); err != nil {
		return "", "", false, fmt.Errorf("failed to read AC CR: %w", err)
	}

	secretName, _, _ := uns.NestedString(acCR.Object, "status", "secretName")
	if secretName == "" {
		return "", "", false, nil
	}

	kclient := helper.GetKClient()

	// Add finalizer to current AC secret
	acSecretObj, err := kclient.CoreV1().Secrets(oscpNamespace).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			return "", "", false, nil
		}
		return "", "", false, err
	}

	if !controllerutil.ContainsFinalizer(acSecretObj, LightspeedACFinalizerName) {
		controllerutil.AddFinalizer(acSecretObj, LightspeedACFinalizerName)
		if _, err := kclient.CoreV1().Secrets(oscpNamespace).Update(ctx, acSecretObj, metav1.UpdateOptions{}); err != nil {
			return "", "", false, fmt.Errorf("failed to add finalizer to AC secret: %w", err)
		}
	}

	// Handle rotation: remove finalizer from previous secret
	prevSecret := instance.Status.ApplicationCredentialSecret
	if prevSecret != "" && prevSecret != secretName {
		helper.GetLogger().Info("AC secret rotated", "old", prevSecret, "new", secretName)
		oldSecret, err := kclient.CoreV1().Secrets(oscpNamespace).Get(ctx, prevSecret, metav1.GetOptions{})
		if err == nil && controllerutil.RemoveFinalizer(oldSecret, LightspeedACFinalizerName) {
			if _, err := kclient.CoreV1().Secrets(oscpNamespace).Update(ctx, oldSecret, metav1.UpdateOptions{}); err != nil {
				helper.GetLogger().Info("Failed to remove finalizer from old AC secret", "secret", prevSecret, "error", err)
			}
		}
	}

	instance.Status.ApplicationCredentialSecret = secretName

	return string(acSecretObj.Data["AC_ID"]), string(acSecretObj.Data["AC_SECRET"]), true, nil
}

// ---------------------------------------------------------------------------
// MCP credential generation
// ---------------------------------------------------------------------------

func generateMCPCredentials(
	ctx context.Context,
	helper *common_helper.Helper,
	instance *apiv1beta1.OpenStackLightspeed,
	cloudCfg *cloudYAMLEntry,
	acID, acSecret string,
) error {
	type acAuth struct {
		AuthURL                     string `json:"auth_url,omitempty"`
		ApplicationCredentialID     string `json:"application_credential_id,omitempty"`
		ApplicationCredentialSecret string `json:"application_credential_secret,omitempty"`
	}
	type acEntry struct {
		AuthType           string `json:"auth_type,omitempty"`
		Auth               acAuth `json:"auth"`
		RegionName         string `json:"region_name,omitempty"`
		IdentityAPIVersion int    `json:"identity_api_version,omitempty"`
	}
	type acClouds struct {
		Clouds map[string]acEntry `json:"clouds"`
	}

	cloudsBytes, err := yaml.Marshal(acClouds{
		Clouds: map[string]acEntry{
			"default": {
				AuthType: "v3applicationcredential",
				Auth: acAuth{
					AuthURL:                 cloudCfg.Auth.AuthURL,
					ApplicationCredentialID: acID,
				},
				RegionName:         cloudCfg.RegionName,
				IdentityAPIVersion: 3,
			},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to marshal clouds.yaml: %w", err)
	}

	cloudsConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      CloudsYAMLConfigMapName,
			Namespace: instance.Namespace,
		},
	}
	_, err = controllerutil.CreateOrPatch(ctx, helper.GetClient(), cloudsConfigMap, func() error {
		cloudsConfigMap.Data = map[string]string{"clouds.yaml": string(cloudsBytes)}
		return controllerutil.SetControllerReference(instance, cloudsConfigMap, helper.GetScheme())
	})
	if err != nil {
		return fmt.Errorf("failed to create clouds.yaml configmap: %w", err)
	}

	secureBytes, err := yaml.Marshal(acClouds{
		Clouds: map[string]acEntry{
			"default": {
				Auth: acAuth{
					ApplicationCredentialSecret: acSecret,
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to marshal secure.yaml: %w", err)
	}

	secureSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      SecureYAMLSecretName,
			Namespace: instance.Namespace,
		},
	}
	_, err = controllerutil.CreateOrPatch(ctx, helper.GetClient(), secureSecret, func() error {
		secureSecret.Data = map[string][]byte{"secure.yaml": secureBytes}
		if err := controllerutil.SetControllerReference(instance, secureSecret, helper.GetScheme()); err != nil {
			return err
		}
		checksum, err := common_secret.Hash(secureSecret)
		if err != nil {
			return err
		}
		SetChecksumAnnotation(secureSecret, checksum)
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to create secure.yaml secret: %w", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// CA bundle copy
// ---------------------------------------------------------------------------

func readCABundle(
	ctx context.Context,
	helper *common_helper.Helper,
	oscp *uns.Unstructured,
) ([]byte, error) {
	caBundleSecretName, _, _ := uns.NestedString(oscp.Object, "status", "tls", "caBundleSecretName")

	secret, err := helper.GetKClient().CoreV1().Secrets(oscp.GetNamespace()).Get(ctx, caBundleSecretName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return secret.Data["tls-ca-bundle.pem"], nil
}

func copyCABundle(
	ctx context.Context,
	helper *common_helper.Helper,
	instance *apiv1beta1.OpenStackLightspeed,
	oscp *uns.Unstructured,
) error {
	caBundleSecretName, _, _ := uns.NestedString(oscp.Object, "status", "tls", "caBundleSecretName")

	source := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      caBundleSecretName,
			Namespace: oscp.GetNamespace(),
		},
	}
	target := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      CombinedCABundleSecretName,
			Namespace: instance.Namespace,
		},
	}

	_, err := CopyResource(ctx, helper, source, target, instance, helper.GetScheme())
	return err
}

// ---------------------------------------------------------------------------
// Reconciliation
// ---------------------------------------------------------------------------

// reconcileMCPServerWithOpenStack creates a service user, application credential,
// generates credential files, and reconciles the MCP config.
func (r *OpenStackLightspeedReconciler) reconcileMCPServerWithOpenStack(
	ctx context.Context,
	helper *common_helper.Helper,
	instance *apiv1beta1.OpenStackLightspeed,
	oscp *uns.Unstructured,
) (bool, error) {
	log := helper.GetLogger()
	oscpNS := oscp.GetNamespace()

	fieldsReady, err := extractOSCPFields(helper, oscp)
	if err != nil {
		log.Info(fmt.Sprintf("OpenStackControlPlane field check failed: %v", err))
		return false, err
	}
	if !fieldsReady {
		log.Info("OpenStackControlPlane fields not ready, deploying MCP without OpenStack")
		return false, r.reconcileMCPServerDeploy(ctx, helper, instance, false)
	}

	caPEM, err := readCABundle(ctx, helper, oscp)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			log.Info("CA bundle not found, deploying MCP without OpenStack")
			return false, r.reconcileMCPServerDeploy(ctx, helper, instance, false)
		}
		return false, err
	}

	osClient, cloudCfg, err := getOpenStackClient(ctx, helper, oscp, caPEM)
	if err != nil {
		return false, fmt.Errorf("failed to get OpenStack client: %w", err)
	}

	instance.Status.Conditions.Set(condition.FalseCondition(
		apiv1beta1.OpenStackLightspeedMCPServerReadyCondition,
		condition.RequestedReason,
		condition.SeverityInfo,
		apiv1beta1.OpenStackLightspeedMCPServerCreatingUser,
	))

	if err := ensureServiceUser(ctx, helper, osClient, oscpNS); err != nil {
		return false, fmt.Errorf("failed to ensure service user: %w", err)
	}

	if err := ensureApplicationCredential(ctx, helper, oscpNS); err != nil {
		return false, fmt.Errorf("failed to ensure application credential: %w", err)
	}

	acID, acSec, ready, err := reconcileACSecret(ctx, helper, instance, oscpNS)
	if err != nil {
		return false, err
	}
	if !ready {
		log.Info("Application credential secret not ready, deploying MCP without OpenStack")
		instance.Status.Conditions.Set(condition.FalseCondition(
			apiv1beta1.OpenStackLightspeedMCPServerReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			apiv1beta1.OpenStackLightspeedMCPServerWaitingAC,
		))
		return false, r.reconcileMCPServerDeploy(ctx, helper, instance, false)
	}

	if err := copyCABundle(ctx, helper, instance, oscp); err != nil {
		return false, fmt.Errorf("failed to copy CA bundle: %w", err)
	}

	if err := generateMCPCredentials(ctx, helper, instance, cloudCfg, acID, acSec); err != nil {
		return false, err
	}

	if err := r.reconcileMCPServerDeploy(ctx, helper, instance, true); err != nil {
		return false, err
	}

	log.Info("MCP server reconciled with application credentials")
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

// extractOSCPFields validates that the required fields in an OpenStackControlPlane are populated.
// Returns (false, nil) when the status TLS field is not yet populated (waiting for readiness).
func extractOSCPFields(
	helper *common_helper.Helper,
	oscp *uns.Unstructured,
) (bool, error) {
	configSecret, found, err := uns.NestedString(oscp.Object, "spec", "openstackclient", "template", "openStackConfigSecret")
	if err != nil || !found || configSecret == "" {
		return false, fmt.Errorf("OpenStackClient.Template.OpenStackConfigSecret is missing value")
	}

	configMap, found, err := uns.NestedString(oscp.Object, "spec", "openstackclient", "template", "openStackConfigMap")
	if err != nil || !found || configMap == "" {
		return false, fmt.Errorf("OpenStackControlPlane.OpenStackClient.Template.OpenStackConfigMap is missing value")
	}

	caBundleSecretName, found, err := uns.NestedString(oscp.Object, "status", "tls", "caBundleSecretName")
	if err != nil || !found || caBundleSecretName == "" {
		helper.GetLogger().Info("Waiting for OpenStackControlPlane.Status.TLS.CaBundleSecretName value")
		return false, nil
	}

	return true, nil
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

	if err := r.reconcileDeleteOpenStackResources(ctx, helper, instance); err != nil {
		return fmt.Errorf("failed to clean up OpenStack resources during RHOS MCP disable: %w", err)
	}

	log.Info("RHOS MCP resources cleaned up")
	return nil
}

// reconcileDeleteOpenStackResources cleans up OpenStack resources created for the
// MCP server: AC secret finalizer, AC CR, keystone user, and password secret.
func (r *OpenStackLightspeedReconciler) reconcileDeleteOpenStackResources(
	ctx context.Context,
	helper *common_helper.Helper,
	instance *apiv1beta1.OpenStackLightspeed,
) error {
	log := helper.GetLogger()

	crdReady, err := IsDynamicCRDWatched(r.DynamicWatchCRD, OpenStackControlPlaneGVK())
	if err != nil || !crdReady {
		return err
	}

	openStackControlPlaneList, err := r.listOpenStackControlPlanes(ctx, helper)
	if err != nil {
		return fmt.Errorf("failed to list OpenStackControlPlanes during deletion: %w", err)
	}
	if len(openStackControlPlaneList.Items) != 1 {
		return nil
	}
	oscp := &openStackControlPlaneList.Items[0]
	oscpNS := oscp.GetNamespace()

	// Remove finalizer from AC secret
	if instance.Status.ApplicationCredentialSecret != "" {
		acSecret, err := helper.GetKClient().CoreV1().Secrets(oscpNS).Get(
			ctx, instance.Status.ApplicationCredentialSecret, metav1.GetOptions{})
		if err == nil && controllerutil.RemoveFinalizer(acSecret, LightspeedACFinalizerName) {
			if _, err := helper.GetKClient().CoreV1().Secrets(oscpNS).Update(ctx, acSecret, metav1.UpdateOptions{}); err != nil {
				log.Info("Failed to remove finalizer from AC secret", "error", err)
			}
		}
	}

	// Delete AC CR
	rawClient, err := getRawClient(helper)
	if err == nil {
		acCR := &uns.Unstructured{}
		acCR.SetGroupVersionKind(KeystoneApplicationCredentialGVK())
		acCR.SetName(LightspeedACCRName)
		acCR.SetNamespace(oscpNS)
		if err := rawClient.Delete(ctx, acCR); err != nil && !k8s_errors.IsNotFound(err) {
			log.Info("Failed to delete AC CR", "error", err)
		}
	}

	// Delete keystone user (best-effort)
	osClient, _, err := getOpenStackClient(ctx, helper, oscp, nil)
	if err == nil {
		if err := osClient.DeleteUser(log, LightspeedServiceUserName, LightspeedServiceUserDomain); err != nil {
			log.Info("Failed to delete keystone user (best-effort)", "error", err)
		}
	} else {
		log.Info("Could not connect to keystone for user deletion (best-effort)", "error", err)
	}

	// Delete password secret
	if err := helper.GetKClient().CoreV1().Secrets(oscpNS).Delete(
		ctx, LightspeedPasswordSecretName, metav1.DeleteOptions{}); err != nil && !k8s_errors.IsNotFound(err) {
		log.Info("Failed to delete password secret", "error", err)
	}

	return nil
}
