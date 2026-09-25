# Quickstart

Already have an OpenShift cluster and an LLM endpoint? Three steps and
you're running. No cluster yet? See [local CRC setup](development.md#local-cluster-with-crc).

## Install the operator

**Operators → OperatorHub**, search **"OpenStack Lightspeed
(Community)"**, click **Install**. Currently published for OpenShift 4.16
and 4.18 — on other versions, or if it's not showing up, see
[Installation Guide](install_guide.md) for the source-based alternative.

## Create the secret and CR

Save as `secret.yaml`, with your own LLM API key:

```yaml
apiVersion: v1
kind: Secret
type: Opaque
metadata:
  name: openstack-lightspeed-apitoken
  namespace: openstack-lightspeed
stringData:
  apitoken: <your-llm-api-key>
```

Save as `cr.yaml`, with your own endpoint, model, and provider type
(see [supported providers](configuration.md#supported-providers) for valid values):

```yaml
apiVersion: lightspeed.openstack.org/v1beta1
kind: OpenStackLightspeed
metadata:
  name: openstack-lightspeed
  namespace: openstack-lightspeed
spec:
  llmEndpoint: https://<llm-provider-host>:<port>/v1
  llmEndpointType: <provider-type>
  llmCredentials: openstack-lightspeed-apitoken
  modelName: <model-name>
```

Then apply both:

```bash
oc apply -f secret.yaml
oc apply -f cr.yaml
```

Self-hosted endpoint with a self-signed certificate? See
[Installation Guide](install_guide.md) and [Configuration](configuration.md) for the full field
reference.

## Open the console

```bash
oc whoami --show-console
```

Open that URL and use the Lightspeed widget (lower-right corner).
