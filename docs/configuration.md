# Configuration

Everything is configured through the `OpenStackLightspeed` custom
resource (`lightspeed.openstack.org/v1beta1`). This page documents every
field in its `spec`.

## Core fields

| Field | Required | Description |
|-------|----------|-------------|
| `defaultModel` | Yes | Default model alias selected for inference. Must match one of `models[].name`. |
| `models` | Yes | List of configured models. Must contain at least one entry. |
| `tlsCACertBundle` | No | Name of a `ConfigMap` containing additional CA certificates to merge into the shared trust bundle used by application components, including model connections. |

## Models (`models[]`)

Each item in `models[]` defines one selectable model alias and the
provider/backend details used to serve it.

| Field | Required | Description |
|-------|----------|-------------|
| `models[].name` | Yes | Kubernetes-style model alias (used by `defaultModel` and request-time `model`). |
| `models[].llmEndpoint` | Yes | URL of the LLM endpoint (e.g. `https://api.openai.com/v1`). Must start with `http://` or `https://`. |
| `models[].llmEndpointType` | Yes | Provider type. See [supported providers](#supported-providers). |
| `models[].llmCredentials` | Yes | `Secret` name (same namespace) with the API token under key `apitoken`. |
| `models[].modelName` | Yes | Provider-native model name to use at the configured endpoint. |
| `models[].maxTokensForResponse` | No | Max response tokens for this model. Minimum `1`. Defaults to `2048`. |
| `models[].llmProjectID` | No | Required by some providers (e.g. WatsonX). |
| `models[].llmDeploymentName` | No | Required by some providers (e.g. Azure OpenAI). |
| `models[].llmAPIVersion` | No | Required by some providers (e.g. Azure OpenAI). |

See [Multi-model request routing](usage.md#multi-model-request-routing) for how
model aliases map to request fields (`provider` and `model`) at query time.

## Supported providers

- `openai` — OpenAI-compatible endpoints (Ollama, vLLM, etc.)
- `azure_openai` — Azure OpenAI (needs `models[].llmDeploymentName`, `models[].llmAPIVersion`)
- `watsonx` — IBM watsonx.ai (needs `models[].llmProjectID`)
- `rhoai_vllm` — vLLM via Red Hat OpenShift AI
- `rhelai_vllm` — vLLM via RHEL AI
- `gemini` — Google Gemini

> [!TIP]
> This list grows over time. Check
> `oc explain openstacklightspeed.spec.models.llmEndpointType` on your cluster
> for the current, authoritative list.

## Logging

| Field | Default | Description |
|-------|---------|-------------|
| `ogx.logLevel` | `all=info` | OGX container. Standard level, or `component=level` pairs (e.g. `core=debug,providers=info`). |
| `lcore.logLevel` | `INFO` | lightspeed-service-api container. `DEBUG`/`INFO`/`WARNING`/`ERROR`/`CRITICAL`. |
| `dataverseExporter.logLevel` | `INFO` | Feedback/transcript exporter sidecar. Same values as above. |
| `database.logLevel` | `INFO` | PostgreSQL container. `DEBUG` also logs every SQL statement. |

## Responses request size

`lcore.responsesRequestMaxSize` sets the maximum character length of the
serialized Responses API request body. It must be at least `1`. When omitted,
the operator leaves the setting out of the generated configuration, allowing
Lightspeed Stack to use its default of `65536` characters.

```yaml
spec:
  lcore:
    responsesRequestMaxSize: 262144
```

The operator maps this field to `customization.responses_request_max_size`
in the Lightspeed Stack configuration. Use a LCore container image that
supports this setting. This request size limit is separate from
`models[].maxTokensForResponse`, which controls response tokens.

## Data collection

```yaml
spec:
  dataverseExporter:
    feedback:
      enabled: true       # default: true
    transcripts:
      enabled: false      # default: false
```

`feedback.enabled` records thumbs-up/down responses. `transcripts.enabled`
records full conversations. Both are sent by the Dataverse exporter sidecar.

## Persistent storage (`database`)

PostgreSQL always gets a PersistentVolumeClaim — this field only overrides
its size/class, it doesn't control whether one exists:

```yaml
spec:
  database:
    size: "5Gi"                # default: 1Gi
    class: "my-storage-class"  # default: cluster's default StorageClass
```

## Container resources

Every container has a default request/limit. Setting one replaces its
default entirely:

```yaml
spec:
  ogx:
    resources:
      requests: {cpu: "500m", memory: "2Gi"}
      limits: {cpu: "2", memory: "8Gi"}
  lcore:
    resources:
      requests: {cpu: "250m", memory: "512Mi"}
      limits: {cpu: "1", memory: "2Gi"}
  database:
    resources:
      requests: {cpu: "30m", memory: "300Mi"}
      limits: {cpu: "500m", memory: "2Gi"}
  okp:
    resources:
      requests: {cpu: "500m", memory: "2Gi"}
      limits: {cpu: "2", memory: "4Gi"}
```

## Container images

Each managed workload can use a custom image. Set `containerImage` under the
relevant component: `rag`, `ogx`, `lcore`, `database`, `dataverseExporter`,
or `okp`. When omitted, the operator uses its configured
default image. For example, to configure LCORE container image:

```yaml
spec:
  lcore:
    containerImage: quay.io/<custom-org>/<custom-image-name>:<tag>
```

## Offline knowledge portal

> [!IMPORTANT]
> OKP is deployed on **every** install and always exposed through the OGX
> `file-search` tool — `spec.okp` configures it, it doesn't gate whether
> it's deployed. Pulling its image needs the same free `registry.redhat.io`
> account described in the [installation guide](install_guide.md#access-to-registry-images).

```yaml
spec:
  okp: {}   # no access key: browse individual pages, full-text search doesn't work
```

```yaml
spec:
  okp:
    accessKey: okp-access-key-secret   # Secret key: "access_key"
    offline: true                      # default: resolve documentation URLs offline
```

- **No `accessKey`** (default) — you can navigate directly to and read
  individual documentation and product lifecycle pages. The full-text
  search index, Solutions, and Articles are encrypted and require a key,
  so keyword search across the corpus doesn't work. What upstream users
  run on.
- **With `accessKey`** — unlocks that search index plus the encrypted
  knowledgebase. Needs an active Red Hat Satellite subscription ([get one](https://access.redhat.com/offline/access)) — a bonus if you already
  have one, not something every user needs.

By default, **RAG grounding is OKP-only** — the bundled community
documentation is disabled unless you set `dev.okpRagOnly: false` (below).

## Quota enforcement

Configure one or more limiters to enable token quota enforcement. The
operator uses its managed PostgreSQL instance for quota storage. Omitting
`quotas` or leaving `limiters` empty disables enforcement.

```yaml
spec:
  quotas:
    limiters:
      - name: per-user-hourly
        type: userLimiter
        initialQuota: 1000
        quotaIncrease: 1000
        period: "1 hour"
      - name: cluster-daily
        type: clusterLimiter
        initialQuota: 100000
        quotaIncrease: 100000
        period: "1 day"
    scheduler:
      period: 10
    enableTokenHistory: true
```

Each entry in `limiters` requires these fields:

| Field | Description |
|-------|-------------|
| `name` | A human-readable limiter name. |
| `type` | `userLimiter` for a per-user quota, or `clusterLimiter` for one quota shared by the cluster. |
| `initialQuota` | Number of tokens granted when the limiter resets. Must be zero or greater. |
| `quotaIncrease` | Number of tokens added by the scheduler at each quota interval. Must be zero or greater. |
| `period` | Interval that controls when the limiter resets or increases, such as `"30 seconds"`, `"1 hour"`, `"1 day"`, or `"1 hour 30 minutes"`. |

`scheduler` is optional and configures the background process that checks
limiters for reset or increase and reconnects to the database after a
connection failure:

- `period`: check interval in seconds. Default: `5`.
- `databaseReconnectionCount`: number of database reconnection attempts.
  Default: `10`.
- `databaseReconnectionDelay`: delay in seconds between reconnection
  attempts. Default: `1`.

Set `enableTokenHistory: true` to record per-user, model, and provider token
usage for auditing. It does not affect enforcement and defaults to `false`.

## Developer / experimental options (`dev`)

> [!WARNING]
> Not part of the stable API — may change without notice.

```yaml
spec:
  dev:
    okpChunkFilterQuery: "product:(*openstack* OR *openshift*)"  # example override
    okpRagOnly: false  # include bundled community docs too, not just OKP
    resourcePollInterval: 60  # requeue/poll interval in seconds (default: 60)
```

- `okpChunkFilterQuery` overrides the version-aware knowledge-base filter.
  If unset, the operator detects your OpenShift/RHOSO versions.
- `okpRagOnly` controls whether the bundled community documentation is used
  alongside OKP. Defaults to `true`.
- `resourcePollInterval` sets the requeue interval in seconds while waiting
  for deployments or other resources to become ready. Defaults to `60`.
