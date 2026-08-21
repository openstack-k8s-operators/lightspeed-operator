Installation Guide
===================

This page covers prerequisites, installing the operator, setting up LLM
credentials, and deploying ``OpenStackLightspeed``. No cluster yet? See
:ref:`dont-have-a-cluster-yet-crc` at the end of this page.

Prerequisites
-------------

* An OpenShift cluster (4.18+).

  .. warning::

     Known issue: the console UI does not currently work on OpenShift 4.20
     or newer. Stick to 4.18/4.19 until this is resolved upstream.

* An LLM endpoint and API key — any provider from
  :ref:`supported-providers` works.
* A free Red Hat Developer account, to pull some images from
  ``registry.redhat.io`` — see :ref:`redhat-registry-access` below.
* Optional: an existing ``OpenStackControlPlane``, only needed for the
  experimental cluster-introspection feature (:doc:`usage`).

.. _redhat-registry-access:

Access to registry.redhat.io images
------------------------------------

The console plugin and OKP images (both always deployed) come from
``registry.redhat.io`` rather than ``quay.io``. This requires a **free**
account, not a paid subscription:

#. Create a free account at `developers.redhat.com
   <https://developers.redhat.com/>`_.
#. Download a pull secret from the `Hybrid Cloud Console
   <https://console.redhat.com/>`_.
#. Add it to your cluster:

   * **CRC**: pass it as ``PULL_SECRET`` when creating the cluster — see
     :ref:`dont-have-a-cluster-yet-crc`.
   * **Existing cluster**: merge it into the cluster-wide pull secret:

     .. code-block:: bash

        oc get secret/pull-secret -n openshift-config -o jsonpath='{.data.\.dockerconfigjson}' \
          | base64 -d > pull-secret.json
        # merge the downloaded auths into pull-secret.json, then:
        oc set data secret/pull-secret -n openshift-config \
          --from-file=.dockerconfigjson=pull-secret.json

#. Verify access:

   .. code-block:: console

      $ podman login registry.redhat.io
      Login Succeeded!

   ``ImagePullBackOff`` on the console plugin or OKP pod almost always
   means this step is missing — see :ref:`console-widget-not-appearing`.

.. tip::

   Once `PR #21 <https://github.com/openstack-k8s-operators/lightspeed-operator/pull/21>`_
   merges, image references become overridable on the CR, so this
   requirement becomes optional. Until then, plan on having registry
   access available.

.. _installing-the-operator:

Installing the operator
------------------------

#. **Operators → OperatorHub**, search for **"OpenStack Lightspeed
   (Community)"**.
#. Click **Install**, choosing the ``openstack-lightspeed`` namespace.
#. Track progress under **Operators → Installed Operators**, or:

   .. code-block:: console

      $ oc get -n openstack-lightspeed pods
      NAME                                                              READY   STATUS    RESTARTS   AGE
      openstack-lightspeed-operator-controller-manager-76df7fbfb5wggr   1/1     Running   0          72s

.. tip::

   Just-merged releases can take a little while to reach a cluster's
   catalog. If a version doesn't show up right away, give it time.

**Alternative — deploy from source** (for testing an unreleased build):

.. code-block:: bash

   git clone https://github.com/openstack-k8s-operators/lightspeed-operator.git
   cd lightspeed-operator
   make openstack-lightspeed-deploy

This sets up its own ``CatalogSource``, namespace, and ``Subscription`` —
bypassing OperatorHub entirely.

Setting up LLM credentials
----------------------------

You need an API key, endpoint URL, and model name.

Create the API key secret — the key **must** be named ``apitoken``:

.. code-block:: bash

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

Using a self-hosted endpoint with a self-signed certificate (e.g. vLLM,
Ollama)? Add its CA bundle too — any key name works, PEM data is all
that's parsed:

.. code-block:: bash

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

Public providers (Gemini, OpenAI, etc.) don't need this — skip straight to
the next step.

Deploying OpenStackLightspeed
--------------------------------

At minimum, set ``llmEndpoint``, ``llmEndpointType``, ``modelName``, and
``llmCredentials`` — see :doc:`configuration` for everything else:

.. code-block:: yaml

   apiVersion: lightspeed.openstack.org/v1beta1
   kind: OpenStackLightspeed
   metadata:
     name: openstack-lightspeed
     namespace: openstack-lightspeed
   spec:
     llmEndpoint: https://<llm-provider-host>:<port>/v1
     llmEndpointType: openai
     llmCredentials: openstack-lightspeed-apitoken
     modelName: <model-name>
     tlsCACertBundle: openstack-lightspeed-certs # optional

This deploys the full stack: the AI engine (lightspeed-stack and
llama-stack/OGX), PostgreSQL, OKP, and the console plugin.

Verifying the deployment
---------------------------

.. code-block:: bash

   oc describe -n openstack-lightspeed openstacklightspeed
   oc get -n openstack-lightspeed deployments,pods

Not reaching ``Ready``? See :doc:`troubleshooting`.

Accessing the assistant
---------------------------

.. code-block:: bash

   oc whoami --show-console

Open that URL and use the Lightspeed widget (lower-right corner). First
time activating the plugin, you may need to click **refresh** on the
console notification that appears.

.. _dont-have-a-cluster-yet-crc:

Don't have a cluster yet? (CRC)
-----------------------------------

For local development/testing only (not for trying the assistant for real
— CRC is resource-constrained). Deploy a CRC cluster before
:ref:`installing-the-operator`:

.. code-block:: bash

   git clone https://github.com/openstack-k8s-operators/install_yamls.git
   cd install_yamls/devsetup
   make download_tools

   CRC_VERSION=2.51.0 PULL_SECRET=~/work/pull-secret CRC_MONITORING_ENABLED=true CPUS=12 MEMORY=25600 DISK=100 make crc
   make crc_attach_default_interface
   eval $(crc oc-env)
   cd ../..

``PULL_SECRET`` is the same pull secret from :ref:`redhat-registry-access`.

CRC's console is always at a fixed address:
`console-openshift-console.apps-crc.testing
<https://console-openshift-console.apps-crc.testing>`_ — not something you
look up with ``oc whoami --show-console``.

Running CRC remotely? Reach that console with ``sshuttle``:

* Add to your local ``/etc/hosts`` (keep the IP as-is):
  ``192.168.130.11 api.crc.testing canary-openshift-ingress-canary.apps-crc.testing console-openshift-console.apps-crc.testing default-route-openshift-image-registry.apps-crc.testing downloads-openshift-console.apps-crc.testing oauth-openshift.apps-crc.testing``
* Run ``sshuttle -r $remote_username@$remote_server 192.168.130.0/24``.
