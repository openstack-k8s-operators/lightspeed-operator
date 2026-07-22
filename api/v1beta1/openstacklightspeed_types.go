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

package v1beta1

import (
	"reflect"

	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/util"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	// OpenStackLightspeedContainerImage is the fall-back container image for OpenStackLightspeed
	OpenStackLightspeedContainerImage = "quay.io/openstack-lightspeed/rag-content:os-docs-2026.1-ogx"

	// LCoreContainerImage is the fall-back container image for LCore
	LCoreContainerImage = "quay.io/lightspeed-core/lightspeed-stack:dev-latest"

	// ExporterContainerImage is the fall-back container image for the Dataverse Exporter
	ExporterContainerImage = "quay.io/lightspeed-core/lightspeed-to-dataverse-exporter:latest"

	// PostgresContainerImage is the fall-back container image for PostgreSQL
	PostgresContainerImage = "registry.redhat.io/rhel9/postgresql-16:latest"

	// ConsoleContainerImage is the fall-back container image for the Console Plugin (PatternFly 6, OCP >= 4.19)
	ConsoleContainerImage = "registry.redhat.io/openshift-lightspeed/lightspeed-console-plugin-rhel9:1.0.12"

	// ConsoleContainerImagePF5 is the fall-back console image for PatternFly 5 (OCP < 4.19)
	ConsoleContainerImagePF5 = "registry.redhat.io/openshift-lightspeed/lightspeed-console-plugin-pf5-rhel9:1.0.12"

	// OKPContainerImage is the fall-back container image for OKP (Offline Knowledge Portal)
	OKPContainerImage = "registry.redhat.io/offline-knowledge-portal/rhokp-rhel9:latest"

	// MCPServerContainerImage is the fall-back container image for the MCP server
	MCPServerContainerImage = "quay.io/openstack-lightspeed/rhos-mcps:latest"

	// MaxTokensForResponseDefault is the default maximum number of tokens that should be used for response
	MaxTokensForResponseDefault = 2048
)

// DevSpec is the internal structure for the Dev field. Not exposed in the CRD.
// May change at any time without backward compatibility.
//
// Supported fields:
//   - featureFlags: list of experimental feature flags to enable
//   - okpChunkFilterQuery: Solr filter query for OKP searches (default: version-aware query combining detected OpenStack and OCP versions)
//   - okpRagOnly: when true, only OKP is used as a RAG source (default: true)
type DevSpec struct {
	FeatureFlags        []string `json:"featureFlags,omitempty"`
	OKPChunkFilterQuery string   `json:"okpChunkFilterQuery,omitempty"`
	OKPRagOnly          *bool    `json:"okpRagOnly,omitempty"`
}

// OKPSpec defines configuration for the Offline Knowledge Portal (OKP).
type OKPSpec struct {
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	// Offline controls how source URLs are resolved.
	// When true, uses parent_id (offline/Mimir-style).
	// When false, uses reference_url (online).
	Offline *bool `json:"offline,omitempty"`

	// +kubebuilder:validation:Optional
	// AccessKey is the name of the Secret containing the access key for the OKP server.
	// The secret must contain a key named "access_key".
	// An access key can be obtained from https://access.redhat.com/offline/access
	AccessKey string `json:"accessKey,omitempty"`
}

// DatabaseSpec defines configuration for persistent PostgreSQL storage.
type DatabaseSpec struct {
	// +kubebuilder:validation:Optional
	// Size of the PersistentVolumeClaim for PostgreSQL data. Defaults to 1Gi.
	Size resource.Quantity `json:"size,omitempty"`

	// +kubebuilder:validation:Optional
	// StorageClass name for the PersistentVolumeClaim. If omitted, the cluster's
	// default StorageClass is used.
	Class string `json:"class,omitempty"`
}

