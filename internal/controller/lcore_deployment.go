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
	"path"
	"slices"
	"strings"

	common_helper "github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	apiv1beta1 "github.com/openstack-k8s-operators/lightspeed-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// buildLCorePodTemplateSpec builds the pod template spec for the LCore deployment.
// This function is used by CreateOrPatch to generate the desired pod spec.
func buildLCorePodTemplateSpec(h *common_helper.Helper, ctx context.Context, instance *apiv1beta1.OpenStackLightspeed) (corev1.PodTemplateSpec, error) {
	// Build shared volumes
	volumes := []corev1.Volume{
		buildOGXConfigVolume(VolumeDefaultMode),
		buildLightspeedStackConfigVolume(VolumeDefaultMode),
		buildVectorDBScriptsVolume(),
	}

	// Shared volumes - CA bundle covers all cluster CAs
	sharedMounts := []corev1.VolumeMount{}
	addCABundleVolumesAndMounts(&volumes, &sharedMounts)
	addVectorDBDataVolumesAndMounts(&volumes, &sharedMounts)

	// Llama cache emptydir
	llamaCacheMounts := []corev1.VolumeMount{}
	addLlamaCacheVolumesAndMounts(&volumes, &llamaCacheMounts)

	// Build env vars
	llamaEnvVars, err := buildLlamaStackEnvVars(h, ctx, instance)
	if err != nil {
		return corev1.PodTemplateSpec{}, fmt.Errorf("failed to build llama-stack env vars: %w", err)
	}
	lsEnvVars := buildLightspeedStackEnvVars(instance)

	// Llama Stack container mounts: its config + shared + cache + vector_store_db data
	llamaStackMounts := []corev1.VolumeMount{}
	llamaStackMounts = append(llamaStackMounts, sharedMounts...)
	llamaStackMounts = append(llamaStackMounts, llamaCacheMounts...)
	llamaStackMounts = append(llamaStackMounts, corev1.VolumeMount{
		Name:      VectorDBScriptsVolumeName,
		MountPath: VectorDBScriptsMountPath,
		ReadOnly:  true,
	})

	llamaStackContainer := corev1.Container{
		Name:         "llama-stack",
		Image:        apiv1beta1.OpenStackLightspeedDefaultValues.LCoreImageURL,
		Command:      []string{"python3", VectorDBScriptsMountPath + "/" + LlamaStartupWrapperKey, "stack", "run", VectorDBVolumeOGXConfigPath},
		Ports:        []corev1.ContainerPort{{Name: "llama-stack", ContainerPort: LlamaStackContainerPort}},
		VolumeMounts: llamaStackMounts,
		Env:          llamaEnvVars,
		StartupProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: LlamaStackHealthPath,
					Port: intstr.FromInt32(LlamaStackContainerPort),
				},
			},
			PeriodSeconds:    LlamaStackProbePeriodSeconds,
			TimeoutSeconds:   LlamaStackProbeTimeoutSeconds,
			FailureThreshold: LlamaStackStartupProbeFailureThreshold,
		},
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: LlamaStackHealthPath,
					Port: intstr.FromInt32(LlamaStackContainerPort),
				},
			},
			PeriodSeconds:    LlamaStackProbePeriodSeconds,
			TimeoutSeconds:   LlamaStackProbeTimeoutSeconds,
			FailureThreshold: LlamaStackProbeFailureThreshold,
		},
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: LlamaStackHealthPath,
					Port: intstr.FromInt32(LlamaStackContainerPort),
				},
			},
			PeriodSeconds:    LlamaStackProbePeriodSeconds,
			TimeoutSeconds:   LlamaStackProbeTimeoutSeconds,
			FailureThreshold: LlamaStackProbeFailureThreshold,
		},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("500m"),
				corev1.ResourceMemory: resource.MustParse("2Gi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("2"),
				corev1.ResourceMemory: resource.MustParse("8Gi"),
			},
		},
		ImagePullPolicy: corev1.PullIfNotPresent,
	}

	// Data collection volumes (shared folder + exporter config)
	dataCollectionEnabled := isDataCollectionEnabled(instance)
	if dataCollectionEnabled {
		addDataCollectorVolumes(&volumes, VolumeDefaultMode)
	}

	// Lightspeed Stack container mounts: its config + shared + TLS (only API container needs TLS)
	lightspeedStackMounts := []corev1.VolumeMount{}
	lightspeedStackMounts = append(lightspeedStackMounts, sharedMounts...)

	tlsMounts := []corev1.VolumeMount{}
	addTLSVolumesAndMounts(&volumes, &tlsMounts, VolumeDefaultMode)
	lightspeedStackMounts = append(lightspeedStackMounts, tlsMounts...)

	// Mount shared data folder on lightspeed-service-api for feedback/transcripts
	if dataCollectionEnabled {
		lightspeedStackMounts = append(lightspeedStackMounts, corev1.VolumeMount{
			Name:      UserDataVolumeName,
			MountPath: LCoreUserDataMountPath,
		})
	}

	lightspeedStackContainer := corev1.Container{
		Name:           "lightspeed-service-api",
		Image:          apiv1beta1.OpenStackLightspeedDefaultValues.LCoreImageURL,
		Args:           []string{"-c", VectorDBVolumeLightspeedStackConfigPath},
		Ports:          []corev1.ContainerPort{{Name: "https", ContainerPort: OpenStackLightspeedAppServerContainerPort}},
		VolumeMounts:   lightspeedStackMounts,
		Env:            lsEnvVars,
		StartupProbe:   buildLightspeedStackStartupProbe(),
		LivenessProbe:  buildLightspeedStackLivenessProbe(),
		ReadinessProbe: buildLightspeedStackReadinessProbe(),
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("250m"),
				corev1.ResourceMemory: resource.MustParse("512Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("1"),
				corev1.ResourceMemory: resource.MustParse("2Gi"),
			},
		},
		ImagePullPolicy: corev1.PullIfNotPresent,
	}
	containers := []corev1.Container{llamaStackContainer, lightspeedStackContainer}

	// Add dataverse exporter sidecar when data collection is enabled
	if dataCollectionEnabled {
		exporterContainer := corev1.Container{
			Name:            DataverseExporterContainerName,
			Image:           apiv1beta1.OpenStackLightspeedDefaultValues.ExporterImageURL,
			ImagePullPolicy: corev1.PullAlways,
			Args: []string{
				"--mode", "openshift",
				"--config", path.Join(ExporterConfigMountPath, ExporterConfigFilename),
				"--log-level", instance.Spec.Logging.DataverseExporterLogLevel,
				"--data-dir", LCoreUserDataMountPath,
			},
			VolumeMounts: []corev1.VolumeMount{
				{
					Name:      UserDataVolumeName,
					MountPath: LCoreUserDataMountPath,
				},
				{
					Name:      ExporterConfigVolumeName,
					MountPath: ExporterConfigMountPath,
					ReadOnly:  true,
				},
				{
					Name:      CABundleVolumeName,
					MountPath: CABundleMountPath,
					SubPath:   CABundleKey,
					ReadOnly:  true,
				},
			},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("50m"),
					corev1.ResourceMemory: resource.MustParse("64Mi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("200Mi"),
				},
			},
		}
		containers = append(containers, exporterContainer)
	}

	// MCP sidecar (only when rhos_mcps feature flag is enabled)
	rhosMCPEnabled, err := isRHOSMCPEnabled(instance)
	if err != nil {
		return corev1.PodTemplateSpec{}, fmt.Errorf("failed to parse dev config: %w", err)
	}
	if rhosMCPEnabled {
		mcpMounts := []corev1.VolumeMount{}
		addMCPVolumesAndMounts(&volumes, &mcpMounts, instance.Status.OpenStackReady)

		mcpContainer := corev1.Container{
			Name:         "rhos-mcp",
			Image:        apiv1beta1.OpenStackLightspeedDefaultValues.MCPServerImageURL,
			VolumeMounts: mcpMounts,
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("50m"),
					corev1.ResourceMemory: resource.MustParse("64Mi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("200Mi"),
				},
			},
			ImagePullPolicy: corev1.PullIfNotPresent,
		}
		containers = append(containers, mcpContainer)
	}

	// Build configmap resource version annotations for change detection
	annotations, err := buildConfigMapAnnotations(h, ctx, instance)
	if err != nil {
		return corev1.PodTemplateSpec{}, err
	}

	initContainers := buildInitContainers(instance)

	return corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      generateAppServerSelectorLabels(),
			Annotations: annotations,
		},
		Spec: corev1.PodSpec{
			ServiceAccountName: OpenStackLightspeedAppServerServiceAccountName,
			InitContainers:     initContainers,
			Containers:         containers,
			Volumes:            volumes,
		},
	}, nil
}

