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

	consolev1 "github.com/openshift/api/console/v1"
	apiv1beta1 "github.com/openstack-k8s-operators/lightspeed-operator/api/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// generateConsoleSelectorLabels returns a map of labels used as selectors
// for the console plugin pods.
func generateConsoleSelectorLabels() map[string]string {
	return map[string]string{
		"app.kubernetes.io/component":  "console-plugin",
		"app.kubernetes.io/managed-by": "openstack-lightspeed-operator",
		"app.kubernetes.io/name":       "lightspeed-console-plugin",
		"app.kubernetes.io/part-of":    "openstack-lightspeed",
	}
}

// consoleLocalesFilename is the filename of the locales JSON file.
const consoleLocalesFilename = "plugin__lightspeed-console-plugin.json"

// consoleLocalesPath is the path to the locales JSON file inside the console image.
const consoleLocalesPath = "/usr/share/nginx/html/locales/en/" + consoleLocalesFilename

// buildConsoleDeploymentSpec builds the Deployment spec for the console plugin.
// Includes an init container that rewrites OpenShift references to OpenStack
// in the locales JSON file using an emptyDir volume.
func buildConsoleDeploymentSpec(consoleImage string, instance *apiv1beta1.OpenStackLightspeed) appsv1.DeploymentSpec {
	consoleRes := instance.Spec.Resources.ConsolePlugin

	replicas := int32(1)
	volumeDefaultMode := VolumeDefaultMode
	labels := generateConsoleSelectorLabels()

	return appsv1.DeploymentSpec{
		Replicas: &replicas,
		Selector: &metav1.LabelSelector{
			MatchLabels: labels,
		},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: labels,
			},
			Spec: corev1.PodSpec{
				SecurityContext: &corev1.PodSecurityContext{
					RunAsNonRoot: toPtr(true),
					SeccompProfile: &corev1.SeccompProfile{
						Type: corev1.SeccompProfileTypeRuntimeDefault,
					},
				},
				ServiceAccountName: ConsoleUIServiceAccountName,
				InitContainers: []corev1.Container{
					{
						Name:  "rewrite-locales",
						Image: consoleImage,
						SecurityContext: &corev1.SecurityContext{
							AllowPrivilegeEscalation: toPtr(false),
							ReadOnlyRootFilesystem:   toPtr(true),
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{"ALL"},
							},
						},
						Command: []string{
							"sh", "-c",
							"awk '" + consoleLocalesRewriteAwk + "' " +
								consoleLocalesPath + " > /locales-rewrite/" + consoleLocalesFilename,
						},
						Resources: consoleRes,
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      "locales-rewrite",
								MountPath: "/locales-rewrite",
							},
						},
					},
				},
				Containers: []corev1.Container{
					{
						Name:  "lightspeed-console-plugin",
						Image: consoleImage,
						Ports: []corev1.ContainerPort{
							{
								ContainerPort: ConsoleUIHTTPSPort,
								Name:          "https",
								Protocol:      corev1.ProtocolTCP,
							},
						},
						ImagePullPolicy: corev1.PullAlways,
						SecurityContext: &corev1.SecurityContext{
							AllowPrivilegeEscalation: toPtr(false),
						},
						Resources: consoleRes,
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      "lightspeed-console-plugin-cert",
								MountPath: "/var/cert",
								ReadOnly:  true,
							},
							{
								Name:      "nginx-config",
								MountPath: "/etc/nginx/nginx.conf",
								SubPath:   "nginx.conf",
								ReadOnly:  true,
							},
							{
								Name:      "nginx-temp",
								MountPath: "/tmp/nginx",
							},
							{
								Name:      "locales-rewrite",
								MountPath: consoleLocalesPath,
								SubPath:   consoleLocalesFilename,
								ReadOnly:  true,
							},
						},
					},
				},
				Volumes: []corev1.Volume{
					{
						Name: "lightspeed-console-plugin-cert",
						VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{
								SecretName:  ConsoleUIServiceCertSecretName,
								DefaultMode: &volumeDefaultMode,
							},
						},
					},
					{
						Name: "nginx-config",
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: ConsoleUIConfigMapName,
								},
								DefaultMode: &volumeDefaultMode,
							},
						},
					},
					{
						Name: "nginx-temp",
						VolumeSource: corev1.VolumeSource{
							EmptyDir: &corev1.EmptyDirVolumeSource{},
						},
					},
					{
						Name: "locales-rewrite",
						VolumeSource: corev1.VolumeSource{
							EmptyDir: &corev1.EmptyDirVolumeSource{},
						},
					},
				},
			},
		},
	}
}

// buildConsolePluginSpec builds the ConsolePlugin spec with backend and proxy configuration.
func buildConsolePluginSpec(namespace string) consolev1.ConsolePluginSpec {
	return consolev1.ConsolePluginSpec{
		Backend: consolev1.ConsolePluginBackend{
			Service: &consolev1.ConsolePluginService{
				Name:      ConsoleUIServiceName,
				Namespace: namespace,
				Port:      ConsoleUIHTTPSPort,
				BasePath:  "/",
			},
			Type: consolev1.Service,
		},
		DisplayName: "Lightspeed Console Plugin",
		I18n: consolev1.ConsolePluginI18n{
			LoadType: consolev1.Preload,
		},
		Proxy: []consolev1.ConsolePluginProxy{
			{
				Alias:         ConsoleProxyAlias,
				Authorization: consolev1.UserToken,
				Endpoint: consolev1.ConsolePluginProxyEndpoint{
					Service: &consolev1.ConsolePluginProxyServiceConfig{
						Name:      OpenStackLightspeedAppServerServiceName,
						Namespace: namespace,
						Port:      OpenStackLightspeedAppServerServicePort,
					},
					Type: consolev1.ProxyTypeService,
				},
			},
		},
	}
}

// buildConsoleNginxConfig returns the nginx configuration content for the console plugin.
func buildConsoleNginxConfig() string {
	return fmt.Sprintf(consoleNginxConfigTemplate, ConsoleUIHTTPSPort)
}

// buildConsoleNetworkPolicySpec builds the NetworkPolicy spec for the console plugin.
func buildConsoleNetworkPolicySpec() networkingv1.NetworkPolicySpec {
	return networkingv1.NetworkPolicySpec{
		PodSelector: metav1.LabelSelector{
			MatchLabels: generateConsoleSelectorLabels(),
		},
		Ingress: []networkingv1.NetworkPolicyIngressRule{
			{
				From: []networkingv1.NetworkPolicyPeer{
					{
						NamespaceSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"kubernetes.io/metadata.name": "openshift-console",
							},
						},
						PodSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"app": "console",
							},
						},
					},
				},
				Ports: []networkingv1.NetworkPolicyPort{
					{
						Protocol: toPtr(corev1.ProtocolTCP),
						Port:     toPtr(intstr.FromInt32(ConsoleUIHTTPSPort)),
					},
				},
			},
		},
		PolicyTypes: []networkingv1.PolicyType{
			networkingv1.PolicyTypeIngress,
		},
	}
}
