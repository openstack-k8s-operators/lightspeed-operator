# Configuration

Everything is configured through the `OpenStackLightspeed` custom
resource (`lightspeed.openstack.org/v1beta1`). This page documents every
field in its `spec`.

## Core fields

| Field | Required | Description |
|-------|----------|-------------|
| `llmEndpoint` | Yes | URL of the LLM endpoint (e.g. `https://api.openai.com/v1`). Must start with `http://` or `https://`. |
| `llmEndpointType` | Yes | Provider type. See [supported providers](configuration.md#supported-providers). |
| `modelName` | Yes | Model name to use at `llmEndpoint`. |
| `llmCredentials` | Yes | `Secret` name (same namespace) with the API token under key `apitoken`. |
| `tlsCACertBundle` | No | `ConfigMap` name (same namespace) with a CA bundle for the LLM endpoint. |
| `maxTokensForResponse` | No | Max response tokens. Minimum `1`. Defaults to `2048`. |
| `llmProjectID` | No | Required by some providers (e.g. WatsonX). |
| `llmDeploymentName` | No | Required by some providers (e.g. Azure OpenAI). |
| `llmAPIVersion` | No | Required by some providers (e.g. Azure OpenAI). |

## Supported providers

- `openai` — OpenAI-compatible endpoints (Ollama, vLLM, etc.)
- `azure_openai` — Azure OpenAI (needs `llmDeploymentName`, `llmAPIVersion`)
- `watsonx` — IBM watsonx.ai (needs `llmProjectID`)
- `rhoai_vllm` — vLLM via Red Hat OpenShift AI
- `rhelai_vllm` — vLLM via RHEL AI
- `gemini` — Google Gemini

> [!TIP]
> This list grows over time. Check
> `oc explain openstacklightspeed.spec.llmEndpointType` on your cluster
> for the current, authoritative list.

## Logging

| Field | Default | Description |
|-------|---------|-------------|
| `ogx.logLevel` | `all=info` | OGX container. Standard level, or `component=level` pairs (e.g. `core=debug,providers=info`). |
| `lcore.logLevel` | `INFO` | lightspeed-service-api container. `DEBUG`/`INFO`/`WARNING`/`ERROR`/`CRITICAL`. |
| `dataverseExporter.logLevel` | `INFO` | Feedback/transcript exporter sidecar. Same values as above. |
| `database.logLevel` | `INFO` | PostgreSQL container. `DEBUG` also logs every SQL statement. |

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
  console:
    resources:
      requests: {cpu: "50m", memory: "64Mi"}
      limits: {cpu: "200m", memory: "256Mi"}
```

The optional RHOSO MCP sidecar has default resources of `50m` CPU and `300Mi`
memory requested, with a `500Mi` memory limit. Configure it at
`dev.rhosMCP.resources` when the `rhoso_mcps` feature flag is enabled.

## Container images

Each managed workload can use a custom image. Set `containerImage` under the
relevant component: `rag`, `ogx`, `lcore`, `database`, `dataverseExporter`,
`okp`, or `console`; for the optional MCP sidecar use
`dev.rhosMCP.containerImage`. When omitted, the operator uses its configured
default image. For example, to configure LCORE container image:

```yaml
spec:
  lcore:
    containerImage: quay.io/<custom-org>/<custom-image-name>:<tag>
```

## Offline knowledge portal

> [!IMPORTANT]
> OKP is deployed on **every** install — `spec.okp` configures it, it
> doesn't gate whether it's deployed. Pulling its image needs the same
> free `registry.redhat.io` account described in the [installation guide](install_guide.md#access-to-registry-images).

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
    featureFlags:
      - rhoso_mcps   # enables the read-only MCP introspection sidecar
    okpChunkFilterQuery: "product:(*openstack* OR *openshift*)"  # example override
    okpRagOnly: false  # include bundled community docs too, not just OKP
    rhosMCP:
      config: |
        debug: true
        workers: 4
      resources:
        requests:
          cpu: "50m"
          memory: "300Mi"
        limits:
          memory: "500Mi"
      containerImage: quay.io/openstack-lightspeed/lightspeed-mcps:latest
```

- `okpChunkFilterQuery` and `okpRagOnly` take effect immediately, with
  no `featureFlags` entry needed — they're independent of
  `rhoso_mcps`. If unset, `okpChunkFilterQuery` auto-detects your
  OpenShift/RHOSO versions instead of using the literal example above.
- `rhoso_mcps` — the one flag that does need to be set. Deploys the MCP
  introspection sidecar, which is read-only **by default**. See
  [Usage](usage.md).
- `rhosMCP` configures the MCP sidecar. Its `config` value is deep-merged
  on top of the operator's defaults and can override anything they set,
  including the `allow_write` flags that keep introspection read-only. Only
  set it if you understand exactly what you're overriding. `resources` and
  `containerImage` respectively configure the sidecar resource requirements
  and image.
