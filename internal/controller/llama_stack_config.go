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
	"fmt"
	"strings"

	common_helper "github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	apiv1beta1 "github.com/openstack-k8s-operators/lightspeed-operator/api/v1beta1"
	"sigs.k8s.io/yaml"
)

func buildLlamaStackCoreConfig(_ *common_helper.Helper, _ *apiv1beta1.OpenStackLightspeed) map[string]interface{} {
	return map[string]interface{}{
		"version": "2",

		// image_name is a semantic identifier for the llama-stack configuration
		// Note: Does NOT affect PostgreSQL database name (llama-stack uses hardcoded "llamastack")
		"image_name": "openstack-lightspeed-configuration",

		// Minimal APIs for RAG + MCP: agents (for MCP), files, inference, safety (required by agents),
		// telemetry, tool_runtime, vector_io.
		"apis": []string{
			"agents",
			"files",
			"inference",
			"safety",
			"tool_runtime",
			"vector_io",
		},
		"benchmarks":             []interface{}{},
		"container_image":        nil,
		"datasets":               []interface{}{},
		"external_providers_dir": nil,
		"inference_store": map[string]interface{}{
			"db_path": ".llama/distributions/ollama/inference_store.db",
			"type":    "sqlite",
		},
		"logging": nil,
		"metadata_store": map[string]interface{}{
			"db_path":   "/tmp/llama-stack/registry.db",
			"namespace": nil,
			"type":      "sqlite",
		},
	}
}

func buildLlamaStackFileProviders(_ *common_helper.Helper, _ *apiv1beta1.OpenStackLightspeed) []interface{} {
	return []interface{}{
		map[string]interface{}{
			"provider_id":   "localfs",
			"provider_type": "inline::localfs",
			"config": map[string]interface{}{
				"storage_dir": "/tmp/llama-stack-files",
				"metadata_store": map[string]interface{}{
					"backend":    "sql_default",
					"namespace":  "files_metadata",
					"table_name": "files_metadata",
				},
			},
		},
	}
}

func buildLlamaStackAgentProviders(_ *common_helper.Helper, _ *apiv1beta1.OpenStackLightspeed) []interface{} {
	return []interface{}{
		map[string]interface{}{
			"provider_id":   "meta-reference",
			"provider_type": "inline::meta-reference",
			"config": map[string]interface{}{
				"persistence": map[string]interface{}{
					"agent_state": map[string]interface{}{
						"backend":    "kv_default",
						"table_name": "agent_state",
						"namespace":  "agent_state",
					},
					"responses": map[string]interface{}{
						"backend":    "sql_default",
						"table_name": "agent_responses",
						"namespace":  "agent_responses",
					},
				},
			},
		},
	}
}

