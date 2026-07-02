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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	consolev1 "github.com/openshift/api/console/v1"
	apiv1beta1 "github.com/openstack-k8s-operators/lightspeed-operator/api/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
)

var _ = Describe("Console Plugin", func() {

	BeforeEach(func() {
		// Set up defaults so the builder functions have image URLs
		apiv1beta1.SetupDefaults()
	})

	Describe("generateConsoleSelectorLabels", func() {
		It("should return the expected labels", func() {
			labels := generateConsoleSelectorLabels()
			Expect(labels).To(HaveKeyWithValue("app.kubernetes.io/component", "console-plugin"))
			Expect(labels).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "openstack-lightspeed-operator"))
			Expect(labels).To(HaveKeyWithValue("app.kubernetes.io/name", "lightspeed-console-plugin"))
			Expect(labels).To(HaveKeyWithValue("app.kubernetes.io/part-of", "openstack-lightspeed"))
		})
	})

	Describe("buildConsoleDeploymentSpec", func() {
		var spec appsv1.DeploymentSpec

		BeforeEach(func() {
			spec = buildConsoleDeploymentSpec(apiv1beta1.OpenStackLightspeedDefaultValues.ConsoleImagePF5URL)
		})

		It("should have one replica", func() {
			Expect(spec.Replicas).NotTo(BeNil())
			Expect(*spec.Replicas).To(Equal(int32(1)))
		})

		It("should have correct selector labels", func() {
			Expect(spec.Selector.MatchLabels).To(Equal(generateConsoleSelectorLabels()))
		})

		It("should have one container with the console image", func() {
			containers := spec.Template.Spec.Containers
			Expect(containers).To(HaveLen(1))
			Expect(containers[0].Name).To(Equal("lightspeed-console-plugin"))
			Expect(containers[0].Image).To(Equal(apiv1beta1.OpenStackLightspeedDefaultValues.ConsoleImagePF5URL))
		})

		It("should expose HTTPS port 9443", func() {
			ports := spec.Template.Spec.Containers[0].Ports
			Expect(ports).To(HaveLen(1))
			Expect(ports[0].ContainerPort).To(Equal(ConsoleUIHTTPSPort))
			Expect(ports[0].Name).To(Equal("https"))
			Expect(ports[0].Protocol).To(Equal(corev1.ProtocolTCP))
		})

		It("should have TLS cert, nginx config, nginx temp, and locales-rewrite volume mounts", func() {
			mounts := spec.Template.Spec.Containers[0].VolumeMounts
			Expect(mounts).To(HaveLen(4))

			var names []string
			for _, m := range mounts {
				names = append(names, m.Name)
			}
			Expect(names).To(ContainElements("lightspeed-console-plugin-cert", "nginx-config", "nginx-temp", "locales-rewrite"))
		})

		It("should mount locales-rewrite with SubPath at the locales file path", func() {
			mounts := spec.Template.Spec.Containers[0].VolumeMounts
			var found bool
			for _, m := range mounts {
				if m.Name == "locales-rewrite" {
					found = true
					Expect(m.MountPath).To(Equal(consoleLocalesPath))
					Expect(m.SubPath).To(Equal(consoleLocalesFilename))
					Expect(m.ReadOnly).To(BeTrue())
				}
			}
			Expect(found).To(BeTrue())
		})

		It("should have TLS cert volume from secret", func() {
			volumes := spec.Template.Spec.Volumes
			var found bool
			for _, v := range volumes {
				if v.Name == "lightspeed-console-plugin-cert" {
					found = true
					Expect(v.VolumeSource.Secret).NotTo(BeNil())
					Expect(v.VolumeSource.Secret.SecretName).To(Equal(ConsoleUIServiceCertSecretName))
				}
			}
			Expect(found).To(BeTrue())
		})

		It("should have nginx config volume from configmap", func() {
			volumes := spec.Template.Spec.Volumes
			var found bool
			for _, v := range volumes {
				if v.Name == "nginx-config" {
					found = true
					Expect(v.VolumeSource.ConfigMap).NotTo(BeNil())
					Expect(v.VolumeSource.ConfigMap.Name).To(Equal(ConsoleUIConfigMapName))
				}
			}
			Expect(found).To(BeTrue())
		})

		It("should have nginx temp emptyDir volume", func() {
			volumes := spec.Template.Spec.Volumes
			var found bool
			for _, v := range volumes {
				if v.Name == "nginx-temp" {
					found = true
					Expect(v.VolumeSource.EmptyDir).NotTo(BeNil())
				}
			}
			Expect(found).To(BeTrue())
		})

		It("should use the console service account", func() {
			Expect(spec.Template.Spec.ServiceAccountName).To(Equal(ConsoleUIServiceAccountName))
		})

		It("should have a locales-rewrite emptyDir volume", func() {
			volumes := spec.Template.Spec.Volumes
			var found bool
			for _, v := range volumes {
				if v.Name == "locales-rewrite" {
					found = true
					Expect(v.VolumeSource.EmptyDir).NotTo(BeNil())
				}
			}
			Expect(found).To(BeTrue())
		})

		It("should have one init container for rewriting locales", func() {
			initContainers := spec.Template.Spec.InitContainers
			Expect(initContainers).To(HaveLen(1))
			Expect(initContainers[0].Name).To(Equal("rewrite-locales"))
		})

		It("should use the same console image for the init container", func() {
			initContainer := spec.Template.Spec.InitContainers[0]
			mainContainer := spec.Template.Spec.Containers[0]
			Expect(initContainer.Image).To(Equal(mainContainer.Image))
		})

		It("should have the init container command with awk for text replacement", func() {
			initContainer := spec.Template.Spec.InitContainers[0]
			Expect(initContainer.Command).To(HaveLen(3))
			Expect(initContainer.Command[0]).To(Equal("sh"))
			Expect(initContainer.Command[1]).To(Equal("-c"))
			cmd := initContainer.Command[2]
			Expect(cmd).To(ContainSubstring("awk"))
			Expect(cmd).To(ContainSubstring("OpenShift"))
			Expect(cmd).To(ContainSubstring("OpenStack"))
			Expect(cmd).To(ContainSubstring(consoleLocalesPath))
			Expect(cmd).To(ContainSubstring("/locales-rewrite/" + consoleLocalesFilename))
		})

		It("should mount locales-rewrite volume in the init container", func() {
			initContainer := spec.Template.Spec.InitContainers[0]
			Expect(initContainer.VolumeMounts).To(HaveLen(1))
			Expect(initContainer.VolumeMounts[0].Name).To(Equal("locales-rewrite"))
			Expect(initContainer.VolumeMounts[0].MountPath).To(Equal("/locales-rewrite"))
		})
	})

	Describe("buildConsolePluginSpec", func() {
		const testNamespace = "test-ns"
		var spec = buildConsolePluginSpec(testNamespace)

		It("should have service backend", func() {
			Expect(spec.Backend.Type).To(Equal(consolev1.Service))
			Expect(spec.Backend.Service).NotTo(BeNil())
			Expect(spec.Backend.Service.Name).To(Equal(ConsoleUIServiceName))
			Expect(spec.Backend.Service.Namespace).To(Equal(testNamespace))
			Expect(spec.Backend.Service.Port).To(Equal(ConsoleUIHTTPSPort))
		})

		It("should have proxy to lightspeed app server", func() {
			Expect(spec.Proxy).To(HaveLen(1))
			proxy := spec.Proxy[0]
			Expect(proxy.Alias).To(Equal(ConsoleProxyAlias))
			Expect(proxy.Authorization).To(Equal(consolev1.UserToken))
			Expect(proxy.Endpoint.Type).To(Equal(consolev1.ProxyTypeService))
			Expect(proxy.Endpoint.Service).NotTo(BeNil())
			Expect(proxy.Endpoint.Service.Name).To(Equal(OpenStackLightspeedAppServerServiceName))
			Expect(proxy.Endpoint.Service.Namespace).To(Equal(testNamespace))
			Expect(proxy.Endpoint.Service.Port).To(Equal(int32(OpenStackLightspeedAppServerServicePort)))
		})

		It("should have display name and i18n", func() {
			Expect(spec.DisplayName).To(Equal("Lightspeed Console Plugin"))
			Expect(spec.I18n.LoadType).To(Equal(consolev1.Preload))
		})
	})

	Describe("buildConsoleNginxConfig", func() {
		It("should contain SSL listener on port 9443", func() {
			config := buildConsoleNginxConfig()
			Expect(config).To(ContainSubstring("listen              9443 ssl"))
			Expect(config).To(ContainSubstring("ssl_certificate     /var/cert/tls.crt"))
			Expect(config).To(ContainSubstring("ssl_certificate_key /var/cert/tls.key"))
		})
	})

	Describe("buildConsoleNetworkPolicySpec", func() {
		var spec = buildConsoleNetworkPolicySpec()

		It("should select console plugin pods", func() {
			Expect(spec.PodSelector.MatchLabels).To(Equal(generateConsoleSelectorLabels()))
		})

		It("should allow ingress from openshift-console namespace", func() {
			Expect(spec.Ingress).To(HaveLen(1))
			Expect(spec.Ingress[0].From).To(HaveLen(1))
			nsSelector := spec.Ingress[0].From[0].NamespaceSelector
			Expect(nsSelector).NotTo(BeNil())
			Expect(nsSelector.MatchLabels).To(HaveKeyWithValue("kubernetes.io/metadata.name", "openshift-console"))
		})

		It("should allow ingress on HTTPS port", func() {
			Expect(spec.Ingress[0].Ports).To(HaveLen(1))
			Expect(spec.Ingress[0].Ports[0].Port.IntVal).To(Equal(ConsoleUIHTTPSPort))
		})

		It("should have ingress policy type", func() {
			Expect(spec.PolicyTypes).To(ContainElement(
				networkingv1.PolicyTypeIngress,
			))
		})
	})

	Describe("consoleImageForVersion", func() {
		It("should return PF5 image when version is empty", func() {
			result := consoleImageForVersion("")
			Expect(result).To(Equal(apiv1beta1.OpenStackLightspeedDefaultValues.ConsoleImagePF5URL))
		})

		It("should return PF5 image for OCP 4.16", func() {
			result := consoleImageForVersion("4.16")
			Expect(result).To(Equal(apiv1beta1.OpenStackLightspeedDefaultValues.ConsoleImagePF5URL))
		})

		It("should return PF5 image for OCP 4.18", func() {
			result := consoleImageForVersion("4.18")
			Expect(result).To(Equal(apiv1beta1.OpenStackLightspeedDefaultValues.ConsoleImagePF5URL))
		})

		It("should return PF6 image for OCP 4.19", func() {
			result := consoleImageForVersion("4.19")
			Expect(result).To(Equal(apiv1beta1.OpenStackLightspeedDefaultValues.ConsoleImageURL))
		})

		It("should return PF6 image for OCP 4.20", func() {
			result := consoleImageForVersion("4.20")
			Expect(result).To(Equal(apiv1beta1.OpenStackLightspeedDefaultValues.ConsoleImageURL))
		})

		It("should return PF6 image for OCP 5.0", func() {
			result := consoleImageForVersion("5.0")
			Expect(result).To(Equal(apiv1beta1.OpenStackLightspeedDefaultValues.ConsoleImageURL))
		})

		It("should return PF5 image for non-numeric version parts", func() {
			result := consoleImageForVersion("abc.def")
			Expect(result).To(Equal(apiv1beta1.OpenStackLightspeedDefaultValues.ConsoleImagePF5URL))
		})

		It("should return PF5 image for single-part version string", func() {
			result := consoleImageForVersion("4")
			Expect(result).To(Equal(apiv1beta1.OpenStackLightspeedDefaultValues.ConsoleImagePF5URL))
		})
	})
})
