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
	"testing"

	apiv1beta1 "github.com/openstack-k8s-operators/lightspeed-operator/api/v1beta1"
)

const (
	// testRAGImage is the test image used in unit tests
	testRAGImage = "test-image:latest"
)

func TestGetOCPIndexName(t *testing.T) {
	tests := []struct {
		name     string
		version  string
		expected string
	}{
		{
			name:     "Version 4.16",
			version:  "4.16",
			expected: "ocp-product-docs-4_16",
		},
		{
			name:     "Version 4.18",
			version:  "4.18",
			expected: "ocp-product-docs-4_18",
		},
		{
			name:     "Latest version",
			version:  "latest",
			expected: "ocp-product-docs-latest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetOCPIndexName(tt.version)
			if result != tt.expected {
				t.Errorf("GetOCPIndexName(%s) = %s, want %s", tt.version, result, tt.expected)
			}
		})
	}
}

func TestGetOCPVectorDBPath(t *testing.T) {
	tests := []struct {
		name     string
		version  string
		expected string
	}{
		{
			name:     "Version 4.16",
			version:  "4.16",
			expected: "/rag/ocp_vector_db/ocp_4.16",
		},
		{
			name:     "Version 4.18",
			version:  "4.18",
			expected: "/rag/ocp_vector_db/ocp_4.18",
		},
		{
			name:     "Latest version",
			version:  "latest",
			expected: "/rag/ocp_vector_db/ocp_latest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetOCPVectorDBPath(tt.version)
			if result != tt.expected {
				t.Errorf("GetOCPVectorDBPath(%s) = %s, want %s", tt.version, result, tt.expected)
			}
		})
	}
}

func TestParseMajorMinorVersion(t *testing.T) {
	tests := []struct {
		name        string
		fullVersion string
		expected    string
		shouldError bool
	}{
		{
			name:        "Standard version",
			fullVersion: "4.16.0",
			expected:    "4.16",
			shouldError: false,
		},
		{
			name:        "Version with build",
			fullVersion: "4.18.0-0.nightly-2024-01-15-123456",
			expected:    "4.18",
			shouldError: false,
		},
		{
			name:        "Invalid version",
			fullVersion: "invalid",
			expected:    "",
			shouldError: true,
		},
		{
			name:        "Empty version",
			fullVersion: "",
			expected:    "",
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseMajorMinorVersion(tt.fullVersion)
			if tt.shouldError {
				if err == nil {
					t.Errorf("ParseMajorMinorVersion(%s) expected error, got nil", tt.fullVersion)
				}
			} else {
				if err != nil {
					t.Errorf("ParseMajorMinorVersion(%s) unexpected error: %v", tt.fullVersion, err)
				}
				if result != tt.expected {
					t.Errorf("ParseMajorMinorVersion(%s) = %s, want %s", tt.fullVersion, result, tt.expected)
				}
			}
		})
	}
}

func TestResolveOCPVersion(t *testing.T) {
	tests := []struct {
		name             string
		detected         string
		override         string
		enableOCPRAG     bool
		expectedVer      string
		expectedFallback bool
		shouldError      bool
	}{
		{
			name:             "OCP RAG disabled",
			detected:         "4.16",
			override:         "",
			enableOCPRAG:     false,
			expectedVer:      "",
			expectedFallback: false,
			shouldError:      false,
		},
		{
			name:             "Supported version detected",
			detected:         "4.16",
			override:         "",
			enableOCPRAG:     true,
			expectedVer:      "4.16",
			expectedFallback: false,
			shouldError:      false,
		},
		{
			name:             "Unsupported version - fallback",
			detected:         "4.17",
			override:         "",
			enableOCPRAG:     true,
			expectedVer:      "latest",
			expectedFallback: true,
			shouldError:      false,
		},
		{
			name:             "Version override",
			detected:         "4.18",
			override:         "4.16",
			enableOCPRAG:     true,
			expectedVer:      "4.16",
			expectedFallback: false,
			shouldError:      false,
		},
		{
			name:             "Custom override (any version allowed)",
			detected:         "4.16",
			override:         "4.99",
			enableOCPRAG:     true,
			expectedVer:      "4.99",
			expectedFallback: false,
			shouldError:      false,
		},
		{
			name:             "No version detected",
			detected:         "",
			override:         "",
			enableOCPRAG:     true,
			expectedVer:      "",
			expectedFallback: false,
			shouldError:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			version, isFallback, err := ResolveOCPVersion(tt.detected, tt.override, tt.enableOCPRAG)
			if tt.shouldError {
				if err == nil {
					t.Errorf("ResolveOCPVersion expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("ResolveOCPVersion unexpected error: %v", err)
				}
				if version != tt.expectedVer {
					t.Errorf("ResolveOCPVersion version = %s, want %s", version, tt.expectedVer)
				}
				if isFallback != tt.expectedFallback {
					t.Errorf("ResolveOCPVersion isFallback = %v, want %v", isFallback, tt.expectedFallback)
				}
			}
		})
	}
}