// OpenStackLightspeedSpec defines the desired state of OpenStackLightspeed
type OpenStackLightspeedSpec struct {
	OpenStackLightspeedCore `json:",inline"`

	// +kubebuilder:validation:Optional
	// Images configures container images used by the operator.
	// When omitted, each image defaults to its environment variable or hardcoded fallback.
	Images OpenStackLightspeedImages `json:"images,omitempty"`

	// +kubebuilder:validation:Optional
	// Database configures persistent storage for PostgreSQL data.
	// When omitted, an emptyDir volume is used (data is lost on pod reschedule).
	// When set, a PersistentVolumeClaim is created and mounted.
	Database *DatabaseSpec `json:"database,omitempty"`

	// +kubebuilder:validation:Optional
	// OKP configures the Offline Knowledge Portal (OKP) RAG source.
	OKP *OKPSpec `json:"okp,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// Dev contains developer/experimental configuration.
	// This section is not part of the stable API and may change at any time without backward compatibility.
	Dev runtime.RawExtension `json:"dev,omitempty"`
}

// LoggingConfig defines logging configuration for OpenStackLightspeed components
type LoggingConfig struct {
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="all=info"
	// +kubebuilder:validation:Pattern=`^\w+(?:=\w+)?(?:,\w+(?:=\w+)?)*$`
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="OGX Log Level"
	// Log level configuration for the OGX/llama-stack container. Supports standard levels (INFO, DEBUG) or fine-grained control using format "component=level,component=level" (e.g., "core=debug,providers=info").
	OGXLogLevel string `json:"ogxLogLevel,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=DEBUG;INFO;WARNING;ERROR;CRITICAL
	// +kubebuilder:default="INFO"
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Lightspeed Stack Log Level"
	// Log level for the lightspeed-service-api container. Supports standard Python log levels: DEBUG, INFO, WARNING, ERROR, CRITICAL.
	LightspeedStackLogLevel string `json:"lightspeedStackLogLevel,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=DEBUG;INFO;WARNING;ERROR;CRITICAL
	// +kubebuilder:default="INFO"
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Dataverse Exporter Log Level"
	// Log level for the dataverse exporter sidecar container. Supports standard Python log levels: DEBUG, INFO, WARNING, ERROR, CRITICAL.
	DataverseExporterLogLevel string `json:"dataverseExporterLogLevel,omitempty"`
}

// OpenStackLightspeedCore defines the desired state of OpenStackLightspeed
type OpenStackLightspeedCore struct {
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^https?://.+`
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="LLM Endpoint"
	// URL pointing to the LLM
	LLMEndpoint string `json:"llmEndpoint"`

	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=azure_openai;openai;watsonx;rhoai_vllm;rhelai_vllm;gemini
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Provider Type"
	// Type of the provider serving the LLM
	LLMEndpointType string `json:"llmEndpointType"`

	// +kubebuilder:validation:Required
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Model Name"
	// Name of the model to use at the API endpoint provided in LLMEndpoint
	ModelName string `json:"modelName"`

	// +kubebuilder:validation:Required
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="LLM Credentials Secret"
	// Secret name containing API token for the LLMEndpoint. The secret must contain
	// a field named "apitoken" which holds the token value.
	LLMCredentials string `json:"llmCredentials"`

	// +kubebuilder:validation:Optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="TLS CA Certificate Bundle"
	// Configmap name containing a CA Certificates bundle
	TLSCACertBundle string `json:"tlsCACertBundle"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Minimum=1
	// MaxTokensForResponse defines the maximum number of tokens to be used for the response generation
	MaxTokensForResponse int `json:"maxTokensForResponse,omitempty"`

	// +kubebuilder:validation:Optional
	// Project ID for LLM providers that require it (e.g., WatsonX)
	LLMProjectID string `json:"llmProjectID,omitempty"`

	// +kubebuilder:validation:Optional
	// Deployment name for LLM providers that require it (e.g., Microsoft Azure OpenAI)
	LLMDeploymentName string `json:"llmDeploymentName,omitempty"`

	// +kubebuilder:validation:Optional
	// LLM API Version for LLM providers that require it (e.g., Microsoft Azure OpenAI)
	LLMAPIVersion string `json:"llmAPIVersion,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	// Enable feedback collection
	FeedbackEnabled *bool `json:"feedbackEnabled,omitempty"`

	// +kubebuilder:validation:Optional
	// Enable conversation transcripts collection
	TranscriptsEnabled bool `json:"transcriptsEnabled,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default={}
	// Logging configuration for OpenStackLightspeed components
	Logging LoggingConfig `json:"logging"`
}

// OpenStackLightspeedStatus defines the observed state of OpenStackLightspeed
type OpenStackLightspeedStatus struct {
	// Conditions
	Conditions condition.Conditions `json:"conditions,omitempty" optional:"true"`

	// ObservedGeneration - the most recent generation observed for this object.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// +optional
	// OpenStackReady indicates whether an OpenStackControlPlane was detected and
	// is ready. When true, the OpenStack MCP tools are included in lightspeed-stack config.
	OpenStackReady bool `json:"openStackReady,omitempty"`

	// +optional
	// ApplicationCredentialSecret is the name of the current AC secret in the
	// OpenStack namespace. Tracked for rotation detection.
	ApplicationCredentialSecret string `json:"applicationCredentialSecret,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.conditions[0].status",description="Status"
// +kubebuilder:printcolumn:name="Message",type="string",JSONPath=".status.conditions[0].message",description="Message"
// +operator-sdk:csv:customresourcedefinitions:resources={{Deployment,v1,lightspeed-stack-deployment}}
// +operator-sdk:csv:customresourcedefinitions:resources={{Deployment,v1,lightspeed-postgres-server}}
// +operator-sdk:csv:customresourcedefinitions:resources={{Service,v1,lightspeed-app-server}}
// +operator-sdk:csv:customresourcedefinitions:resources={{Service,v1,lightspeed-postgres-server}}
// +operator-sdk:csv:customresourcedefinitions:resources={{ConfigMap,v1,llama-stack-config}}
// +operator-sdk:csv:customresourcedefinitions:resources={{ConfigMap,v1,lightspeed-stack-config}}
// +operator-sdk:csv:customresourcedefinitions:resources={{ConfigMap,v1,lightspeed-postgres-conf}}
// +operator-sdk:csv:customresourcedefinitions:resources={{Secret,v1,lightspeed-postgres-secret}}
// +operator-sdk:csv:customresourcedefinitions:resources={{Secret,v1,lightspeed-postgres-bootstrap}}
// +operator-sdk:csv:customresourcedefinitions:resources={{Secret,v1,metrics-reader-token}}
// +operator-sdk:csv:customresourcedefinitions:resources={{Secret,v1,lightspeed-tls}}
// +operator-sdk:csv:customresourcedefinitions:resources={{Secret,v1,lightspeed-postgres-certs}}
// +operator-sdk:csv:customresourcedefinitions:resources={{ServiceAccount,v1,lightspeed-app-server}}
// +operator-sdk:csv:customresourcedefinitions:resources={{NetworkPolicy,v1,lightspeed-app-server}}
// +operator-sdk:csv:customresourcedefinitions:resources={{NetworkPolicy,v1,lightspeed-postgres-server}}
// +operator-sdk:csv:customresourcedefinitions:resources={{PersistentVolumeClaim,v1,openstack-lightspeed-database}}
// +operator-sdk:csv:customresourcedefinitions:resources={{ClusterRole,v1,lightspeed-app-server-sar-role}}
// +operator-sdk:csv:customresourcedefinitions:resources={{ClusterRoleBinding,v1,lightspeed-app-server-sar-role-binding}}
// +operator-sdk:csv:customresourcedefinitions:resources={{ConfigMap,v1,mcp-config}}
// +operator-sdk:csv:customresourcedefinitions:resources={{Subscription,v1alpha1}}
// +operator-sdk:csv:customresourcedefinitions:resources={{ClusterServiceVersion,v1alpha1}}
// +operator-sdk:csv:customresourcedefinitions:resources={{InstallPlan,v1alpha1}}

// OpenStackLightspeed is the Schema for the openstacklightspeeds API
type OpenStackLightspeed struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OpenStackLightspeedSpec   `json:"spec,omitempty"`
	Status OpenStackLightspeedStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// OpenStackLightspeedList contains a list of OpenStackLightspeed
type OpenStackLightspeedList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OpenStackLightspeed `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OpenStackLightspeed{}, &OpenStackLightspeedList{})
}

// IsReady - returns true if OpenStackLightspeed is reconciled successfully
func (instance OpenStackLightspeed) IsReady() bool {
	return instance.Status.Conditions.IsTrue(OpenStackLightspeedReadyCondition)
}

// OpenStackLightspeedImages groups container image URLs used by the operator.
type OpenStackLightspeedImages struct {
	RAGImageURL        string `json:"ragImage,omitempty"`
	LCoreImageURL      string `json:"lcoreImage,omitempty"`
	ExporterImageURL   string `json:"exporterImage,omitempty"`
	PostgresImageURL   string `json:"postgresImage,omitempty"`
	ConsoleImageURL    string `json:"consoleImage,omitempty"`
	ConsoleImagePF5URL string `json:"consoleImagePF5,omitempty"`
	OKPImageURL        string `json:"okpImage,omitempty"`
	MCPServerImageURL  string `json:"mcpServerImage,omitempty"`
}

type OpenStackLightspeedDefaults struct {
	OpenStackLightspeedImages `json:",inline"`
	MaxTokensForResponse      int `json:"maxTokensForResponse,omitempty"`
}

var OpenStackLightspeedDefaultValues OpenStackLightspeedDefaults

// envVarDefaults holds the pristine env-var defaults set once by SetupDefaults.
// MergeDefaults copies from this so that removing dev overrides correctly
// reverts to the original values (the exported global gets overwritten each reconcile).
var envVarDefaults OpenStackLightspeedDefaults

// mergeImages applies non-zero fields from src onto dst.
func mergeImages(dst, src *OpenStackLightspeedImages) {
	dstVal := reflect.ValueOf(dst).Elem()
	srcVal := reflect.ValueOf(src).Elem()
	for i := 0; i < srcVal.NumField(); i++ {
		if !srcVal.Field(i).IsZero() {
			dstVal.Field(i).Set(srcVal.Field(i))
		}
	}
}

// SetupDefaults initializes OpenStackLightspeedDefaultValues from env vars.
// Call once at startup; the values never change inside a container.
func SetupDefaults() {
	envVarDefaults = OpenStackLightspeedDefaults{
		OpenStackLightspeedImages: OpenStackLightspeedImages{
			RAGImageURL: util.GetEnvVar(
				"RELATED_IMAGE_OPENSTACK_LIGHTSPEED_IMAGE_URL_DEFAULT", OpenStackLightspeedContainerImage),
			LCoreImageURL: util.GetEnvVar(
				"RELATED_IMAGE_LCORE_IMAGE_URL_DEFAULT", LCoreContainerImage),
			ExporterImageURL: util.GetEnvVar(
				"RELATED_IMAGE_EXPORTER_IMAGE_URL_DEFAULT", ExporterContainerImage),
			PostgresImageURL: util.GetEnvVar(
				"RELATED_IMAGE_POSTGRES_IMAGE_URL_DEFAULT", PostgresContainerImage),
			ConsoleImageURL: util.GetEnvVar(
				"RELATED_IMAGE_CONSOLE_IMAGE_URL_DEFAULT", ConsoleContainerImage),
			ConsoleImagePF5URL: util.GetEnvVar(
				"RELATED_IMAGE_CONSOLE_PF5_IMAGE_URL_DEFAULT", ConsoleContainerImagePF5),
			OKPImageURL: util.GetEnvVar(
				"RELATED_IMAGE_OKP_IMAGE_URL_DEFAULT", OKPContainerImage),
			MCPServerImageURL: util.GetEnvVar(
				"RELATED_IMAGE_MCP_SERVER_IMAGE_URL_DEFAULT", MCPServerContainerImage),
		},
		MaxTokensForResponse: MaxTokensForResponseDefault,
	}
	OpenStackLightspeedDefaultValues = envVarDefaults
}

// MergeDefaults returns a copy of the env-var defaults with the spec image
// overrides (if any) applied on top.
func MergeDefaults(specImages *OpenStackLightspeedImages) OpenStackLightspeedDefaults {
	merged := envVarDefaults
	if specImages != nil {
		mergeImages(&merged.OpenStackLightspeedImages, specImages)
	}
	return merged
}