func buildLlamaStackInferenceProviders(_ *common_helper.Helper, _ context.Context, instance *apiv1beta1.OpenStackLightspeed) ([]interface{}, error) {
	// Always include sentence-transformers for embeddings
	providers := []interface{}{
		map[string]interface{}{
			"provider_id":   "sentence-transformers",
			"provider_type": "inline::sentence-transformers",
			"config":        map[string]interface{}{},
		},
	}

	// Add the LLM provider from the instance spec
	{
		provider := buildProvider(instance)
		providerConfig := map[string]interface{}{
			"provider_id": provider.Name,
		}

		// Convert provider name to valid environment variable name
		envVarName := providerNameToEnvVarName(provider.Name)

		// Map provider types to Llama Stack provider types
		switch provider.Type {
		case OpenAIProviderName, GeminiProviderName, RHOAIVLLMProviderName, RHELAIVLLMProviderName:
			config := map[string]interface{}{}
			// Determine the appropriate Llama Stack provider type:
			//  - OpenAI uses remote::openai
			//  - vLLM uses remote::vllm
			var apiKeyField string
			switch provider.Type {
			case OpenAIProviderName:
				providerConfig["provider_type"] = "remote::openai"
				apiKeyField = "api_key"
			case GeminiProviderName:
				providerConfig["provider_type"] = "remote::gemini"
				apiKeyField = "api_key"
			default:
				providerConfig["provider_type"] = "remote::vllm"
				apiKeyField = "api_token"
			}
			// Llama Stack will substitute ${env.VAR_NAME} with the actual env var value
			config[apiKeyField] = fmt.Sprintf("${env.%s%s}", envVarName, EnvVarSuffixAPIKey)

			// Add custom URL if specified
			if provider.URL != "" {
				config["base_url"] = provider.URL
			}

			providerConfig["config"] = config

		case AzureOpenAIProviderName:
			providerConfig["provider_type"] = "remote::azure"
			config := map[string]interface{}{}

			// Azure supports both API key and client credentials authentication
			// Always include api_key (required by LiteLLM's Pydantic validation)
			config["api_key"] = fmt.Sprintf("${env.%s_API_KEY}", envVarName)

			// Also include client credentials fields (will be empty if not using client credentials)
			config["client_id"] = fmt.Sprintf("${env.%s_CLIENT_ID:=}", envVarName)
			config["tenant_id"] = fmt.Sprintf("${env.%s_TENANT_ID:=}", envVarName)
			config["client_secret"] = fmt.Sprintf("${env.%s_CLIENT_SECRET:=}", envVarName)

			// Azure-specific fields
			if provider.AzureDeploymentName != "" {
				config["deployment_name"] = provider.AzureDeploymentName
			}
			if provider.APIVersion != "" {
				config["api_version"] = provider.APIVersion
			}
			if provider.URL != "" {
				config["base_url"] = provider.URL
			}
			providerConfig["config"] = config

		case WatsonXProviderName:
			providerConfig["provider_type"] = "remote::watsonx"

			config := map[string]interface{}{}
			config["base_url"] = provider.URL
			config["api_key"] = fmt.Sprintf("${env.%s_API_KEY}", envVarName)

			if provider.WatsonProjectID != "" {
				config["project_id"] = provider.WatsonProjectID
			}

			providerConfig["config"] = config

		default:
			supportedProviders := []string{
				OpenAIProviderName, GeminiProviderName, RHOAIVLLMProviderName, RHELAIVLLMProviderName,
				AzureOpenAIProviderName, WatsonXProviderName,
			}
			return nil, fmt.Errorf(
				"unknown provider type '%s' (provider '%s'). Supported types: %s",
				provider.Type,
				provider.Name,
				strings.Join(supportedProviders, ","),
			)
		}

		providers = append(providers, providerConfig)
	}

	return providers, nil
}

// Safety API - Required by agents provider (for MCP)
func buildLlamaStackSafety(_ *common_helper.Helper, _ *apiv1beta1.OpenStackLightspeed) []interface{} {
	return []interface{}{
		map[string]interface{}{
			"provider_id":   "llama-guard",
			"provider_type": "inline::llama-guard",
			"config": map[string]interface{}{
				"excluded_categories": []interface{}{},
			},
		},
	}
}

func buildLlamaStackToolRuntime(_ *common_helper.Helper, _ *apiv1beta1.OpenStackLightspeed) []interface{} {
	return []interface{}{
		map[string]interface{}{
			"provider_id":   "model-context-protocol",
			"provider_type": "remote::model-context-protocol",
			"config":        map[string]interface{}{},
		},
		map[string]interface{}{
			"provider_id":   "rag-runtime",
			"provider_type": "inline::rag-runtime",
			"config":        map[string]interface{}{},
		},
	}
}

func buildLlamaStackVectorDB(_ *common_helper.Helper, _ *apiv1beta1.OpenStackLightspeed) []interface{} {
	return []interface{}{
		map[string]interface{}{
			"provider_id":   "faiss",
			"provider_type": "inline::faiss",
			"config": map[string]interface{}{
				"kvstore": map[string]interface{}{
					"backend":    "sql_default",
					"table_name": "vector_store",
				},
				"persistence": map[string]interface{}{
					"backend":   "kv_default",
					"namespace": "vector_persistence",
				},
			},
		},
	}
}

