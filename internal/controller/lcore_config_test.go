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
	"testing"

	apiv1beta1 "github.com/openstack-k8s-operators/lightspeed-operator/api/v1beta1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/yaml"
)

func TestBuildLCoreConfigYAML_ResponsesRequestMaxSize(t *testing.T) {
	for _, tc := range []struct {
		name  string
		lcore *apiv1beta1.LCoreSpec
		want  *int
	}{
		{name: "lcore omitted"},
		{name: "limit omitted", lcore: &apiv1beta1.LCoreSpec{}},
		{name: "minimum limit", lcore: &apiv1beta1.LCoreSpec{ResponsesRequestMaxSize: ptr.To(1)}, want: ptr.To(1)},
		{name: "increased limit", lcore: &apiv1beta1.LCoreSpec{ResponsesRequestMaxSize: ptr.To(262144)}, want: ptr.To(262144)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			instance := &apiv1beta1.OpenStackLightspeed{}
			instance.Spec.LCore = tc.lcore
			data, err := buildLCoreConfigYAML(context.Background(), newTestHelper(t), instance)
			if err != nil {
				t.Fatalf("build LCore configuration: %v", err)
			}
			var config struct {
				Customization struct {
					ResponsesRequestMaxSize  *int   `json:"responses_request_max_size"`
					SystemPrompt             string `json:"system_prompt"`
					DisableQuerySystemPrompt bool   `json:"disable_query_system_prompt"`
				} `json:"customization"`
			}
			if err := yaml.Unmarshal([]byte(data), &config); err != nil {
				t.Fatalf("parse LCore configuration: %v", err)
			}
			got := config.Customization.ResponsesRequestMaxSize
			if !ptr.Equal(got, tc.want) {
				t.Errorf("responses_request_max_size = %v, want %v", got, tc.want)
			}
			if config.Customization.SystemPrompt != getSystemPrompt() || !config.Customization.DisableQuerySystemPrompt {
				t.Error("expected the operator system prompt and query system prompt protection")
			}
		})
	}
}

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

func TestBuildLCoreRetrievalConfig(t *testing.T) {
	config := buildLCoreRetrievalConfig(&apiv1beta1.OpenStackLightspeed{})

	inline, hasInline := config["inline"].(map[string]interface{})
	if !hasInline {
		t.Fatalf("expected inline retrieval, got %v", config)
	}
	inlineSources, ok := inline["sources"].([]interface{})
	if !ok || len(inlineSources) != 0 {
		t.Errorf("inline sources = %v, want []", inline["sources"])
	}

	tool, hasTool := config["tool"].(map[string]interface{})
	if !hasTool {
		t.Fatalf("expected tool retrieval, got %v", config)
	}
	sources, ok := tool["sources"].([]interface{})
	if !ok || len(sources) != 1 || sources[0] != "okp" {
		t.Errorf("tool sources = %v, want [okp]", tool["sources"])
	}
}

func TestBuildLCoreRAGConfig_AlwaysIncludesOKP(t *testing.T) {
	h := newTestHelper(t)
	instance := &apiv1beta1.OpenStackLightspeed{}

	rag := buildLCoreRAGConfig(context.Background(), h, instance)
	if _, hasOKP := rag["okp"]; !hasOKP {
		t.Errorf("expected rag.okp to always be present, got %v", rag)
	}
	retrieval := rag["retrieval"].(map[string]interface{})
	if _, hasTool := retrieval["tool"]; !hasTool {
		t.Errorf("expected rag.retrieval.tool, got %v", retrieval)
	}
	inline, hasInline := retrieval["inline"].(map[string]interface{})
	if !hasInline {
		t.Fatalf("expected rag.retrieval.inline, got %v", retrieval)
	}
	inlineSources, ok := inline["sources"].([]interface{})
	if !ok || len(inlineSources) != 0 {
		t.Errorf("inline sources = %v, want []", inline["sources"])
	}
}

func assertField(t *testing.T, m map[string]interface{}, key string, want interface{}) {
	t.Helper()
	got := m[key]
	if got != want {
		t.Errorf("%s: got %v (%T), want %v (%T)", key, got, got, want, want)
	}
}
