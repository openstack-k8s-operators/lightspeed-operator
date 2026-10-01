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
	"testing"

	apiv1beta1 "github.com/openstack-k8s-operators/lightspeed-operator/api/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestBuildLCoreQuotaHandlersConfig_DisabledWhenNoLimiters(t *testing.T) {
	h := newTestHelper(t)

	instance := &apiv1beta1.OpenStackLightspeed{}
	if got := buildLCoreQuotaHandlersConfig(h, instance); got != nil {
		t.Errorf("expected nil config when Quotas is nil, got %v", got)
	}

	instance.Spec.Quotas = &apiv1beta1.QuotaSpec{}
	if got := buildLCoreQuotaHandlersConfig(h, instance); got != nil {
		t.Errorf("expected nil config when Limiters is empty, got %v", got)
	}
}

func TestBuildLCoreQuotaHandlersConfig_PostgresBlock(t *testing.T) {
	h := newTestHelper(t)

	instance := &apiv1beta1.OpenStackLightspeed{
		Spec: apiv1beta1.OpenStackLightspeedSpec{
			Quotas: &apiv1beta1.QuotaSpec{
				Limiters: []apiv1beta1.QuotaLimiterSpec{
					{Name: "per-user-hourly", Type: "userLimiter", InitialQuota: 1000, QuotaIncrease: 1000, Period: "1 hour"},
				},
			},
		},
	}

	config := buildLCoreQuotaHandlersConfig(h, instance)
	if config == nil {
		t.Fatal("expected non-nil config")
	}

	postgres, ok := config["postgres"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected postgres block, got %v", config["postgres"])
	}

	wantHost := PostgresServiceName + "." + h.GetBeforeObject().GetNamespace() + ".svc"
	assertField(t, postgres, "host", wantHost)
	assertField(t, postgres, "port", PostgresServicePort)
	assertField(t, postgres, "db", PostgresLightspeedStackDbName)
	assertField(t, postgres, "user", "${env.POSTGRESQL_USER}")
	assertField(t, postgres, "password", "${env.POSTGRESQL_PASSWORD}")
	assertField(t, postgres, "ssl_mode", PostgresDefaultSSLMode)
	assertField(t, postgres, "gss_encmode", "disable")
	assertField(t, postgres, "ca_cert_path", CABundleMountPath)
	assertField(t, postgres, "namespace", "quota_handlers")
}

func TestBuildLCoreQuotaHandlersConfig_LimiterTypeMapping(t *testing.T) {
	h := newTestHelper(t)

	instance := &apiv1beta1.OpenStackLightspeed{
		Spec: apiv1beta1.OpenStackLightspeedSpec{
			Quotas: &apiv1beta1.QuotaSpec{
				Limiters: []apiv1beta1.QuotaLimiterSpec{
					{Name: "per-user-hourly", Type: "userLimiter", InitialQuota: 1000, QuotaIncrease: 1000, Period: "1 hour"},
					{Name: "cluster-daily", Type: "clusterLimiter", InitialQuota: 100000, QuotaIncrease: 100000, Period: "1 day"},
				},
			},
		},
	}

	config := buildLCoreQuotaHandlersConfig(h, instance)
	limiters, ok := config["limiters"].([]interface{})
	if !ok || len(limiters) != 2 {
		t.Fatalf("expected 2 limiters, got %v", config["limiters"])
	}

	first := limiters[0].(map[string]interface{})
	assertField(t, first, "name", "per-user-hourly")
	assertField(t, first, "type", "user_limiter")
	assertField(t, first, "initial_quota", 1000)
	assertField(t, first, "quota_increase", 1000)
	assertField(t, first, "period", "1 hour")

	second := limiters[1].(map[string]interface{})
	assertField(t, second, "name", "cluster-daily")
	assertField(t, second, "type", "cluster_limiter")
	assertField(t, second, "initial_quota", 100000)
	assertField(t, second, "quota_increase", 100000)
	assertField(t, second, "period", "1 day")
}

func TestBuildLCoreQuotaHandlersConfig_SchedulerDefaults(t *testing.T) {
	h := newTestHelper(t)

	instance := &apiv1beta1.OpenStackLightspeed{
		Spec: apiv1beta1.OpenStackLightspeedSpec{
			Quotas: &apiv1beta1.QuotaSpec{
				Limiters: []apiv1beta1.QuotaLimiterSpec{
					{Name: "per-user-hourly", Type: "userLimiter", InitialQuota: 1000, QuotaIncrease: 1000, Period: "1 hour"},
				},
			},
		},
	}

	config := buildLCoreQuotaHandlersConfig(h, instance)
	scheduler, ok := config["scheduler"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected scheduler block, got %v", config["scheduler"])
	}

	assertField(t, scheduler, "period", 5)
	assertField(t, scheduler, "database_reconnection_count", 10)
	assertField(t, scheduler, "database_reconnection_delay", 1)
}

func TestBuildLCoreQuotaHandlersConfig_SchedulerOverride(t *testing.T) {
	h := newTestHelper(t)

	instance := &apiv1beta1.OpenStackLightspeed{
		Spec: apiv1beta1.OpenStackLightspeedSpec{
			Quotas: &apiv1beta1.QuotaSpec{
				Limiters: []apiv1beta1.QuotaLimiterSpec{
					{Name: "per-user-hourly", Type: "userLimiter", InitialQuota: 1000, QuotaIncrease: 1000, Period: "1 hour"},
				},
				Scheduler: &apiv1beta1.QuotaSchedulerSpec{
					Period:                    30,
					DatabaseReconnectionCount: 3,
					DatabaseReconnectionDelay: 2,
				},
			},
		},
	}

	config := buildLCoreQuotaHandlersConfig(h, instance)
	scheduler := config["scheduler"].(map[string]interface{})

	assertField(t, scheduler, "period", 30)
	assertField(t, scheduler, "database_reconnection_count", 3)
	assertField(t, scheduler, "database_reconnection_delay", 2)
}