// buildInitContainers returns the configuration for initContainers that run
// before the main OGX and Lightspeed Stack containers in the Lightspeed Stack
// deployment. These initContainers are responsible for generating the final OGX
// and Lightspeed Stack configuration files, incorporating information from
// the provided vector database images. For details on their logic, see:
// (1) assets/vector_database_collect.sh and (2) assets/vector_database_build.py.
func buildInitContainers(instance *apiv1beta1.OpenStackLightspeed) []corev1.Container {
	securityContext := &corev1.SecurityContext{
		RunAsNonRoot:             &[]bool{true}[0],
		AllowPrivilegeEscalation: &[]bool{false}[0],
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{"ALL"},
		},
	}

	resourceRequirements := corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("100m"),
			corev1.ResourceMemory: resource.MustParse("256Mi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("500m"),
			corev1.ResourceMemory: resource.MustParse("1Gi"),
		},
	}

	var containers []corev1.Container
	containers = append(containers, corev1.Container{
		Name:  "vector-database-collect",
		Image: apiv1beta1.OpenStackLightspeedDefaultValues.RAGImageURL,
		Command: []string{
			"sh", VectorDBScriptsMountPath + "/" + VectorDBCollectScriptKey,
			"--vector-db-path", VectorDBVolumeMountPath,
			"--enable-okp",
		},
		SecurityContext: securityContext,
		Resources:       resourceRequirements,
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      VectorDBVolumeName,
				MountPath: VectorDBVolumeMountPath,
			},
			{
				Name:      VectorDBScriptsVolumeName,
				MountPath: VectorDBScriptsMountPath,
				ReadOnly:  true,
			},
		},
	})

	configBuildCmd := []string{
		"python3", VectorDBScriptsMountPath + "/" + VectorDBBuildScriptKey,
		"--vector-db-path", VectorDBVolumeMountPath,
		"--ogx-config-path", OGXConfigInitContainerMountPath,
		"--lightspeed-stack-path", LightspeedStackInitContainerMountPath,
	}
	devConfig, _ := parseDevConfig(instance)
	if devConfig.OKPRagOnly == nil || *devConfig.OKPRagOnly {
		configBuildCmd = append(configBuildCmd, "--disable-rag-entries")
	}

	containers = append(containers, corev1.Container{
		Name:            "vector-database-config-build",
		Image:           apiv1beta1.OpenStackLightspeedDefaultValues.LCoreImageURL,
		Command:         configBuildCmd,
		SecurityContext: securityContext,
		Resources:       resourceRequirements,
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      VectorDBVolumeName,
				MountPath: VectorDBVolumeMountPath,
			},
			{
				Name:      VectorDBScriptsVolumeName,
				MountPath: VectorDBScriptsMountPath,
				ReadOnly:  true,
			},
			{
				Name:      OGXConfigVolumeName,
				MountPath: OGXConfigInitContainerMountPath,
				SubPath:   OGXConfigCMKey,
			},
			{
				Name:      LightspeedStackConfig,
				MountPath: LightspeedStackInitContainerMountPath,
				SubPath:   LightspeedStackConfigCMKey,
			},
		},
	})

	return containers
}

