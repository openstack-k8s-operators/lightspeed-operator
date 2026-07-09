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
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	common_helper "github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	apiv1beta1 "github.com/openstack-k8s-operators/lightspeed-operator/api/v1beta1"
	uns "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// OpenStackLightspeedOCPVectorDBPath - base path for OCP vector databases
	OpenStackLightspeedOCPVectorDBPath = "/rag/ocp_vector_db/ocp"

	// OpenStackLightspeedOCPIndexPrefix - prefix for OCP index names
	OpenStackLightspeedOCPIndexPrefix = "ocp-product-docs"

	// Supported OCP versions in the RAG database
	OCPVersion416    = "4.16"
	OCPVersion418    = "4.18"
	OCPVersionLatest = "latest"
)

// SupportedOCPVersions lists the OCP versions available in the RAG database
var SupportedOCPVersions = []string{OCPVersion416, OCPVersion418, OCPVersionLatest}

// resolveOCPVersion detects and resolves the OCP version to use for RAG configuration.
// Returns the active OCP version to use (or empty string if OCP RAG is disabled).
func (r *OpenStackLightspeedReconciler) resolveOCPVersion(
	ctx context.Context,
	helper *common_helper.Helper,
	instance *apiv1beta1.OpenStackLightspeed,
) string {
	Log := helper.GetLogger()

	// If OCP RAG is disabled, mark condition as True with "disabled" message
	if !instance.Spec.EnableOCPRAG {
		instance.Status.Conditions.MarkTrue(
			apiv1beta1.OCPRAGCondition,
			apiv1beta1.OCPRAGDisabledMessage,
		)
		instance.Status.ActiveOCPRAGVersion = ""
		return ""
	}

	// Step 1: Detect cluster version
	detectedVersion, err := DetectOCPVersion(ctx, helper)

	if err != nil {
		Log.Info("Failed to detect OCP version, disabling OCP RAG", "error", err)
		cond := condition.FalseCondition(
			apiv1beta1.OCPRAGCondition,
			condition.ErrorReason,
			condition.SeverityError,
			apiv1beta1.OCPRAGDetectionFailedMessage,
		)
		cond.Message = fmt.Sprintf("%s: %s", apiv1beta1.OCPRAGDetectionFailedMessage, err.Error())
		instance.Status.Conditions.Set(cond)
		instance.Status.ActiveOCPRAGVersion = ""
		return ""
	}

	Log.Info("Detected OCP cluster version", "version", detectedVersion)

	// Step 2: Resolve which version to use (with override and fallback)
	activeVersion, isFallback, err := ResolveOCPVersion(
		detectedVersion,
		instance.Spec.OCPRAGVersionOverride,
		instance.Spec.EnableOCPRAG,
	)

	if err != nil {
		// Invalid override
		Log.Error(err, "Invalid OCP version configuration")
		cond := condition.FalseCondition(
			apiv1beta1.OCPRAGCondition,
			condition.ErrorReason,
			condition.SeverityError,
			apiv1beta1.OCPRAGOverrideInvalidMessage,
		)
		cond.Message = fmt.Sprintf("%s: %s", apiv1beta1.OCPRAGOverrideInvalidMessage, err.Error())
		instance.Status.Conditions.Set(cond)
		instance.Status.ActiveOCPRAGVersion = ""
		return ""
	}

	// Step 3: Update status and conditions based on resolution
	instance.Status.ActiveOCPRAGVersion = activeVersion

	if isFallback {
		Log.Info("Using 'latest' OCP documentation as fallback",
			"detectedVersion", detectedVersion,
			"supportedVersions", SupportedOCPVersions)

		cond := condition.TrueCondition(
			apiv1beta1.OCPRAGCondition,
			"Fallback",
		)
		cond.Message = fmt.Sprintf(apiv1beta1.OCPRAGVersionFallbackMessage,
			detectedVersion, SupportedOCPVersions)
		instance.Status.Conditions.Set(cond)
	} else {
		Log.Info("Using OCP RAG documentation", "version", activeVersion)
		cond := condition.TrueCondition(
			apiv1beta1.OCPRAGCondition,
			"Resolved",
		)
		cond.Message = fmt.Sprintf(apiv1beta1.OCPRAGVersionResolvedMessage, activeVersion)
		instance.Status.Conditions.Set(cond)
	}

	return activeVersion
}