func buildLlamaStackVectorIO(h *common_helper.Helper, instance *apiv1beta1.OpenStackLightspeed, chunkFilterQuery string) []interface{} {
	providers := buildLlamaStackVectorDB(h, instance)
	if isOKPEnabled(instance) {
		providers = append(providers, buildOKPVectorIOProvider(chunkFilterQuery))
	}
	return providers
}

func buildOKPVectorIOProvider(chunkFilterQuery string) map[string]interface{} {
	chunkFilterQuery = "is_chunk:true AND " + chunkFilterQuery

	return map[string]interface{}{
		"provider_id":   "okp_solr",
		"provider_type": "remote::solr_vector_io",
		"config": map[string]interface{}{
			"solr_url":            "${env.RH_SERVER_OKP}/solr",
			"collection_name":     "${env.SOLR_COLLECTION:=portal-rag}",
			"content_field":       "${env.SOLR_CONTENT_FIELD:=chunk}",
			"vector_field":        "${env.SOLR_VECTOR_FIELD:=chunk_vector}",
			"embedding_dimension": "${env.SOLR_EMBEDDING_DIM:=384}",
			"embedding_model":     "sentence-transformers/" + OKPEmbeddingModelMountPath,
			"persistence": map[string]interface{}{
				"backend":   "kv_default",
				"namespace": "portal-rag",
			},
			"chunk_window_config": map[string]interface{}{
				"chunk_content_field":           "chunk_field",
				"chunk_index_field":             "chunk_index",
				"chunk_filter_query":            chunkFilterQuery,
				"chunk_parent_id_field":         "parent_id",
				"chunk_source_path_field":       "source_path",
				"chunk_online_source_url_field": "online_source_url",
				"chunk_token_count_field":       "num_tokens",
				"parent_total_chunks_field":     "total_chunks",
				"parent_total_tokens_field":     "total_tokens",
				"chunk_family_fields":           []interface{}{"headings"},
			},
		},
	}
}

func buildLlamaStackServerConfig(_ *common_helper.Helper, _ *apiv1beta1.OpenStackLightspeed) map[string]interface{} {
	return map[string]interface{}{
		"auth":         nil,
		"host":         "0.0.0.0", // Listen on all interfaces so lightspeed-stack container can connect
		"port":         LlamaStackContainerPort,
		"quota":        nil,
		"tls_cafile":   nil,
		"tls_certfile": nil,
		"tls_keyfile":  nil,
	}
}

// buildLlamaStackStorage configures persistent storage for Llama Stack
func buildLlamaStackStorage(_ *common_helper.Helper, instance *apiv1beta1.OpenStackLightspeed) map[string]interface{} {
	// Define storage backends - SQL only
	backends := map[string]interface{}{
		"sql_default": map[string]interface{}{
			"type":    "sql_sqlite",
			"db_path": "/tmp/llama-stack/sql_store.db",
		},
		"kv_default": map[string]interface{}{
			"type":    "kv_sqlite",
			"db_path": "/tmp/llama-stack/kv_store.db",
		},
		"postgres_backend": map[string]interface{}{
			"type":     "sql_postgres",
			"host":     fmt.Sprintf("lightspeed-postgres-server.%s.svc", instance.GetNamespace()),
			"port":     PostgresServicePort,
			"user":     "postgres",
			"password": "${env.POSTGRES_PASSWORD}",
			// Note: Database name is HARDCODED to "llamastack" in llama-stack's postgres adapter
			// Not configurable - llama-stack ignores image_name for database selection
		},
	}

	// Map data stores to backends - all use SQL with table_name
	stores := map[string]interface{}{
		"metadata": map[string]interface{}{
			"namespace": "registry",
			"backend":   "kv_default",
		},
		"inference": map[string]interface{}{
			"table_name": "inference_store",
			"backend":    "sql_default",
		},
		"conversations": map[string]interface{}{
			"table_name": "openai_conversations", // Required by config schema but ignored - llama-stack uses hardcoded names
			"backend":    "postgres_backend",
		},
	}

	return map[string]interface{}{
		"backends": backends,
		"stores":   stores,
	}
}