// buildLightspeedStackConfigVolume returns the volume for the lightspeed-stack config.
func buildLightspeedStackConfigVolume(volumeDefaultMode int32) corev1.Volume {
	return corev1.Volume{
		Name: LightspeedStackConfig,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: LCoreConfigCmName,
				},
				DefaultMode: toPtr(volumeDefaultMode),
			},
		},
	}
}

// buildOGXConfigVolume returns the volume for the OGX config.
func buildOGXConfigVolume(volumeDefaultMode int32) corev1.Volume {
	return corev1.Volume{
		Name: OGXConfigVolumeName,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: LlamaStackConfigCmName,
				},
				DefaultMode: toPtr(volumeDefaultMode),
			},
		},
	}
}

// buildVectorDBScriptsVolume returns the volume for the Vector DB scripts.
func buildVectorDBScriptsVolume() corev1.Volume {
	return corev1.Volume{
		Name: VectorDBScriptsVolumeName,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: VectorDBScriptsConfigMapName,
				},
				DefaultMode: toPtr(VolumeExecutableMode),
			},
		},
	}
}

func addVectorDBDataVolumesAndMounts(volumes *[]corev1.Volume, mounts *[]corev1.VolumeMount) {
	*volumes = append(*volumes, corev1.Volume{
		Name: VectorDBVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	})

	*mounts = append(*mounts, corev1.VolumeMount{
		Name:      VectorDBVolumeName,
		MountPath: VectorDBVolumeMountPath,
	})
}

