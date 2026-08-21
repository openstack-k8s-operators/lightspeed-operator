Troubleshooting
===============

Common failures, how to diagnose them, and where to get help if none of
this resolves it.

Start with the resource's conditions
----------------------------------------

.. code-block:: bash

   oc describe -n <namespace> openstacklightspeed

.. list-table::
   :header-rows: 1
   :widths: 30 70

   * - Condition
     - Meaning
   * - ``OpenStackLightspeedReady``
     - Overall readiness. ``False``/``Unknown``: engine, database, OKP, or
       console plugin hasn't converged yet.
   * - ``OpenStackLightspeedMCPServerReady``
     - Only relevant with ``rhoso_mcps`` enabled. Tracks the MCP sidecar.

Deployment-specific issues
------------------------------

lightspeed-stack (engine) pod not becoming healthy
+++++++++++++++++++++++++++++++++++++++++++++++++++

.. code-block:: bash

   oc logs -n <namespace> deploy/lightspeed-stack-deployment -c lightspeed-service-api
   oc logs -n <namespace> deploy/lightspeed-stack-deployment -c llama-stack

Usual causes: bad/unreachable ``llmEndpoint``, invalid ``apitoken``, or a
missing ``tlsCACertBundle`` for a self-signed endpoint. llama-stack logs
the actual auth/TLS error from the provider.

PostgreSQL pod not starting
++++++++++++++++++++++++++++

.. code-block:: bash

   oc logs -n <namespace> deploy/lightspeed-postgres-server

Shrinking ``spec.database.size`` below the existing PVC is rejected (not
supported in place). Revert the size, or delete/recreate the PVC to
actually shrink it (loses data).

.. _console-widget-not-appearing:

Console widget not appearing
+++++++++++++++++++++++++++++

* Confirm the ``ConsolePlugin`` (``lightspeed-console-plugin``) exists and
  is listed under ``spec.plugins`` on ``oc get
  console.operator.openshift.io cluster -o yaml``.
* Newly-activated plugins need a moment — click **refresh** on the
  console notification.
* Check the plugin's pod logs for TLS errors — its service-ca certificate
  can take a few seconds to appear after first deploy.

.. _imagepullbackoff:

ImagePullBackOff on any operator-managed pod
+++++++++++++++++++++++++++++++++++++++++++++

The console plugin and OKP pods come from ``registry.redhat.io``, not
``quay.io``:

.. code-block:: console

   Failed to pull image "registry.redhat.io/...": unauthorized: Please login to the Red Hat Registry using your Customer Portal credentials.

Means the pull secret is missing ``registry.redhat.io`` credentials — see
:ref:`redhat-registry-access` to fix and verify with Podman.

CA bundle errors
++++++++++++++++++

If ``tlsCACertBundle`` causes a CA parsing error, check every key in the
ConfigMap's ``data`` for valid PEM data and no stray whitespace (all keys
are parsed, not just one named ``cert``).

Getting operator logs
-------------------------

.. code-block:: bash

   oc logs -n <operator-namespace> deploy/openstack-lightspeed-operator-controller-manager

Still stuck? See :doc:`usage` for support.
