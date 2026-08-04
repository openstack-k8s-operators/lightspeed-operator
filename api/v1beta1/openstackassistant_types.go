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
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// OpenStackAssistantContainerImage is the fall-back container image for OpenStackAssistant
	OpenStackAssistantContainerImage = "quay.io/dprince/goose@sha256:07d7200f62bc2e8082de7a58396f8699b5f33fade1dbecc6a5b4ca03ab2f1d33"

	// OpenStackAssistantGooseServiceAccountName is the name of the ServiceAccount
	// that lightspeed-operator creates and owns for the assistant/goose pod
	// itself. Unlike OpenStackAssistantServiceAccountName, it carries no k8s API
	// resource RBAC grants - it exists so the pod can be bound to the nonroot-v2
	// SecurityContextConstraint (the goose image home directory is owned by
	// fixed UID 1000) and so goose can authenticate to Lightspeed via its SA
	// token. Lightspeed's k8s auth module requires GET on the /ls-access
	// non-resource URL; that single grant is created alongside this SA.
	OpenStackAssistantGooseServiceAccountName = "openstackassistant-goose"

	// OpenStackAssistantGooseSCCName is the SecurityContextConstraint that the
	// goose ServiceAccount is bound to, permitting the pod to run as the
	// fixed, non-root UID the goose container image expects.
	OpenStackAssistantGooseSCCName = "nonroot-v2"

	// OpenStackAssistantGooseLSAccessPath is the non-resource URL that
	// Lightspeed's k8s authentication module SubjectAccessReviews before
	// allowing a caller to use the /v1/responses API.
	OpenStackAssistantGooseLSAccessPath = "/ls-access"
)

// OpenStackAssistantGooseLSAccessClusterRoleName returns the namespace-scoped
// name of the ClusterRole that grants the goose SA GET /ls-access. The name is
// namespace-scoped because ClusterRole objects are cluster-scoped: multiple
// OpenStackAssistant instances in different namespaces each own their own role.
func OpenStackAssistantGooseLSAccessClusterRoleName(namespace string) string {
	return OpenStackAssistantGooseServiceAccountName + "-ls-access-" + namespace
}

// OpenStackAssistantGooseLSAccessClusterRoleBindingName returns the
// namespace-scoped name of the ClusterRoleBinding for the goose /ls-access
// grant. See OpenStackAssistantGooseLSAccessClusterRoleName for why this is
// namespace-scoped.
func OpenStackAssistantGooseLSAccessClusterRoleBindingName(namespace string) string {
	return OpenStackAssistantGooseLSAccessClusterRoleName(namespace) + "-binding"
}

// ProviderType defines the AI agent provider
// +kubebuilder:validation:Enum=goose
type ProviderType string

const (
	// ProviderGoose is the Goose AI agent provider
	ProviderGoose ProviderType = "goose"
)

// LightspeedStackSpec defines connectivity to the Lightspeed Stack AI backend
type LightspeedStackSpec struct {
	// ProviderSecret is the name of a Secret containing the lightspeed
	// provider config JSON (custom_providers/lightspeed.json content).
	// Must contain key "lightspeed.json".
	// +kubebuilder:validation:Required
	ProviderSecret string `json:"providerSecret"`

	// CaBundleSecretName is the name of a Secret containing CA certs
	// to trust for TLS connections to the lightspeed-stack endpoint.
	// The Secret must contain a key "ca-bundle.crt" with PEM-encoded certs.
	// +kubebuilder:validation:Optional
	CaBundleSecretName string `json:"caBundleSecretName,omitempty"`
}

// MCPServerRef references an MCP server endpoint to configure as a Goose extension.
// Either URL or OpenStackClientRef must be specified, but not both.
type MCPServerRef struct {
	// Name is the extension name in Goose config
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// URL is the MCP server's Streamable HTTP endpoint.
	// Mutually exclusive with OpenStackClientRef.
	// +kubebuilder:validation:Optional
	URL string `json:"url,omitempty"`

	// OpenStackClientRef is the name of an OpenStackClient CR, managed by
	// openstack-operator in the same namespace, that has MCP enabled. The
	// controller auto-computes the service URL by convention
	// (http(s)://<OpenStackClientRef>-mcp.<namespace>.svc:8080/openstack/)
	// and TLS CA configuration.
	// Mutually exclusive with URL.
	// +kubebuilder:validation:Optional
	OpenStackClientRef string `json:"openstackClientRef,omitempty"`
}

