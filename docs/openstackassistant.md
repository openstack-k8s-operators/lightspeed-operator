# Testing OpenStackAssistant with OpenStack Lightspeed

This guide deploys OpenStack Lightspeed and an OpenStack Assistant instance so
you can run Goose inside the cluster and try its OpenStack and OpenShift MCP
tools.

> [!IMPORTANT]
> Before starting, you need a working RHOSO deployment. The MCP tools in this
> guide expect resources from the deployed OpenStack control plane and are not
> available without RHOSO.

## 1. Deploy the OpenStack Lightspeed operator

From the lightspeed-operator repository, deploy the operator using the adjacent
`install_yamls` checkout:

```bash
(cd ../install_yamls && make openstack_lightspeed)
```

## 2. Create the Nemotron credentials and OpenStackLightspeed instance

Create the API token Secret in the `openstack-lightspeed` namespace. Replace
the redacted value locally with the token for your endpoint; do not commit the
real token or add it to this document.

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: nemotron-super-120b-bf16-ga-secret
  namespace: openstack-lightspeed
type: Opaque
stringData:
  apitoken: REDACTED
```

The endpoint uses Red Hat's internal CA. Create the `redhat-internal-ca`
ConfigMap in `openstack-lightspeed` with the approved CA bundle in PEM format.
Keep the CA certificate material in your local secure configuration; do not
put it in this guide or commit it.

Then create the `OpenStackLightspeed` resource. The API token and CA contents
are intentionally omitted here. The `rhoso_mcps` feature flag enables the
read-only RHOSO MCP server.

```yaml
apiVersion: lightspeed.openstack.org/v1beta1
kind: OpenStackLightspeed
metadata:
  name: nemotron-super-120b-bf16-ga
  namespace: openstack-lightspeed
spec:
  dev:
    featureFlags:
      - rhoso_mcps
    okpChunkFilterQuery: "is_chunk:true"
  tlsCACertBundle: redhat-internal-ca
  llmEndpoint: <fixme>
  llmEndpointType: rhelai_vllm
  llmCredentials: nemotron-super-120b-bf16-ga-secret
  modelName: 'nvidia/NVIDIA-Nemotron-3-Super-120B-A12B-BF16'
```

Prepare `redhat-internal-ca.yaml` locally from the approved CA bundle and keep
that file out of version control. Apply the Secret, CA ConfigMap, and custom
resource from the lightspeed-operator repository root:

```bash
oc apply -f nemotron-secret.yaml
oc apply -f redhat-internal-ca.yaml
oc apply -f nemotron-openstacklightspeed.yaml
```

## 3. Wait for OpenStack Lightspeed to become ready

Wait for the custom resource to report `Ready`, then check its pods and
conditions for errors before proceeding:

```bash
oc wait -n openstack-lightspeed \
  openstacklightspeed/nemotron-super-120b-bf16-ga \
  --for=condition=Ready --timeout=20m
oc get -n openstack-lightspeed deployments,pods
oc describe -n openstack-lightspeed \
  openstacklightspeed/nemotron-super-120b-bf16-ga
```

Resolve any failing conditions, pod errors, or image pull issues before
creating the assistant.

## 4. Deploy OpenStackAssistant

The `openstack_assistant` target creates the `OpenStackAssistant` custom
resource and its supporting configuration. It expects the RHOSO
`OpenStackClient` resource named `openstackclient` in the `openstack`
namespace. The target checks that the OpenStackClient MCP endpoint is enabled
and enables it through the owning `OpenStackControlPlane` if needed.

```bash
(cd ../install_yamls && make openstack_assistant)
```

Confirm the assistant and its pod are ready before entering the pod:

```bash
oc get -n openstack openstackassistant/openstack-assistant
oc get -n openstack pod/openstack-assistant
```

## 5. Start Goose in the assistant pod

Enter the pod:

```bash
oc rsh -n openstack openstack-assistant
```

Then launch Goose in the pod:

```bash
goose
```

You can use interactive chat directly or run a pre-configured recipe with:

```bash
/cluster-health
```

The assistant can use the read-only OpenStack MCP server running alongside
OpenStackClient in its `mcp-server` sidecar. A Kubernetes NetworkPolicy protects
that MCP connection. The Goose developer extension also provides `oc` tools;
those run under a locked-down service account created by openstack-operator and
are restricted to read-only commands.