// addTLSVolumesAndMounts adds the service-ca TLS certificate volume and mount.
func addTLSVolumesAndMounts(volumes *[]corev1.Volume, mounts *[]corev1.VolumeMount, volumeDefaultMode int32) {
	*volumes = append(*volumes, corev1.Volume{
		Name: "tls-certs",
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName:  OpenStackLightspeedCertsSecretName,
				DefaultMode: toPtr(volumeDefaultMode),
			},
		},
	})
	*mounts = append(*mounts, corev1.VolumeMount{
		Name:      "tls-certs",
		MountPath: OpenStackLightspeedAppCertsMountRoot + "/lightspeed-tls",
		ReadOnly:  true,
	})
}

// addLlamaCacheVolumesAndMounts adds an emptydir volume for llama-stack cache.
func addLlamaCacheVolumesAndMounts(volumes *[]corev1.Volume, mounts *[]corev1.VolumeMount) {
	*volumes = append(*volumes, corev1.Volume{
		Name: "llama-cache",
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	})
	*mounts = append(*mounts, corev1.VolumeMount{
		Name:      "llama-cache",
		MountPath: "/tmp/llama-stack",
	})
}

// addDataCollectorVolumes adds the shared data EmptyDir and exporter config volumes.
func addDataCollectorVolumes(volumes *[]corev1.Volume, volumeDefaultMode int32) {
	*volumes = append(*volumes, corev1.Volume{
		Name: UserDataVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	})

	*volumes = append(*volumes, corev1.Volume{
		Name: ExporterConfigVolumeName,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: ExporterConfigCmName,
				},
				DefaultMode: toPtr(volumeDefaultMode),
			},
		},
	})
}