func TestBuildRAGConfigs(t *testing.T) {
	apiv1beta1.OpenStackLightspeedDefaultValues.RAGImageURL = testRAGImage

	t.Run("OCP RAG disabled (empty version)", func(t *testing.T) {
		instance := &apiv1beta1.OpenStackLightspeed{}

		configs := BuildRAGConfigs(instance, "")

		if len(configs) != 1 {
			t.Errorf("Expected 1 RAG config, got %d", len(configs))
		}

		// Type assert to map[string]interface{}
		firstConfig, ok := configs[0].(map[string]interface{})
		if !ok {
			t.Fatalf("Expected config to be map[string]interface{}, got %T", configs[0])
		}

		if firstConfig["image"] != testRAGImage {
			t.Errorf("Expected image test-image:latest, got %v", firstConfig["image"])
		}

		if firstConfig["indexPath"] != OpenStackLightspeedVectorDBPath {
			t.Errorf("Expected indexPath %s, got %v", OpenStackLightspeedVectorDBPath, firstConfig["indexPath"])
		}

		// Verify priority field is NOT present
		if _, hasPriority := firstConfig["priority"]; hasPriority {
			t.Errorf("Expected no priority field, but it was present")
		}
	})

	t.Run("OCP RAG enabled", func(t *testing.T) {
		instance := &apiv1beta1.OpenStackLightspeed{}

		configs := BuildRAGConfigs(instance, "4.16")

		if len(configs) != 2 {
			t.Errorf("Expected 2 RAG configs, got %d", len(configs))
		}

		// Check OpenStack RAG (first config)
		osConfig, ok := configs[0].(map[string]interface{})
		if !ok {
			t.Fatalf("Expected first config to be map[string]interface{}, got %T", configs[0])
		}

		if osConfig["image"] != testRAGImage {
			t.Errorf("OpenStack RAG image = %v, want test-image:latest", osConfig["image"])
		}

		if osConfig["indexPath"] != OpenStackLightspeedVectorDBPath {
			t.Errorf("OpenStack RAG indexPath = %v, want %s", osConfig["indexPath"], OpenStackLightspeedVectorDBPath)
		}

		// Verify priority field is NOT present in OpenStack config
		if _, hasPriority := osConfig["priority"]; hasPriority {
			t.Errorf("Expected no priority field in OpenStack config, but it was present")
		}

		// Check OCP RAG (second config)
		ocpConfig, ok := configs[1].(map[string]interface{})
		if !ok {
			t.Fatalf("Expected second config to be map[string]interface{}, got %T", configs[1])
		}

		if ocpConfig["image"] != testRAGImage {
			t.Errorf("OCP RAG image = %v, want test-image:latest", ocpConfig["image"])
		}

		ocpPath, ok := ocpConfig["indexPath"].(string)
		if !ok {
			t.Fatalf("Expected indexPath to be string, got %T", ocpConfig["indexPath"])
		}
		if ocpPath != "/rag/ocp_vector_db/ocp_4.16" {
			t.Errorf("OCP indexPath = %s, want /rag/ocp_vector_db/ocp_4.16", ocpPath)
		}

		ocpIndexID, ok := ocpConfig["indexID"].(string)
		if !ok {
			t.Fatalf("Expected indexID to be string, got %T", ocpConfig["indexID"])
		}
		if ocpIndexID != "ocp-product-docs-4_16" {
			t.Errorf("OCP indexID = %s, want ocp-product-docs-4_16", ocpIndexID)
		}

		// Verify priority field is NOT present in OCP config
		if _, hasPriority := ocpConfig["priority"]; hasPriority {
			t.Errorf("Expected no priority field in OCP config, but it was present")
		}
	})

	t.Run("OCP RAG with latest version", func(t *testing.T) {
		instance := &apiv1beta1.OpenStackLightspeed{}

		configs := BuildRAGConfigs(instance, "latest")

		if len(configs) != 2 {
			t.Errorf("Expected 2 RAG configs, got %d", len(configs))
		}

		// Check OCP RAG with latest version
		ocpConfig, ok := configs[1].(map[string]interface{})
		if !ok {
			t.Fatalf("Expected second config to be map[string]interface{}, got %T", configs[1])
		}

		ocpPath, ok := ocpConfig["indexPath"].(string)
		if !ok {
			t.Fatalf("Expected indexPath to be string, got %T", ocpConfig["indexPath"])
		}
		if ocpPath != "/rag/ocp_vector_db/ocp_latest" {
			t.Errorf("OCP indexPath = %s, want /rag/ocp_vector_db/ocp_latest", ocpPath)
		}

		ocpIndexID, ok := ocpConfig["indexID"].(string)
		if !ok {
			t.Fatalf("Expected indexID to be string, got %T", ocpConfig["indexID"])
		}
		if ocpIndexID != "ocp-product-docs-latest" {
			t.Errorf("OCP indexID = %s, want ocp-product-docs-latest", ocpIndexID)
		}
	})
}

func TestIsSupportedOCPVersion(t *testing.T) {
	tests := []struct {
		name     string
		version  string
		expected bool
	}{
		{
			name:     "Supported version 4.16",
			version:  "4.16",
			expected: true,
		},
		{
			name:     "Supported version 4.18",
			version:  "4.18",
			expected: true,
		},
		{
			name:     "Supported version latest",
			version:  "latest",
			expected: true,
		},
		{
			name:     "Unsupported version 4.17",
			version:  "4.17",
			expected: false,
		},
		{
			name:     "Unsupported version 4.19",
			version:  "4.19",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsSupportedOCPVersion(tt.version)
			if result != tt.expected {
				t.Errorf("IsSupportedOCPVersion(%s) = %v, want %v", tt.version, result, tt.expected)
			}
		})
	}
}
