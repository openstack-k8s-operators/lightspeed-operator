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

	common_helper "github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	uns "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

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