// addMCPVolumesAndMounts adds MCP sidecar volumes and mounts.
// OpenStack-specific volumes (clouds.yaml, secure.yaml, tls-ca-bundle.pem) are
// only added when openStackReady is true, since they only exist after being
// copied from the OpenStackControlPlane namespace.
func addMCPVolumesAndMounts(volumes *[]corev1.Volume, mounts *[]corev1.VolumeMount, openStackReady bool) {
	if openStackReady {
		*volumes = append(*volumes,
			corev1.Volume{
				Name: SecureYAMLSecretName,
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: SecureYAMLSecretName,
						Items:      []corev1.KeyToPath{{Key: "secure.yaml", Path: "secure.yaml"}},
					},
				},
			},
			corev1.Volume{
				Name: CloudsYAMLConfigMapName,
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{Name: CloudsYAMLConfigMapName},
						Items:                []corev1.KeyToPath{{Key: "clouds.yaml", Path: "clouds.yaml"}},
					},
				},
			},
			corev1.Volume{
				Name: CombinedCABundleSecretName,
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: CombinedCABundleSecretName,
						Items:      []corev1.KeyToPath{{Key: "tls-ca-bundle.pem", Path: "tls-ca-bundle.pem"}},
					},
				},
			},
		)

		*mounts = append(*mounts,
			corev1.VolumeMount{Name: SecureYAMLSecretName, MountPath: "/app/secure.yaml", SubPath: "secure.yaml"},
			corev1.VolumeMount{Name: CloudsYAMLConfigMapName, MountPath: "/app/clouds.yaml", SubPath: "clouds.yaml"},
			corev1.VolumeMount{Name: CombinedCABundleSecretName, MountPath: "/app/tls-ca-bundle.pem", SubPath: "tls-ca-bundle.pem", ReadOnly: true},
		)
	}

	*volumes = append(*volumes,
		corev1.Volume{
			Name: MCPConfigYAMLConfigMapName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: MCPConfigYAMLConfigMapName},
					Items:                []corev1.KeyToPath{{Key: "config.yaml", Path: "config.yaml"}},
				},
			},
		},
	)

	*mounts = append(*mounts,
		corev1.VolumeMount{Name: MCPConfigYAMLConfigMapName, MountPath: "/app/config.yaml", SubPath: "config.yaml"},
	)
}

// addCABundleVolumesAndMounts adds the CA bundle volume and mount.
// The CA bundle is always present (created by reconcileCABundleConfigMap)
// and mounted at the RHEL system CA path so applications find it automatically.
func addCABundleVolumesAndMounts(volumes *[]corev1.Volume, mounts *[]corev1.VolumeMount) {
	*volumes = append(*volumes, corev1.Volume{
		Name: CABundleVolumeName,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: CABundleConfigMapName,
				},
				DefaultMode: toPtr(VolumeDefaultMode),
			},
		},
	})
	*mounts = append(*mounts, corev1.VolumeMount{
		Name:      CABundleVolumeName,
		MountPath: CABundleMountPath,
		SubPath:   CABundleKey,
		ReadOnly:  true,
	})
}