// GooseConfig defines Goose-specific provider configuration
type GooseConfig struct {
	// Model is the model identifier for the Goose AI agent
	// (e.g., "gemini/models/gemini-2.5-flash"). Sets the GOOSE_MODEL env var.
	// +kubebuilder:validation:Optional
	Model string `json:"model,omitempty"`

	// Recipes is a ConfigMap name containing Goose recipe YAML files.
	// Each key in the ConfigMap becomes a recipe file registered as a
	// Goose slash command (e.g., /cluster-health).
	// +kubebuilder:validation:Optional
	Recipes *string `json:"recipes,omitempty"`

	// Skills is a ConfigMap name containing Goose Agent Skill files.
	// Each key in the ConfigMap becomes a skill named after the key
	// (extension stripped), written as ~/.config/goose/skills/<name>/SKILL.md.
	// Unlike Recipes, skills are not explicitly invoked - Goose loads
	// them automatically when their description matches the task at hand.
	// +kubebuilder:validation:Optional
	Skills *string `json:"skills,omitempty"`

	// Hints is a ConfigMap name containing Goose hints/context.
	// The ConfigMap must have a key "hints" with the content that
	// will be written to ~/.goosehints in the pod.
	// +kubebuilder:validation:Optional
	Hints *string `json:"hints,omitempty"`

	// MCPServers lists MCP server endpoints to configure as Goose extensions.
	// +kubebuilder:validation:Optional
	MCPServers []MCPServerRef `json:"mcpServers,omitempty"`
}

// OpenStackAssistantSpec defines the desired state of OpenStackAssistant.
type OpenStackAssistantSpec struct {
	// ContainerImage for the assistant container (will be set to environmental default if empty).
	// +kubebuilder:validation:Optional
	ContainerImage string `json:"containerImage,omitempty"`

	// Provider is the AI agent provider type. Currently only "goose" is supported.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=goose
	Provider ProviderType `json:"provider"`

	// LightspeedStack configuration for the AI backend.
	// +kubebuilder:validation:Required
	LightspeedStack LightspeedStackSpec `json:"lightspeedStack"`

	// Goose contains Goose-specific provider configuration.
	// Only applicable when provider is "goose".
	// +kubebuilder:validation:Optional
	Goose *GooseConfig `json:"goose,omitempty"`

	// NodeSelector to target subset of worker nodes for pod scheduling.
	// +kubebuilder:validation:Optional
	NodeSelector *map[string]string `json:"nodeSelector,omitempty"`

	// Env is a list of additional environment variables for the container.
	// +kubebuilder:validation:Optional
	// +listType=map
	// +listMapKey=name
	Env []corev1.EnvVar `json:"env,omitempty"`
}

// OpenStackAssistantStatus defines the observed state of OpenStackAssistant.
type OpenStackAssistantStatus struct {
	// PodName is the name of the running assistant pod
	PodName string `json:"podName,omitempty"`

	// Conditions tracks the state of each sub-resource
	Conditions condition.Conditions `json:"conditions,omitempty" optional:"true"`

	// ObservedGeneration - the most recent generation observed
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Hash tracks input hashes to detect changes
	Hash map[string]string `json:"hash,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +operator-sdk:csv:customresourcedefinitions:displayName="OpenStack Assistant"
// +operator-sdk:csv:customresourcedefinitions:resources={{ServiceAccount,v1,openstackassistant-goose}}
// +operator-sdk:csv:customresourcedefinitions:resources={{RoleBinding,v1,openstackassistant-goose-nonroot-v2}}
// +operator-sdk:csv:customresourcedefinitions:resources={{ClusterRole,v1,openstackassistant-goose-ls-access}}
// +operator-sdk:csv:customresourcedefinitions:resources={{ClusterRoleBinding,v1,openstackassistant-goose-ls-access-binding}}
// +kubebuilder:resource:shortName=osassistant;osassistants
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.conditions[0].status",description="Status"
// +kubebuilder:printcolumn:name="Message",type="string",JSONPath=".status.conditions[0].message",description="Message"

// OpenStackAssistant is the Schema for the openstackassistants API.
type OpenStackAssistant struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OpenStackAssistantSpec   `json:"spec,omitempty"`
	Status OpenStackAssistantStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// OpenStackAssistantList contains a list of OpenStackAssistant.
type OpenStackAssistantList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OpenStackAssistant `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OpenStackAssistant{}, &OpenStackAssistantList{})
}

// IsReady - returns true if OpenStackAssistant is reconciled successfully
func (instance OpenStackAssistant) IsReady() bool {
	return instance.Status.Conditions.IsTrue(OpenStackAssistantReadyCondition)
}

// OpenStackAssistantDefaults holds defaults for the assistant
type OpenStackAssistantDefaults struct {
	ContainerImageURL string
}

var openStackAssistantDefaults OpenStackAssistantDefaults

// Default implements webhook.Defaulter
func (r *OpenStackAssistant) Default() {
	if r.Spec.ContainerImage == "" {
		r.Spec.ContainerImage = openStackAssistantDefaults.ContainerImageURL
	}
}
