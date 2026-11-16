# Installation Guide

This page covers prerequisites, installing the operator, setting up LLM
credentials, and deploying `OpenStackLightspeed`. No cluster yet? See
[local CRC setup](development.md#local-cluster-with-crc).

## Prerequisites

- An OpenShift cluster (4.16+).

- An LLM endpoint and API key — any [supported provider](configuration.md#supported-providers)
  works.
- A free Red Hat Developer account, to pull some images from
  `registry.redhat.io` — see [registry access](install_guide.md#access-to-registry-images)
  below.

## Access to registry images

The OKP image is deployed on every install and comes from
`registry.redhat.io` rather than `quay.io`. This requires a **free**
account you can create by following these steps:

1. Create a free account at [developers.redhat.com](https://developers.redhat.com/).
2. Download a pull secret from the [Hybrid Cloud Console](https://console.redhat.com/).
3. Add it to your cluster:

   - **CRC**: pass it as `PULL_SECRET` when creating the cluster — see
     [local CRC setup](development.md#local-cluster-with-crc).
   - **Existing cluster**: merge it into the cluster-wide pull secret:

     ```bash
     oc get secret/pull-secret -n openshift-config -o jsonpath='{.data.\.dockerconfigjson}' \
       | base64 -d > pull-secret.json
     # merge the downloaded auths into pull-secret.json, then:
     oc set data secret/pull-secret -n openshift-config \
       --from-file=.dockerconfigjson=pull-secret.json
     ```

4. Verify the cluster itself can pull, using its own pull secret:

   ```console
   $ oc run registry-pull-test --image=registry.redhat.io/offline-knowledge-portal/rhokp-rhel9@sha256:576abe26ace61e70c077ca45bbb7c754ae3e1579b3122a09ea97ca311b3c8c3f --restart=Never
   $ oc get pod registry-pull-test
   ```

   Any status other than `ImagePullBackOff`/`ErrImagePull` means it's
   working — clean up with `oc delete pod registry-pull-test`. If you do
   see it, the secret from the previous step didn't propagate — see
   [Image pull troubleshooting](troubleshooting.md#imagepullbackoff-on-any-operator-managed-pod).

## Installing the operator

1. **Operators → OperatorHub**, search for **"OpenStack Lightspeed
   (Community)"**.
2. Click **Install**, choosing the `openstack-lightspeed` namespace.
3. Track progress under **Operators → Installed Operators**, or:

   ```console
   $ oc get -n openstack-lightspeed pods
   NAME                                                              READY   STATUS    RESTARTS   AGE
   openstack-lightspeed-operator-controller-manager-76df7fbfb5wggr   1/1     Running   0          72s
   ```

> [!NOTE]
> Currently published for OpenShift 4.16 and 4.18 specifically — on
> other versions it won't appear in OperatorHub search. Use the
> alternative below instead.

**Alternative — deploy from source** (for testing an unreleased build, or
if your OpenShift version isn't in the catalog yet):

```bash
git clone https://github.com/openstack-k8s-operators/lightspeed-operator.git
cd lightspeed-operator
make openstack-lightspeed-deploy
```

This sets up its own `CatalogSource`, namespace, and `Subscription` —
bypassing OperatorHub entirely.

## Setting up LLM credentials

You need an API key, endpoint URL, and model name.

Create the API key secret — the key **must** be named `apitoken`:

```bash
oc apply -f - <<EOF
apiVersion: v1
kind: Secret
type: Opaque
metadata:
  name: openstack-lightspeed-apitoken
  namespace: openstack-lightspeed
stringData:
  apitoken: <your-llm-api-key>
EOF
```

Using a self-hosted endpoint with a self-signed certificate (e.g. vLLM,
Ollama)? Add its CA bundle too — any key name works, PEM data is all
that's parsed:

```bash
oc apply -f - <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: openstack-lightspeed-certs
  namespace: openstack-lightspeed
data:
  cert: |
$(sed 's/^/    /' /path/to/cert.crt)
EOF
```

Public providers (Gemini, OpenAI, etc.) don't need this — skip straight to
the next step.

## Deploying OpenStackLightspeed

At minimum, set `defaultModel` and one entry in `models[]` with
`llmEndpoint`, `llmEndpointType`, `modelName`, and
`llmCredentials` — see [Configuration](configuration.md) for the full list of
supported `models[].llmEndpointType` values and everything else:

```yaml
apiVersion: lightspeed.openstack.org/v1beta1
kind: OpenStackLightspeed
metadata:
  name: openstack-lightspeed
  namespace: openstack-lightspeed
spec:
  tlsCACertBundle: openstack-lightspeed-certs # optional
  defaultModel: my-model
  models:
    - name: my-model
      llmEndpoint: https://<llm-provider-host>:<port>/v1
      llmEndpointType: <provider-type>
      llmCredentials: openstack-lightspeed-apitoken
      modelName: <model-name>
```

This deploys the full stack: the AI engine (lightspeed-stack and
OGX), PostgreSQL, and OKP.

> [!NOTE]
> Only `OwnNamespace` installations are supported; therefore, you can create
> `OpenStackLightspeed` only in the namespace where the operator is installed.
> Additionally, no more than one `OpenStackLightspeed` instance is allowed. If
> you create more than one `OpenStackLightspeed` instance within the namespace
> where the operator is installed, it will be rejected.

## Verifying the deployment

```bash
oc describe -n openstack-lightspeed openstacklightspeed
oc get -n openstack-lightspeed deployments,pods
```

Not reaching `Ready`? See [Troubleshooting](troubleshooting.md).

> [!NOTE]
> When collecting metrics through telemetry-operator, wait for
> `OpenStackLightspeed` to become `Ready` before enabling `MetricStorage` through
> the `OpenStackControlPlane`. If metric storage was enabled first, Lightspeed
> metrics may not appear until `MetricStorage` reconciles again, even if it
> reports `Ready`. See [Telemetry metrics discovery](development.md#telemetry-metrics-discovery)
> for details.

## Accessing the assistant

Goose CLI is the primary supported interface for RHOSO 19 Beta.
See [Usage](usage.md) for available features and model selection.

## Accessing the API

API requests are authenticated with an OpenShift access token. The OpenShift
identity represented by that token must be assigned the `lightspeed-access`
ClusterRole. For example, grant the current user access:

```bash
oc adm policy add-cluster-role-to-user lightspeed-access "$(oc whoami)"
```

To call the API from your workstation, forward the service port in one terminal:

```bash
oc -n openstack-lightspeed port-forward service/lightspeed-app-server 8443:8443
```

Then, in another terminal, use the current OpenShift token to submit a query:

```bash
curl --insecure \
  --header "Authorization: Bearer $(oc whoami -t)" \
  --header 'Content-Type: application/json' \
  --data '{"query":"How can I check Nova services?"}' \
  https://localhost:8443/v1/query
```

`--insecure` is needed here because the service certificate is issued for the
in-cluster service DNS name, not `localhost`. For a service account or another
user, bind `lightspeed-access` to that identity and use its token instead.