func buildLlamaStackModels(_ *common_helper.Helper, instance *apiv1beta1.OpenStackLightspeed) []interface{} {
	models := []interface{}{}
	// Add LLM models from the instance spec
	{
		provider := buildProvider(instance)
		for _, model := range provider.Models {
			modelConfig := map[string]interface{}{
				"model_id":          model.Name,
				"model_type":        "llm",
				"provider_id":       provider.Name,
				"provider_model_id": model.Name,
			}

			// Add model-specific metadata if available
			metadata := map[string]interface{}{}
			if model.MaxTokensForResponse > 0 {
				metadata["max_tokens"] = model.MaxTokensForResponse
			}
			if len(metadata) > 0 {
				modelConfig["metadata"] = metadata
			}

			models = append(models, modelConfig)
		}
	}

	if isOKPEnabled(instance) {
		models = append(models, map[string]interface{}{
			"model_id":          "solr_embedding",
			"model_type":        "embedding",
			"provider_id":       "sentence-transformers",
			"provider_model_id": OKPEmbeddingModelMountPath,
			"metadata": map[string]interface{}{
				"embedding_dimension": 384,
			},
		})
	}

	return models
}

func buildLlamaStackVectorStores(_ *common_helper.Helper, instance *apiv1beta1.OpenStackLightspeed) []interface{} {
	stores := []interface{}{}
	if isOKPEnabled(instance) {
		stores = append(stores, map[string]interface{}{
			"vector_store_id":     "portal-rag",
			"provider_id":         "okp_solr",
			"embedding_dimension": 384,
			"embedding_model":     "sentence-transformers/" + OKPEmbeddingModelMountPath,
		})
	}
	return stores
}

func buildLlamaStackToolGroups(_ *common_helper.Helper, _ *apiv1beta1.OpenStackLightspeed) []interface{} {
	return []interface{}{
		map[string]interface{}{
			"toolgroup_id": "builtin::rag",
			"provider_id":  "rag-runtime",
		},
	}
}

// buildLlamaStackYAML assembles the complete Llama Stack configuration and converts to YAML
func buildLlamaStackYAML(h *common_helper.Helper, ctx context.Context, instance *apiv1beta1.OpenStackLightspeed) (string, error) {
	// Build the complete config as a map
	config := buildLlamaStackCoreConfig(h, instance)

	// Build inference providers with error handling
	inferenceProviders, err := buildLlamaStackInferenceProviders(h, ctx, instance)
	if err != nil {
		return "", fmt.Errorf("failed to build inference providers: %w", err)
	}

	okpChunkFilterQuery := ""
	if isOKPEnabled(instance) {
		config["external_providers_dir"] = ExternalProvidersDir
		okpChunkFilterQuery = getOKPChunkFilterQuery(ctx, h, instance)
	}

	// Build providers map - only include providers for enabled APIs
	config["providers"] = map[string]interface{}{
		"files":        buildLlamaStackFileProviders(h, instance),
		"agents":       buildLlamaStackAgentProviders(h, instance),
		"inference":    inferenceProviders,
		"safety":       buildLlamaStackSafety(h, instance),
		"tool_runtime": buildLlamaStackToolRuntime(h, instance),
		"vector_io":    buildLlamaStackVectorIO(h, instance, okpChunkFilterQuery),
	}

	// Add top-level fields
	config["scoring_fns"] = []interface{}{}
	config["server"] = buildLlamaStackServerConfig(h, instance)
	config["storage"] = buildLlamaStackStorage(h, instance)
	config["telemetry"] = map[string]interface{}{
		"enabled": false,
	}
	config["registered_resources"] = map[string][]interface{}{
		"models":        buildLlamaStackModels(h, instance),
		"vector_stores": buildLlamaStackVectorStores(h, instance),
		"tool_groups":   buildLlamaStackToolGroups(h, instance),
	}

	// Convert to YAML
	yamlBytes, err := yaml.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("failed to marshal Llama Stack config to YAML: %w", err)
	}

	return string(yamlBytes), nil
}
