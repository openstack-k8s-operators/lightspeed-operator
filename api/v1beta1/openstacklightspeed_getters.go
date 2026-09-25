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

package v1beta1

import "encoding/json"

// ParseDevConfig unmarshals the Dev RawExtension into a DevSpec.
// Returns a zero-value DevSpec and an error on malformed input.
func (instance *OpenStackLightspeed) ParseDevConfig() (DevSpec, error) {
	var devConfig DevSpec
	if len(instance.Spec.Dev.Raw) > 0 {
		if err := json.Unmarshal(instance.Spec.Dev.Raw, &devConfig); err != nil {
			return devConfig, err
		}
	}
	return devConfig, nil
}

func resolveContainerImage(manifestImage, defaultImage string) string {
	if manifestImage != "" {
		return manifestImage
	}
	return defaultImage
}

// RAGContainerImage returns the RAG init-container image for this instance.
func (instance *OpenStackLightspeed) RAGContainerImage() string {
	manifestImage := ""
	if instance.Spec.RAG != nil {
		manifestImage = instance.Spec.RAG.ContainerImage
	}
	return resolveContainerImage(manifestImage, OpenStackLightspeedDefaultValues.RAGImageURL)
}

// OGXContainerImage returns the OGX container image for this instance.
func (instance *OpenStackLightspeed) OGXContainerImage() string {
	manifestImage := ""
	if instance.Spec.OGX != nil {
		manifestImage = instance.Spec.OGX.ContainerImage
	}
	return resolveContainerImage(manifestImage, OpenStackLightspeedDefaultValues.OGXImageURL)
}

// LightspeedContainerImage returns the lightspeed-service-api container image for this instance.
func (instance *OpenStackLightspeed) LightspeedContainerImage() string {
	manifestImage := ""
	if instance.Spec.LCore != nil {
		manifestImage = instance.Spec.LCore.ContainerImage
	}
	return resolveContainerImage(manifestImage, OpenStackLightspeedDefaultValues.LCoreImageURL)
}

// ExporterContainerImage returns the dataverse exporter sidecar container image for this instance.
func (instance *OpenStackLightspeed) ExporterContainerImage() string {
	manifestImage := ""
	if instance.Spec.DataverseExporter != nil {
		manifestImage = instance.Spec.DataverseExporter.ContainerImage
	}
	return resolveContainerImage(manifestImage, OpenStackLightspeedDefaultValues.ExporterImageURL)
}

// PostgresContainerImage returns the PostgreSQL container image for this instance.
func (instance *OpenStackLightspeed) PostgresContainerImage() string {
	manifestImage := ""
	if instance.Spec.Database != nil {
		manifestImage = instance.Spec.Database.ContainerImage
	}
	return resolveContainerImage(manifestImage, OpenStackLightspeedDefaultValues.PostgresImageURL)
}

// OKPContainerImage returns the OKP container image for this instance.
func (instance *OpenStackLightspeed) OKPContainerImage() string {
	manifestImage := ""
	if instance.Spec.OKP != nil {
		manifestImage = instance.Spec.OKP.ContainerImage
	}
	return resolveContainerImage(manifestImage, OpenStackLightspeedDefaultValues.OKPImageURL)
}

// ConsoleContainerImage returns the console plugin container image for this instance.
// When spec.console.containerImage is unset, ocpDefault is used (typically PF5/PF6 selection).
func (instance *OpenStackLightspeed) ConsoleContainerImage(ocpDefault string) string {
	if instance.Spec.Console != nil && instance.Spec.Console.ContainerImage != "" {
		return instance.Spec.Console.ContainerImage
	}
	return ocpDefault
}

// MCPContainerImage returns the MCP container image for this instance.
func (instance *OpenStackLightspeed) MCPContainerImage() string {
	manifestImage := ""

	devConfig, err := instance.ParseDevConfig()
	if err == nil && devConfig.RhosMCP != nil && devConfig.RhosMCP.ContainerImage != "" {
		manifestImage = devConfig.RhosMCP.ContainerImage
	}
	return resolveContainerImage(manifestImage, OpenStackLightspeedDefaultValues.MCPServerImageURL)
}