// buildLlamaStackEnvVars builds environment variables for llama-stack,
// primarily provider API keys read from Kubernetes secrets.
func buildLlamaStackEnvVars(h *common_helper.Helper, ctx context.Context, instance *apiv1beta1.OpenStackLightspeed) ([]corev1.EnvVar, error) {
	envVars := []corev1.EnvVar{}

	{
		provider := buildProvider(instance)
		if provider.CredentialsSecret == "" {
			return envVars, nil
		}

		envVarName := providerNameToEnvVarName(provider.Name)

		if provider.Type == AzureOpenAIProviderName {
			// Azure supports both API key and client credentials authentication.
			// Read the secret to determine which fields are present.
			secret := &corev1.Secret{}
			err := h.GetClient().Get(ctx, types.NamespacedName{
				Name:      provider.CredentialsSecret,
				Namespace: h.GetBeforeObject().GetNamespace(),
			}, secret)
			if err != nil {
				return nil, fmt.Errorf("failed to get Azure provider secret %s: %w", provider.CredentialsSecret, err)
			}

			// API key (always include - required by LiteLLM's Pydantic validation)
			if _, ok := secret.Data["apitoken"]; ok {
				envVars = append(envVars, corev1.EnvVar{
					Name: envVarName + "_API_KEY",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: provider.CredentialsSecret,
							},
							Key: "apitoken",
						},
					},
				})
			} else {
				// Provide an empty default so the env var exists
				envVars = append(envVars, corev1.EnvVar{
					Name:  envVarName + "_API_KEY",
					Value: "",
				})
			}

			// Client credentials fields for Azure AD authentication
			for _, field := range []struct {
				secretKey string
				envSuffix string
			}{
				{"client_id", "_CLIENT_ID"},
				{"tenant_id", "_TENANT_ID"},
				{"client_secret", "_CLIENT_SECRET"},
			} {
				if _, ok := secret.Data[field.secretKey]; ok {
					envVars = append(envVars, corev1.EnvVar{
						Name: envVarName + field.envSuffix,
						ValueFrom: &corev1.EnvVarSource{
							SecretKeyRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: provider.CredentialsSecret,
								},
								Key: field.secretKey,
							},
						},
					})
				} else {
					envVars = append(envVars, corev1.EnvVar{
						Name:  envVarName + field.envSuffix,
						Value: "",
					})
				}
			}
		} else {
			// Non-Azure providers: single API_KEY from the "apitoken" key
			envVars = append(envVars, corev1.EnvVar{
				Name: envVarName + "_API_KEY",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: provider.CredentialsSecret,
						},
						Key: "apitoken",
					},
				},
			})

			// For vLLM providers, also set the URL environment variable
			// The vLLM adapter checks for VLLM_URL as a fallback if URL is not in config
			if provider.Type == RHOAIVLLMProviderName || provider.Type == RHELAIVLLMProviderName {
				if provider.URL != "" {
					envVars = append(envVars, corev1.EnvVar{
						Name:  "VLLM_URL",
						Value: provider.URL,
					})
				}
			}
		}
	}

	// Postgres credentials for ${env.POSTGRESQL_PASSWORD} and ${env.POSTGRESQL_USER}
	// substitution in llama-stack config
	envVars = append(envVars, buildPostgresCredsEnvVars()...)

	// PostgreSQL SSL configuration for OGX (llama-stack).
	// OGX's PostgresSqlStoreConfig does not support ssl_mode/ca_cert_path fields yet
	// (ogx-ai/ogx#5978), so we configure asyncpg via standard libpq environment
	// variables to enforce TLS with full certificate verification.
	envVars = append(envVars, corev1.EnvVar{
		Name:  "PGSSLMODE",
		Value: PostgresDefaultSSLMode,
	})
	envVars = append(envVars, corev1.EnvVar{
		Name:  "PGSSLROOTCERT",
		Value: CABundleMountPath,
	})

	// Logging configuration - set both for compatibility with llama-stack and OGX
	ogxLogLevel := getOGXLogLevel(instance)
	envVars = append(envVars, corev1.EnvVar{
		Name:  "LLAMA_STACK_LOGGING",
		Value: ogxLogLevel,
	})
	envVars = append(envVars, corev1.EnvVar{
		Name:  "OGX_LOGGING",
		Value: ogxLogLevel,
	})

	envVars = append(envVars, corev1.EnvVar{
		Name:  "VECTOR_DB_DATA_PATH",
		Value: VectorDBVolumeMountPath,
	})

	envVars = append(envVars, corev1.EnvVar{
		Name:  "RH_SERVER_OKP",
		Value: fmt.Sprintf("http://%s.%s.svc:%d", OKPServiceName, instance.GetNamespace(), OKPServicePort),
	})

	return envVars, nil
}

// buildPostgresCredsEnvVars returns the POSTGRESQL_PASSWORD and POSTGRESQL_USER env var sourced from
// the postgres secret generated by the operator.
func buildPostgresCredsEnvVars() []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name: "POSTGRESQL_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: PostgresSecretName,
					},
					Key: OpenStackLightspeedComponentPasswordFileName,
				},
			},
		},
		{
			Name: "POSTGRESQL_USER",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: PostgresSecretName,
					},
					Key: PostgresUsernameSecretKey,
				},
			},
		},
	}
}

