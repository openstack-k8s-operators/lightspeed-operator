# Available Features

Day-to-day use: asking questions, optional cluster introspection, and
feedback/transcripts.

## Asking questions

Open the Lightspeed widget (bottom-right corner of the OpenShift console)
once `OpenStackLightspeed` is `Ready`:

- "How can I spin up a VM using the OpenStack CLI?"
- "Why would a Nova compute service show as down?"

Answers are grounded via RAG, with references you can verify. By default,
grounding comes from the [Offline knowledge portal](configuration.md#offline-knowledge-portal) (always deployed, no
credentials needed to browse — see [Configuration](configuration.md) for the free vs.
keyed tiers). The bundled community documentation is also available, but
only if you set `dev.okpRagOnly: false`.

## Cluster introspection (optional)

Enabling the `rhoso_mcps` dev flag ([Configuration](configuration.md)) gives the
assistant read-only tools to inspect your actual OpenStack/OpenShift
resources instead of relying on docs alone.

- **Strictly read-only by default** — only list/get/describe-style
  `openstack` and `oc` commands are exposed as tools; nothing that
  creates, updates, or deletes resources is available to the assistant
  out of the box.
- Introspection stays local to your cluster; only the query and retrieved
  context go to your LLM provider.
- Credentials are automatic — the operator provisions a scoped Keystone
  Application Credential when an `OpenStackControlPlane` is detected.

Disabled by default; still evolving.

## Quota enforcement (optional)

OpenStack Lightspeed can enforce token quotas per user and across the whole
cluster using lightspeed-stack's built-in quota system. The operator manages
the quota storage automatically, so no additional setup is needed.

Quota enforcement is opt-in: it is disabled until you configure at least one
limiter. You can combine per-user and cluster-wide limiters; requests must
satisfy each configured limiter. See [Quota enforcement](configuration.md#quota-enforcement) for the
configuration and an example.

## Feedback and transcripts

- `dataverseExporter.feedback.enabled` (default `true`) — thumbs-up/down on
  responses.
- `dataverseExporter.transcripts.enabled` (default `false`) — full
  conversation transcripts.

Both configured on the CR ([Configuration](configuration.md)). Used to improve answer
quality — disable either if that doesn't fit your data policy.