// DetectOCPVersion detects the OpenShift cluster version
func DetectOCPVersion(ctx context.Context, helper *common_helper.Helper) (string, error) {
	// Use raw client to access cluster-scoped resources
	rawClient, err := getRawClient(helper)
	if err != nil {
		return "", fmt.Errorf("failed to get raw client: %w", err)
	}

	// Get ClusterVersion object
	clusterVersion := &uns.Unstructured{}
	clusterVersion.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "config.openshift.io",
		Version: "v1",
		Kind:    "ClusterVersion",
	})

	err = rawClient.Get(ctx, client.ObjectKey{Name: "version"}, clusterVersion)
	if err != nil {
		return "", fmt.Errorf("failed to get ClusterVersion: %w", err)
	}

	// Extract version from status.desired.version
	// NOTE: We intentionally use desired.version rather than history[0].version because:
	// - During OCP upgrades, desired.version reflects the target version
	// - Users troubleshooting upgrade issues need docs for the NEW version
	// - This provides proactive access to relevant documentation
	version, found, err := uns.NestedString(clusterVersion.Object, "status", "desired", "version")
	if err != nil {
		return "", fmt.Errorf("failed to extract version from ClusterVersion: %w", err)
	}
	if !found {
		return "", fmt.Errorf("version field not found in ClusterVersion status.desired.version")
	}

	// Parse version to get major.minor (e.g., "4.15.0" -> "4.15")
	majorMinor, err := ParseMajorMinorVersion(version)
	if err != nil {
		return "", fmt.Errorf("failed to parse version %s: %w", version, err)
	}

	return majorMinor, nil
}

// ParseMajorMinorVersion extracts major.minor version from full version string
// Example: "4.15.0-0.nightly-2024-01-15-123456" -> "4.15"
func ParseMajorMinorVersion(fullVersion string) (string, error) {
	// Match major.minor pattern at the start
	re := regexp.MustCompile(`^(\d+\.\d+)`)
	matches := re.FindStringSubmatch(fullVersion)

	if len(matches) < 2 {
		return "", fmt.Errorf("invalid version format: %s", fullVersion)
	}

	return matches[1], nil
}

// GetOCPIndexName converts version to index name format
// Example: "4.16" -> "ocp-product-docs-4_16"
//
//	"latest" -> "ocp-product-docs-latest"
func GetOCPIndexName(version string) string {
	// Replace dots with underscores (no-op for "latest")
	versionFormatted := strings.ReplaceAll(version, ".", "_")
	return fmt.Sprintf("%s-%s", OpenStackLightspeedOCPIndexPrefix, versionFormatted)
}

// GetOCPVectorDBPath returns the full path to OCP vector DB for given version
// Example: "4.16" -> "/rag/ocp_vector_db/ocp_4.16"
//
//	"latest" -> "/rag/ocp_vector_db/ocp_latest"
func GetOCPVectorDBPath(version string) string {
	return fmt.Sprintf("%s_%s", OpenStackLightspeedOCPVectorDBPath, version)
}

// IsSupportedOCPVersion checks if the version is explicitly supported in RAG DB
func IsSupportedOCPVersion(version string) bool {
	return slices.Contains(SupportedOCPVersions, version)
}

// ResolveOCPVersion determines the OCP version to use for RAG configuration
// Returns (version, isFallback, error)
// - version: The version to use (might be "latest" as fallback)
// - isFallback: true if falling back to "latest" for unsupported version
// - error: any error during version resolution
func ResolveOCPVersion(detectedVersion, overrideVersion string, enableOCPRAG bool) (string, bool, error) {
	if !enableOCPRAG {
		return "", false, nil
	}

	// Use override if provided
	if overrideVersion != "" {
		return overrideVersion, false, nil
	}

	if detectedVersion == "" {
		return "", false, fmt.Errorf("no OCP version detected")
	}

	// Check if detected version is supported
	if IsSupportedOCPVersion(detectedVersion) {
		return detectedVersion, false, nil
	}

	// Fallback to latest for unsupported versions
	return OCPVersionLatest, true, nil
}

// BuildRAGConfigs builds the RAG configuration array.
// OpenStack RAG is always included first.
// OCP RAG is added if ocpVersion is provided.
func BuildRAGConfigs(instance *apiv1beta1.OpenStackLightspeed, ocpVersion string) []interface{} {
	rags := []interface{}{
		// OpenStack RAG
		map[string]interface{}{
			"image":     apiv1beta1.OpenStackLightspeedDefaultValues.RAGImageURL,
			"indexPath": OpenStackLightspeedVectorDBPath,
		},
	}

	// Add OCP RAG if enabled
	if ocpVersion != "" {
		rags = append(rags, map[string]interface{}{
			"image":     apiv1beta1.OpenStackLightspeedDefaultValues.RAGImageURL,
			"indexPath": GetOCPVectorDBPath(ocpVersion),
			"indexID":   GetOCPIndexName(ocpVersion),
		})
	}

	return rags
}