// buildLightspeedStackEnvVars builds environment variables for the lightspeed-stack container.
func buildLightspeedStackEnvVars(instance *apiv1beta1.OpenStackLightspeed) []corev1.EnvVar {
	envVars := []corev1.EnvVar{
		{
			Name:  "LIGHTSPEED_STACK_LOG_LEVEL",
			Value: instance.Spec.Logging.LightspeedStackLogLevel,
		},
	}
	envVars = append(envVars, corev1.EnvVar{
		Name:  "RH_SERVER_OKP",
		Value: fmt.Sprintf("http://%s.%s.svc:%d", OKPServiceName, instance.GetNamespace(), OKPServicePort),
	})
	envVars = append(envVars, buildPostgresCredsEnvVars()...)
	return envVars
}

// buildLightspeedStackStartupProbe returns the startup probe for the lightspeed-stack container.
func buildLightspeedStackStartupProbe() *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path:   LightspeedStackReadinessPath,
				Port:   intstr.FromInt32(OpenStackLightspeedAppServerContainerPort),
				Scheme: corev1.URISchemeHTTPS,
			},
		},
		PeriodSeconds:    LightspeedStackProbePeriodSeconds,
		TimeoutSeconds:   LightspeedStackProbeTimeoutSeconds,
		FailureThreshold: LightspeedStackStartupProbeFailureThreshold,
	}
}

// buildLightspeedStackLivenessProbe returns the liveness probe for the lightspeed-stack container.
func buildLightspeedStackLivenessProbe() *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path:   LightspeedStackLivenessPath,
				Port:   intstr.FromInt32(OpenStackLightspeedAppServerContainerPort),
				Scheme: corev1.URISchemeHTTPS,
			},
		},
		PeriodSeconds:    LightspeedStackProbePeriodSeconds,
		TimeoutSeconds:   LightspeedStackProbeTimeoutSeconds,
		FailureThreshold: LightspeedStackProbeFailureThreshold,
	}
}

// buildLightspeedStackReadinessProbe returns the readiness probe for the lightspeed-stack container.
func buildLightspeedStackReadinessProbe() *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path:   LightspeedStackReadinessPath,
				Port:   intstr.FromInt32(OpenStackLightspeedAppServerContainerPort),
				Scheme: corev1.URISchemeHTTPS,
			},
		},
		PeriodSeconds:    LightspeedStackProbePeriodSeconds,
		TimeoutSeconds:   LightspeedStackProbeTimeoutSeconds,
		FailureThreshold: LightspeedStackProbeFailureThreshold,
	}
}

// getOGXLogLevel returns the log level for OGX/llama-stack container.
// Supports either standard levels (INFO, DEBUG, WARNING, ERROR, CRITICAL) or fine-grained control.
// Examples: "INFO" -> "all=info", "DEBUG" -> "all=debug", "core=debug,providers=info" -> "core=debug,providers=info"
// Defaults to "all=info" if not specified.
func getOGXLogLevel(instance *apiv1beta1.OpenStackLightspeed) string {
	logLevel := instance.Spec.Logging.OGXLogLevel

	// If it's a simple level (INFO, DEBUG, etc.), convert to "all=<level>" format
	// Otherwise, pass through for fine-grained control (e.g., "core=debug,providers=info")
	upperLogLevel := strings.ToUpper(logLevel)
	allowedLogLevels := []string{"DEBUG", "INFO", "WARNING", "ERROR", "CRITICAL"}
	if slices.Contains(allowedLogLevels, upperLogLevel) {
		return fmt.Sprintf("all=%s", strings.ToLower(logLevel))
	}

	return logLevel
}

