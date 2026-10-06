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
	"fmt"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"
)

func TestBuildMCPServerConfigData_OpenStackNotReady(t *testing.T) {
	result, err := buildMCPServerConfigData(false, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Count(result, "enabled: false") != 1 {
		t.Error("expected exactly one 'enabled: false' (openstack) in config when OpenStack is not ready")
	}
	if strings.Count(result, "enabled: true") != 1 {
		t.Error("expected exactly one 'enabled: true' (openshift) in config when OpenStack is not ready")
	}
}

func TestBuildMCPServerConfigData_OpenStackReady(t *testing.T) {
	result, err := buildMCPServerConfigData(true, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Count(result, "enabled: true") != 2 {
		t.Error("expected two 'enabled: true' (openstack + openshift) in config when OpenStack is ready")
	}
	if strings.Contains(result, "enabled: false") {
		t.Error("unexpected 'enabled: false' in config when OpenStack is ready")
	}
}

func TestBuildMCPServerConfigData_CustomConfig_OverridesEnabled(t *testing.T) {
	customConfig := `
openstack:
  enabled: true
  allow_write: true
openshift:
  enabled: false
`
	result, err := buildMCPServerConfigData(false, customConfig)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]interface{}
	if err := yaml.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	osSection := parsed["openstack"].(map[string]interface{})
	if osSection["enabled"] != false {
		t.Errorf("expected openstack.enabled=false (operator override), got %v", osSection["enabled"])
	}
	if osSection["allow_write"] != true {
		t.Errorf("expected openstack.allow_write=true (user value), got %v", osSection["allow_write"])
	}

	ocpSection := parsed["openshift"].(map[string]interface{})
	if ocpSection["enabled"] != true {
		t.Errorf("expected openshift.enabled=true (operator override), got %v", ocpSection["enabled"])
	}
}

func TestBuildMCPServerConfigData_CustomConfig_DeepMergesWithDefaults(t *testing.T) {
	customConfig := `
debug: true
workers: 4
`
	result, err := buildMCPServerConfigData(false, customConfig)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]interface{}
	if err := yaml.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	if parsed["debug"] != true {
		t.Errorf("expected debug=true (user override), got %v", parsed["debug"])
	}
	if parsed["workers"] != float64(4) {
		t.Errorf("expected workers=4 (user override), got %v", parsed["workers"])
	}
	if parsed["port"] != float64(8080) {
		t.Errorf("expected port=8080 (template default preserved), got %v", parsed["port"])
	}

	osSection := parsed["openstack"].(map[string]interface{})
	if osSection["ca_cert"] != "./tls-ca-bundle.pem" {
		t.Errorf("expected openstack.ca_cert preserved from template, got %v", osSection["ca_cert"])
	}
}

func TestBuildMCPServerConfigData_CustomConfig_AddsNewFields(t *testing.T) {
	customConfig := `
custom_section:
  key1: value1
  key2: 42
`
	result, err := buildMCPServerConfigData(true, customConfig)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]interface{}
	if err := yaml.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	custom := parsed["custom_section"].(map[string]interface{})
	if custom["key1"] != "value1" {
		t.Errorf("expected custom_section.key1=value1, got %v", custom["key1"])
	}
	if custom["key2"] != float64(42) {
		t.Errorf("expected custom_section.key2=42, got %v", custom["key2"])
	}
}

func TestBuildMCPServerConfigData_CustomConfig_InvalidYAML(t *testing.T) {
	_, err := buildMCPServerConfigData(false, "not: valid: yaml: [")
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestBuildLCoreMCPServersConfig_WithOpenStack(t *testing.T) {
	servers := buildLCoreMCPServersConfig(true)

	if len(servers) != 2 {
		t.Errorf("expected 2 MCP servers, got %d", len(servers))
	}

	// Verify first server is OpenShift MCP
	first := servers[0].(map[string]interface{})
	if first["name"] != "rhoso-ocp-tools" {
		t.Errorf("expected first server name 'rhoso-ocp-tools', got '%s'", first["name"])
	}
	expectedURL := fmt.Sprintf("http://127.0.0.1:%d/openshift/", MCPServerPort)
	if first["url"] != expectedURL {
		t.Errorf("expected first server url '%s', got '%s'", expectedURL, first["url"])
	}
	authHeaders := first["authorization_headers"].(map[string]interface{})
	if authHeaders["OCP_TOKEN"] != "kubernetes" {
		t.Errorf("expected OCP_TOKEN authorization_header 'kubernetes', got '%s'", authHeaders["OCP_TOKEN"])
	}

	// Verify second server is OpenStack MCP
	second := servers[1].(map[string]interface{})
	if second["name"] != "rhoso-osp-tools" {
		t.Errorf("expected second server name 'rhoso-osp-tools', got '%s'", second["name"])
	}
	expectedURL = fmt.Sprintf("http://127.0.0.1:%d/openstack/", MCPServerPort)
	if second["url"] != expectedURL {
		t.Errorf("expected second server url '%s', got '%s'", expectedURL, second["url"])
	}
}

func TestBuildLCoreMCPServersConfig_WithoutOpenStack(t *testing.T) {
	servers := buildLCoreMCPServersConfig(false)

	if len(servers) != 1 {
		t.Errorf("expected 1 MCP server, got %d", len(servers))
	}

	// Verify only OpenShift MCP is present
	first := servers[0].(map[string]interface{})
	if first["name"] != "rhoso-ocp-tools" {
		t.Errorf("expected first server name 'rhoso-ocp-tools', got '%s'", first["name"])
	}
}

func TestGetMCPServerURL(t *testing.T) {
	expected := fmt.Sprintf("http://127.0.0.1:%d", MCPServerPort)
	actual := GetMCPServerURL()
	if actual != expected {
		t.Errorf("expected '%s', got '%s'", expected, actual)
	}
}
