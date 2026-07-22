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
)

// OpenStackLightspeed Condition Types used by API objects.
const (
	// OpenStackLightspeedReadyCondition Status=True condition which indicates if OpenStackLightspeedReadyCondition
	// is configured and operational
	OpenStackLightspeedReadyCondition condition.Type = "OpenStackLightspeedReady"

	// OpenShift Lightspeed Operator Status=True condition which indicates if OpenShift Lightspeed is installed and
	// operational and it can be used by OpenStack Lightspeed operator.
	OpenShiftLightspeedOperatorReadyCondition condition.Type = "OpenShiftLightspeedOperatorReady"

	// OpenStackLightspeedMCPServerReadyCondition is set to True when the MCP server
	// deployment succeeds. False indicates a failure during MCP server deployment.
	OpenStackLightspeedMCPServerReadyCondition condition.Type = "OpenStackLightspeedMCPServerReady"
)

// Common Messages used by API objects.
const (
	// OpenStackLightspeedReadyInitMessage
	OpenStackLightspeedReadyInitMessage = "OpenStack Lightspeed not started"

	// OpenStackLightspeedReadyMessage
	OpenStackLightspeedReadyMessage = "OpenStack Lightspeed created"

	// OpenStackLightspeedWaitingVectorDBMessage
	OpenStackLightspeedWaitingVectorDBMessage = "Waiting for OpenStackLightspeed vector DB pod to become ready"

	// OpenShiftLightspeedOperatorWaiting
	OpenShiftLightspeedOperatorWaiting = "Waiting for the OpenShift Lightspeed operator to deploy."

	// OpenShiftLightspeedOperatorReady
	OpenShiftLightspeedOperatorReady = "OpenShift Lightspeed operator is ready."

	// OpenStackLightspeedMCPServerInitMessage
	OpenStackLightspeedMCPServerInitMessage = "MCP server deployment has not resolved"

	// OpenStackLightspeedMCPServerDeployed
	OpenStackLightspeedMCPServerDeployed = "MCP server is ready"

	// OpenStackLightspeedMCPServerWaitingOpenStack
	OpenStackLightspeedMCPServerWaitingOpenStack = "MCP server deployed, waiting for OpenStackControlPlane to become ready"

	// OpenStackLightspeedMCPServerCreatingUser
	OpenStackLightspeedMCPServerCreatingUser = "Creating OpenStack service user"

	// OpenStackLightspeedMCPServerWaitingAC
	OpenStackLightspeedMCPServerWaitingAC = "Waiting for application credential secret"

	// OpenStackLightspeedMCPServerDisabledMessage
	OpenStackLightspeedMCPServerDisabledMessage = "RHOS MCP server is disabled (rhos_mcps feature flag not set)"

	// DeploymentCheckFailedMessage
	DeploymentCheckFailedMessage = "Failed to check deployment status: %s"

	// DeploymentsNotReadyMessage
	DeploymentsNotReadyMessage = "Waiting for deployments to be ready: %s"
)
