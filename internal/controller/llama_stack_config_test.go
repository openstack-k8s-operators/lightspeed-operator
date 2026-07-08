package controller

import (
	"context"
	"fmt"

	apiv1beta1 "github.com/openstack-k8s-operators/lightspeed-operator/api/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func expectSentenceTransformersProvider(providers []interface{}) {
	sentenceTransformers := providers[0].(map[string]interface{})
	Expect(sentenceTransformers["provider_id"]).To(Equal("sentence-transformers"))
	Expect(sentenceTransformers["provider_type"]).To(Equal("inline::sentence-transformers"))
}

func getOpenStackLightspeedProvidersInstance(provider string) *apiv1beta1.OpenStackLightspeed {
	instance := &apiv1beta1.OpenStackLightspeed{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "openstack-lightspeed",
			Namespace: "openstack-lightspeed",
		},
	}

	switch provider {
	case OpenAIProviderName:
		instance.Spec.LLMEndpointType = OpenAIProviderName
		instance.Spec.LLMEndpoint = "https://api.openai.com/v1"
		instance.Spec.ModelName = "gpt-4o"
		return instance
	case GeminiProviderName:
		instance.Spec.LLMEndpointType = GeminiProviderName
		instance.Spec.ModelName = "gemini-2.0-flash"
		return instance
	case RHOAIVLLMProviderName:
		instance.Spec.LLMEndpointType = RHOAIVLLMProviderName
		instance.Spec.LLMEndpoint = "https://vllm.example.com/v1"
		instance.Spec.ModelName = "meta-llama/Llama-3.1-70B-Instruct"
		return instance
	case RHELAIVLLMProviderName:
		instance.Spec.LLMEndpointType = RHELAIVLLMProviderName
		instance.Spec.LLMEndpoint = "https://rhelai-vllm.example.com/v1"
		instance.Spec.ModelName = "meta-llama/Llama-3.1-70B-Instruct"
		return instance
	case AzureOpenAIProviderName:
		instance.Spec.LLMEndpointType = AzureOpenAIProviderName
		instance.Spec.LLMEndpoint = "https://my-resource.openai.azure.com"
		instance.Spec.LLMDeploymentName = "gpt-4o-deployment"
		instance.Spec.LLMAPIVersion = "2024-02-01"
		instance.Spec.ModelName = "gpt-4o"
		return instance
	case WatsonXProviderName:
		instance.Spec.LLMEndpointType = WatsonXProviderName
		instance.Spec.LLMEndpoint = "https://watsonx.example.com"
		instance.Spec.LLMProjectID = "test-project-id"
		instance.Spec.ModelName = "ibm/granite-13b-chat-v2"
		return instance
	default:
		Fail(fmt.Sprintf("Unknown provider %s", provider))
	}

	return nil
}

func checkModelCommonConfig(modelConfig map[string]interface{}, instance *apiv1beta1.OpenStackLightspeed) {
	Expect(modelConfig["model_id"]).To(Equal(instance.Spec.ModelName))
	Expect(modelConfig["model_type"]).To(Equal("llm"))
	Expect(modelConfig["provider_id"]).To(Equal(OpenStackLightspeedDefaultProvider))
	Expect(modelConfig["provider_model_id"]).To(Equal(instance.Spec.ModelName))
	Expect(modelConfig).NotTo(HaveKey("metadata"))
}