func TestBuildLCoreQuotaHandlersConfig_EnableTokenHistory(t *testing.T) {
	h := newTestHelper(t)

	for _, want := range []bool{true, false} {
		instance := &apiv1beta1.OpenStackLightspeed{
			Spec: apiv1beta1.OpenStackLightspeedSpec{
				Quotas: &apiv1beta1.QuotaSpec{
					Limiters: []apiv1beta1.QuotaLimiterSpec{
						{Name: "per-user-hourly", Type: "userLimiter", InitialQuota: 1000, QuotaIncrease: 1000, Period: "1 hour"},
					},
					EnableTokenHistory: want,
				},
			},
		}

		config := buildLCoreQuotaHandlersConfig(h, instance)
		assertField(t, config, "enable_token_history", want)
	}
}

func assertField(t *testing.T, m map[string]interface{}, key string, want interface{}) {
	t.Helper()
	got := m[key]
	if got != want {
		t.Errorf("%s: got %v (%T), want %v (%T)", key, got, got, want, want)
	}
}

func instanceWithDevConfig(t *testing.T, dev map[string]interface{}) *apiv1beta1.OpenStackLightspeed {
	t.Helper()
	devRaw, err := json.Marshal(dev)
	if err != nil {
		t.Fatalf("failed to marshal dev config: %v", err)
	}
	return &apiv1beta1.OpenStackLightspeed{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: apiv1beta1.OpenStackLightspeedSpec{
			Dev: runtime.RawExtension{Raw: devRaw},
		},
	}
}

func TestBuildAdditionalMCPServersConfig_Empty(t *testing.T) {
	servers := buildAdditionalMCPServersConfig(nil)
	if len(servers) != 0 {
		t.Errorf("expected no servers, got %d", len(servers))
	}
}

func TestBuildAdditionalMCPServersConfig_WithAuthHeaders(t *testing.T) {
	servers := buildAdditionalMCPServersConfig([]apiv1beta1.MCPServerEntry{
		{
			Name:                 "korrel8r",
			URL:                  "https://korrel8r.openshift-cluster-observability-operator.svc:9443/mcp",
			AuthorizationHeaders: map[string]string{"Authorization": "kubernetes"},
		},
	})

	if len(servers) != 1 {
		t.Fatalf("expected 1 server, got %d", len(servers))
	}

	server := servers[0].(map[string]interface{})
	if server["name"] != "korrel8r" {
		t.Errorf("expected name 'korrel8r', got '%v'", server["name"])
	}
	headers, ok := server["authorization_headers"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected authorization_headers map, got %T", server["authorization_headers"])
	}
	if headers["Authorization"] != "kubernetes" {
		t.Errorf("expected Authorization header 'kubernetes', got '%v'", headers["Authorization"])
	}
}

func TestBuildAdditionalMCPServersConfig_NoAuthHeaders(t *testing.T) {
	servers := buildAdditionalMCPServersConfig([]apiv1beta1.MCPServerEntry{
		{Name: "obs-mcp", URL: "http://obs-mcp.obs-mcp.svc:9100/mcp"},
	})

	server := servers[0].(map[string]interface{})
	if _, present := server["authorization_headers"]; present {
		t.Errorf("expected no authorization_headers key when unset, got %v", server["authorization_headers"])
	}
}

func TestBuildLCoreMCPServersConfigIfEnabled_AdditionalServersWithoutRhosoMCP(t *testing.T) {
	instance := instanceWithDevConfig(t, map[string]interface{}{
		"additionalMCPServers": []map[string]interface{}{
			{"name": "korrel8r", "url": "https://korrel8r.example.svc:9443/mcp"},
		},
	})

	servers, err := buildLCoreMCPServersConfigIfEnabled(instance)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(servers) != 1 {
		t.Fatalf("expected 1 server (rhoso_mcps not enabled), got %d", len(servers))
	}
	if servers[0].(map[string]interface{})["name"] != "korrel8r" {
		t.Errorf("expected additional server to be present without rhoso_mcps enabled")
	}
}

func TestBuildLCoreMCPServersConfigIfEnabled_CombinesRhosoMCPAndAdditional(t *testing.T) {
	instance := instanceWithDevConfig(t, map[string]interface{}{
		"featureFlags": []string{"rhoso_mcps"},
		"additionalMCPServers": []map[string]interface{}{
			{"name": "korrel8r", "url": "https://korrel8r.example.svc:9443/mcp"},
		},
	})

	servers, err := buildLCoreMCPServersConfigIfEnabled(instance)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(servers) != 2 {
		t.Fatalf("expected 2 servers (rhoso-ocp-tools + korrel8r), got %d", len(servers))
	}
	if servers[0].(map[string]interface{})["name"] != "rhoso-ocp-tools" {
		t.Errorf("expected rhoso_mcps entries first, got %v", servers[0])
	}
	if servers[1].(map[string]interface{})["name"] != "korrel8r" {
		t.Errorf("expected additional server appended after rhoso_mcps entries, got %v", servers[1])
	}
}

func TestBuildLCoreMCPServersConfigIfEnabled_NoneConfigured(t *testing.T) {
	instance := &apiv1beta1.OpenStackLightspeed{}

	servers, err := buildLCoreMCPServersConfigIfEnabled(instance)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(servers) != 0 {
		t.Errorf("expected no servers when neither rhoso_mcps nor additionalMCPServers is set, got %d", len(servers))
	}
}
