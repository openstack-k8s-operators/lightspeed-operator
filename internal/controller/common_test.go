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
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	apiv1beta1 "github.com/openstack-k8s-operators/lightspeed-operator/api/v1beta1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// TestOKPChunkFilterQueryFmtExcludesOpenShiftVirtualization guards against OKP RAG
// answers being grounded in OpenShift Virtualization docs when the query is about
// OpenStack. OpenShift Virtualization docs share the same "openshift_container_platform"
// product as docs we want to keep (e.g. Migration Toolkit for Containers), so they can
// only be distinguished by their parent_id path (".../html-single/virtualization/index").
func TestOKPChunkFilterQueryFmtExcludesOpenShiftVirtualization(t *testing.T) {
	query := fmt.Sprintf(OKPChunkFilterQueryFmt, "18.0", "4.21")

	if !strings.Contains(query, "product:*openstack* AND product_version:18.0") {
		t.Errorf("expected OpenStack clause with version 18.0, got: %s", query)
	}
	if !strings.Contains(query, "product:*openshift*") {
		t.Errorf("expected OpenShift clause, got: %s", query)
	}
	if !strings.Contains(query, "product_version:4.21") {
		t.Errorf("expected OpenShift clause with version 4.21, got: %s", query)
	}
	if !strings.Contains(query, "-parent_id:*html-single/virtualization/*") {
		t.Errorf("expected OpenShift Virtualization docs to be excluded via parent_id, got: %s", query)
	}
}

func TestGenerateRandomStringLength(t *testing.T) {
	t.Run("Below minimum length returns error", func(t *testing.T) {
		_, err := generateRandomString(15)
		if err == nil {
			t.Error("generateRandomString(15) expected error, got nil")
		}
	})

	tests := []struct {
		name   string
		length int
	}{
		{name: "Min length 16", length: 16},
		{name: "Even length 32", length: 32},
		{name: "Odd length 33", length: 33},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := generateRandomString(tt.length)
			if err != nil {
				t.Errorf("generateRandomString(%d) unexpected error: %v", tt.length, err)
			}
			if len(result) != tt.length {
				t.Errorf("generateRandomString(%d) returned length %d, want %d", tt.length, len(result), tt.length)
			}
		})
	}
}

func TestGenerateRandomStringCharacters(t *testing.T) {
	result, err := generateRandomString(32)
	if err != nil {
		t.Fatalf("generateRandomString(32) unexpected error: %v", err)
	}

	var hasLower, hasUpper, hasDigit bool
	for i, c := range result {
		switch {
		case c >= 'a' && c <= 'z':
			hasLower = true
		case c >= 'A' && c <= 'Z':
			hasUpper = true
		case c >= '0' && c <= '9':
			hasDigit = true
		default:
			t.Errorf("generateRandomString(32) character at index %d is %q, not alphanumeric", i, c)
		}
	}
	if !hasLower {
		t.Error("generateRandomString(32) result has no lowercase letter")
	}
	if !hasUpper {
		t.Error("generateRandomString(32) result has no uppercase letter")
	}
	if !hasDigit {
		t.Error("generateRandomString(32) result has no digit")
	}
}

func TestGenerateRandomStringUniqueness(t *testing.T) {
	const length = 16
	a, err := generateRandomString(length)
	if err != nil {
		t.Fatalf("first call unexpected error: %v", err)
	}
	b, err := generateRandomString(length)
	if err != nil {
		t.Fatalf("second call unexpected error: %v", err)
	}
	if a == b {
		t.Errorf("generateRandomString(%d) returned identical values across two calls: %q", length, a)
	}
}

func TestGetRhosMCPResources_DefaultsWhenUnset(t *testing.T) {
	instance := &apiv1beta1.OpenStackLightspeed{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
	}

	resources := getRhosMCPResources(instance)
	defaults := defaultRhosMCPResources()

	if !resources.Requests.Cpu().Equal(*defaults.Requests.Cpu()) {
		t.Errorf("expected default CPU request %v, got %v", defaults.Requests.Cpu(), resources.Requests.Cpu())
	}
	if !resources.Requests.Memory().Equal(*defaults.Requests.Memory()) {
		t.Errorf("expected default memory request %v, got %v", defaults.Requests.Memory(), resources.Requests.Memory())
	}
	if !resources.Limits.Memory().Equal(*defaults.Limits.Memory()) {
		t.Errorf("expected default memory limit %v, got %v", defaults.Limits.Memory(), resources.Limits.Memory())
	}
}

func TestGetRhosMCPResources_CustomFromDevConfig(t *testing.T) {
	devRaw, err := json.Marshal(map[string]interface{}{
		"rhosMCP": map[string]interface{}{
			"resources": map[string]interface{}{
				"requests": map[string]string{
					"cpu":    "100m",
					"memory": "128Mi",
				},
				"limits": map[string]string{
					"memory": "256Mi",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("failed to marshal dev config: %v", err)
	}

	instance := &apiv1beta1.OpenStackLightspeed{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: apiv1beta1.OpenStackLightspeedSpec{
			Dev: runtime.RawExtension{Raw: devRaw},
		},
	}

	resources := getRhosMCPResources(instance)

	expectedCPU := resource.MustParse("100m")
	if !resources.Requests.Cpu().Equal(expectedCPU) {
		t.Errorf("expected CPU request %v, got %v", expectedCPU, resources.Requests.Cpu())
	}
	expectedMemory := resource.MustParse("128Mi")
	if !resources.Requests.Memory().Equal(expectedMemory) {
		t.Errorf("expected memory request %v, got %v", expectedMemory, resources.Requests.Memory())
	}
	expectedLimit := resource.MustParse("256Mi")
	if !resources.Limits.Memory().Equal(expectedLimit) {
		t.Errorf("expected memory limit %v, got %v", expectedLimit, resources.Limits.Memory())
	}
}

func TestBuildMCPServerConfigMap_UsesDevRhosMCPConfig(t *testing.T) {
	devRaw, err := json.Marshal(map[string]interface{}{
		"rhosMCP": map[string]interface{}{
			"config": "debug: true\nworkers: 2\n",
		},
	})
	if err != nil {
		t.Fatalf("failed to marshal dev config: %v", err)
	}

	instance := &apiv1beta1.OpenStackLightspeed{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: apiv1beta1.OpenStackLightspeedSpec{
			Dev: runtime.RawExtension{Raw: devRaw},
		},
	}

	configMap, err := BuildMCPServerConfigMap(instance, false, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	configData := configMap.Data["config.yaml"]
	if configData == "" {
		t.Fatal("expected config.yaml data")
	}
	if !containsAll(configData, "debug: true", "workers: 2") {
		t.Errorf("expected merged config to contain user overrides, got:\n%s", configData)
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}