// buildConfigMapAnnotations builds annotations with configmap resource versions
// so that changes to the configmaps trigger a deployment rollout.
func buildConfigMapAnnotations(h *common_helper.Helper, ctx context.Context, instance *apiv1beta1.OpenStackLightspeed) (map[string]string, error) {
	annotations := make(map[string]string)

	lcoreVersion, err := getConfigMapResourceVersion(ctx, h, LCoreConfigCmName, h.GetBeforeObject().GetNamespace())
	if err != nil {
		// ConfigMap may not exist yet during initial creation
		if !errors.IsNotFound(err) {
			return nil, fmt.Errorf("failed to get LCore configmap resource version: %w", err)
		}
	} else {
		annotations[LCoreConfigMapResourceVersionAnnotation] = lcoreVersion
	}

	llamaVersion, err := getConfigMapResourceVersion(ctx, h, LlamaStackConfigCmName, h.GetBeforeObject().GetNamespace())
	if err != nil {
		if !errors.IsNotFound(err) {
			return nil, fmt.Errorf("failed to get Llama Stack configmap resource version: %w", err)
		}
	} else {
		annotations[LlamaStackConfigMapResourceVersionAnnotation] = llamaVersion
	}

	vectorDBScriptsVersion, err := getConfigMapResourceVersion(ctx, h, VectorDBScriptsConfigMapName, h.GetBeforeObject().GetNamespace())
	if err != nil {
		if !errors.IsNotFound(err) {
			return nil, fmt.Errorf("failed to get Vector DB scripts configmap resource version: %w", err)
		}
	} else {
		annotations[VectorDBScriptsConfigMapVersionAnnotation] = vectorDBScriptsVersion
	}

	caBundleVersion, err := getConfigMapResourceVersion(ctx, h, CABundleConfigMapName, h.GetBeforeObject().GetNamespace())
	if err != nil {
		if !errors.IsNotFound(err) {
			return nil, fmt.Errorf("failed to get CA bundle configmap resource version: %w", err)
		}
	} else {
		annotations[CABundleConfigMapVersionAnnotation] = caBundleVersion
	}

	postgresSecretVersion, err := getSecretResourceVersion(ctx, h, PostgresSecretName, h.GetBeforeObject().GetNamespace())
	if err != nil {
		if !errors.IsNotFound(err) {
			return nil, fmt.Errorf("failed to get postgres secret resource version: %w", err)
		}
	} else {
		annotations[PostgresSecretResourceVersionAnnotation] = postgresSecretVersion
	}

	mcpVersion, err := getConfigMapResourceVersion(ctx, h, MCPConfigYAMLConfigMapName, h.GetBeforeObject().GetNamespace())
	if err != nil {
		if !errors.IsNotFound(err) {
			return nil, fmt.Errorf("failed to get MCP config configmap resource version: %w", err)
		}
	} else {
		annotations[MCPConfigMapResourceVersionAnnotation] = mcpVersion
	}

	// OpenStack-specific resources are only mounted when OpenStackReady is
	// true (via addMCPVolumesAndMounts). Tracking their ResourceVersion
	// when they're not mounted would cause unnecessary pod rollouts if
	// they get created before OpenStackReady transitions to true.
	if instance.Status.OpenStackReady {
		cloudsVersion, err := getConfigMapResourceVersion(ctx, h, CloudsYAMLConfigMapName, h.GetBeforeObject().GetNamespace())
		if err != nil {
			if !errors.IsNotFound(err) {
				return nil, fmt.Errorf("failed to get clouds.yaml configmap resource version: %w", err)
			}
		} else {
			annotations[CloudsYAMLConfigMapVersionAnnotation] = cloudsVersion
		}

		secureVersion, err := getSecretResourceVersion(ctx, h, SecureYAMLSecretName, h.GetBeforeObject().GetNamespace())
		if err != nil {
			if !errors.IsNotFound(err) {
				return nil, fmt.Errorf("failed to get secure.yaml secret resource version: %w", err)
			}
		} else {
			annotations[SecureYAMLSecretVersionAnnotation] = secureVersion
		}

		caBundleSecretVersion, err := getSecretResourceVersion(ctx, h, CombinedCABundleSecretName, h.GetBeforeObject().GetNamespace())
		if err != nil {
			if !errors.IsNotFound(err) {
				return nil, fmt.Errorf("failed to get CA bundle secret resource version: %w", err)
			}
		} else {
			annotations[CombinedCABundleSecretVersionAnnotation] = caBundleSecretVersion
		}
	}

	return annotations, nil
}
