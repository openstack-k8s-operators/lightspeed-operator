Configuration
=============

Everything is configured through the ``OpenStackLightspeed`` custom
resource (``lightspeed.openstack.org/v1beta1``). This page documents every
field in its ``spec``.

Core fields
-----------

.. list-table::
   :header-rows: 1
   :widths: 20 10 70

   * - Field
     - Required
     - Description
   * - ``llmEndpoint``
     - Yes
     - URL of the LLM endpoint (e.g. ``https://api.openai.com/v1``). Must
       start with ``http://`` or ``https://``.
   * - ``llmEndpointType``
     - Yes
     - Provider type. See :ref:`supported-providers`.
   * - ``modelName``
     - Yes
     - Model name to use at ``llmEndpoint``.
   * - ``llmCredentials``
     - Yes
     - ``Secret`` name (same namespace) with the API token under key
       ``apitoken``.
   * - ``tlsCACertBundle``
     - No
     - ``ConfigMap`` name (same namespace) with a CA bundle, for
       self-signed endpoints.
   * - ``maxTokensForResponse``
     - No
     - Max response tokens. Minimum ``1``. Defaults to ``2048``.
   * - ``llmProjectID``
     - No
     - Required by some providers (e.g. WatsonX).
   * - ``llmDeploymentName``
     - No
     - Required by some providers (e.g. Azure OpenAI).
   * - ``llmAPIVersion``
     - No
     - Required by some providers (e.g. Azure OpenAI).
   * - ``feedbackEnabled``
     - No
     - User feedback collection. Defaults to ``true``.
   * - ``transcriptsEnabled``
     - No
     - Conversation transcript collection. Defaults to ``false``.

.. _supported-providers:

Supported LLM providers (``llmEndpointType``)
------------------------------------------------

* ``openai`` — OpenAI-compatible endpoints (Ollama, vLLM, etc.)
* ``azure_openai`` — Azure OpenAI (needs ``llmDeploymentName``, ``llmAPIVersion``)
* ``watsonx`` — IBM watsonx.ai (needs ``llmProjectID``)
* ``rhoai_vllm`` — vLLM via Red Hat OpenShift AI
* ``rhelai_vllm`` — vLLM via RHEL AI
* ``gemini`` — Google Gemini

.. tip::

   This list is enforced by the CRD schema and grows over time. Check
   ``oc explain openstacklightspeed.spec.llmEndpointType`` on your cluster
   for the current, authoritative list.

Logging (``logging``)
-----------------------

.. list-table::
   :header-rows: 1
   :widths: 25 15 60

   * - Field
     - Default
     - Description
   * - ``logging.ogxLogLevel``
     - ``all=info``
     - llama-stack/OGX container. Standard level, or
       ``component=level`` pairs (e.g. ``core=debug,providers=info``).
   * - ``logging.lightspeedStackLogLevel``
     - ``INFO``
     - lightspeed-service-api container. ``DEBUG``/``INFO``/``WARNING``/``ERROR``/``CRITICAL``.
   * - ``logging.dataverseExporterLogLevel``
     - ``INFO``
     - Feedback/transcript exporter sidecar. Same values as above.
   * - ``logging.postgresLogLevel``
     - ``INFO``
     - PostgreSQL container. ``DEBUG`` also logs every SQL statement.

Persistent storage (``database``)
------------------------------------

Omit for an ``emptyDir`` volume (data lost on pod reschedule), or set to
provision a PVC:

.. code-block:: yaml

   spec:
     database:
       size: "5Gi"                # default: 1Gi
       class: "my-storage-class"  # default: cluster's default StorageClass

Container resources (``resources``)
--------------------------------------

Every container has a default request/limit. Setting one replaces its
default entirely:

.. code-block:: yaml

   spec:
     resources:
       llamaStack:
         requests: {cpu: "500m", memory: "2Gi"}
         limits: {cpu: "2", memory: "8Gi"}
       lightspeedService:
         requests: {cpu: "250m", memory: "512Mi"}
         limits: {cpu: "1", memory: "2Gi"}
       postgres:
         requests: {cpu: "30m", memory: "300Mi"}
         limits: {cpu: "500m", memory: "2Gi"}
       okp:
         requests: {cpu: "500m", memory: "2Gi"}
         limits: {cpu: "2", memory: "4Gi"}
       consolePlugin:
         requests: {cpu: "50m", memory: "64Mi"}
         limits: {cpu: "200m", memory: "256Mi"}
       mcp:
         requests: {cpu: "50m", memory: "64Mi"}
         limits: {memory: "200Mi"}

.. _offline-knowledge-portal:

Offline Knowledge Portal (``okp``)
--------------------------------------

.. important::

   OKP is deployed on **every** install — ``spec.okp`` configures it, it
   doesn't gate whether it's deployed. Pulling its image needs the same
   free ``registry.redhat.io`` account as :ref:`redhat-registry-access`.

.. code-block:: yaml

   spec:
     okp: {}   # no access key: browsing works, search doesn't

.. code-block:: yaml

   spec:
     okp:
       accessKey: okp-access-key-secret   # Secret key: "access_key"

* **No ``accessKey``** (default) — browse docs and product lifecycle
  content; no search, Solutions, or Articles. What upstream users run on.
* **With ``accessKey``** — full search and the encrypted knowledgebase.
  Needs an active Red Hat Satellite subscription (`get one
  <https://access.redhat.com/offline/access>`_) — a bonus if you already
  have one, not something every user needs.

By default, **RAG grounding is OKP-only** — the bundled community
documentation is disabled unless you set ``dev.okpRagOnly: false`` (below).

Developer / experimental options (``dev``)
-----------------------------------------------

.. warning::

   Not part of the stable API — may change without notice.

.. code-block:: yaml

   spec:
     dev:
       featureFlags:
         - rhoso_mcps   # enables the read-only MCP introspection sidecar
       okpChunkFilterQuery: "product:(*openstack* OR *openshift*)"  # example override
       okpRagOnly: false  # include bundled community docs too, not just OKP
       rhosMCPConfig: |
         debug: true
         workers: 4

* ``okpChunkFilterQuery`` — if unset, auto-detects your OpenShift/RHOSO
  versions instead of using the literal example above.
* ``rhoso_mcps`` — deploys the MCP introspection sidecar (read-only). See
  :doc:`usage`.
