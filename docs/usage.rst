Usage
=====

Day-to-day use: asking questions, optional cluster introspection,
feedback/transcripts, and support.

Asking questions
--------------------

Open the Lightspeed widget (bottom-right corner of the OpenShift console)
once ``OpenStackLightspeed`` is ``Ready``:

* "How can I spin up a VM using the OpenStack CLI?"
* "Why would a Nova compute service show as down?"

Answers are grounded via RAG, with references you can verify. By default,
grounding comes from :ref:`offline-knowledge-portal` (always deployed, no
credentials needed to browse — see :doc:`configuration` for the free vs.
keyed tiers). The bundled community documentation is also available, but
only if you set ``dev.okpRagOnly: false``.

Cluster introspection (optional)
------------------------------------

Enabling the ``rhoso_mcps`` dev flag (:doc:`configuration`) gives the
assistant read-only tools to inspect your actual OpenStack/OpenShift
resources instead of relying on docs alone.

* **Strictly read-only** — can't modify your cluster or OpenStack deployment.
* Introspection stays local to your cluster; only the query and retrieved
  context go to your LLM provider.
* Credentials are automatic — the operator provisions a scoped Keystone
  Application Credential when an ``OpenStackControlPlane`` is detected.

Disabled by default; still evolving.

Feedback and transcripts
----------------------------

* ``feedbackEnabled`` (default ``true``) — thumbs-up/down on responses.
* ``transcriptsEnabled`` (default ``false``) — full conversation transcripts.

Both configured on the CR (:doc:`configuration`). Used to improve answer
quality — disable either if that doesn't fit your data policy.

.. _support:

Support
-----------

Community release — support is **upstream only**, via GitHub Issues:

* `lightspeed-operator issues <https://github.com/openstack-k8s-operators/lightspeed-operator/issues>`_
* `lightspeed-rag-content issues <https://github.com/openstack-k8s-operators/lightspeed-rag-content/issues>`_
* `lightspeed-mcps issues <https://github.com/openstack-k8s-operators/lightspeed-mcps/issues>`_

Include ``oc describe openstacklightspeed`` output, pod logs, and your
(redacted) CR spec — see :doc:`troubleshooting`.