var _ = Describe("Llama Stack config", func() {
	Describe("buildLlamaStackInferenceProviders", func() {
		DescribeTable("should return correct inference providers config",
			func(provider, providerType string, checkConfig func(map[string]interface{}, *apiv1beta1.OpenStackLightspeed)) {
				instance := getOpenStackLightspeedProvidersInstance(provider)
				inferenceProvidersConfig, err := buildLlamaStackInferenceProviders(nil, context.Background(), instance)

				Expect(err).NotTo(HaveOccurred())
				Expect(inferenceProvidersConfig).To(HaveLen(2))

				expectSentenceTransformersProvider(inferenceProvidersConfig)

				inferenceProvider := inferenceProvidersConfig[1].(map[string]interface{})
				Expect(inferenceProvider["provider_id"]).To(Equal(OpenStackLightspeedDefaultProvider))
				Expect(inferenceProvider["provider_type"]).To(Equal(providerType))

				checkConfig(inferenceProvider["config"].(map[string]interface{}), instance)
			},
			Entry("for openai", OpenAIProviderName, "remote::openai",
				func(config map[string]interface{}, _ *apiv1beta1.OpenStackLightspeed) {
					Expect(config["api_key"]).To(Equal("${env.OPENSTACK_LIGHTSPEED_PROVIDER_API_KEY}"))
				}),
			Entry("for gemini", GeminiProviderName, "remote::gemini",
				func(config map[string]interface{}, _ *apiv1beta1.OpenStackLightspeed) {
					Expect(config["api_key"]).To(Equal("${env.OPENSTACK_LIGHTSPEED_PROVIDER_API_KEY}"))
					Expect(config).NotTo(HaveKey("base_url"))
				}),
			Entry("for rhoai_vllm", RHOAIVLLMProviderName, "remote::vllm",
				func(config map[string]interface{}, instance *apiv1beta1.OpenStackLightspeed) {
					Expect(config["api_token"]).To(Equal("${env.OPENSTACK_LIGHTSPEED_PROVIDER_API_KEY}"))
					Expect(config["base_url"]).To(Equal(instance.Spec.LLMEndpoint))
				}),
			Entry("for rhelai_vllm", RHELAIVLLMProviderName, "remote::vllm",
				func(config map[string]interface{}, instance *apiv1beta1.OpenStackLightspeed) {
					Expect(config["api_token"]).To(Equal("${env.OPENSTACK_LIGHTSPEED_PROVIDER_API_KEY}"))
					Expect(config["base_url"]).To(Equal(instance.Spec.LLMEndpoint))
				}),
			Entry("for azure_openai", AzureOpenAIProviderName, "remote::azure",
				func(config map[string]interface{}, instance *apiv1beta1.OpenStackLightspeed) {
					Expect(config["api_key"]).To(Equal("${env.OPENSTACK_LIGHTSPEED_PROVIDER_API_KEY}"))
					Expect(config["client_id"]).To(Equal("${env.OPENSTACK_LIGHTSPEED_PROVIDER_CLIENT_ID:=}"))
					Expect(config["tenant_id"]).To(Equal("${env.OPENSTACK_LIGHTSPEED_PROVIDER_TENANT_ID:=}"))
					Expect(config["client_secret"]).To(Equal("${env.OPENSTACK_LIGHTSPEED_PROVIDER_CLIENT_SECRET:=}"))
					Expect(config["base_url"]).To(Equal(instance.Spec.LLMEndpoint))
					Expect(config["deployment_name"]).To(Equal(instance.Spec.LLMDeploymentName))
					Expect(config["api_version"]).To(Equal(instance.Spec.LLMAPIVersion))
				}),
			Entry("for watsonx", WatsonXProviderName, "remote::watsonx",
				func(config map[string]interface{}, instance *apiv1beta1.OpenStackLightspeed) {
					Expect(config["base_url"]).To(Equal(instance.Spec.LLMEndpoint))
					Expect(config["project_id"]).To(Equal(instance.Spec.LLMProjectID))
					Expect(config["api_key"]).To(Equal("${env.OPENSTACK_LIGHTSPEED_PROVIDER_API_KEY}"))
				}),
		)
	})

	Describe("buildLlamaStackModels", func() {
		DescribeTable("should return correct models config",
			func(provider string) {
				instance := getOpenStackLightspeedProvidersInstance(provider)
				modelsConfig := buildLlamaStackModels(nil, instance)

				Expect(modelsConfig).To(HaveLen(2))

				modelConfig := modelsConfig[0].(map[string]interface{})
				checkModelCommonConfig(modelConfig, instance)

				okpModel := modelsConfig[1].(map[string]interface{})
				Expect(okpModel["model_id"]).To(Equal("solr_embedding"))
				Expect(okpModel["model_type"]).To(Equal("embedding"))
				Expect(okpModel["provider_id"]).To(Equal("sentence-transformers"))
			},
			Entry("for openai", OpenAIProviderName),
			Entry("for gemini", GeminiProviderName),
			Entry("for rhoai_vllm", RHOAIVLLMProviderName),
			Entry("for rhelai_vllm", RHELAIVLLMProviderName),
			Entry("for azure_openai", AzureOpenAIProviderName),
			Entry("for watsonx", WatsonXProviderName),
		)
	})
})
