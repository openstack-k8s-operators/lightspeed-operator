Quickstart
==========

Already have an OpenShift cluster (4.18+) and an LLM endpoint? Three
steps and you're running. No cluster yet? See
:ref:`dont-have-a-cluster-yet-crc`.

Install the operator
------------------------

**Operators → OperatorHub**, search **"OpenStack Lightspeed
(Community)"**, click **Install**. Full details (including what to do if
it's not visible yet): :doc:`install_guide`.

Create the secret and CR
------------------------------

Save as ``secret.yaml``, with your own LLM API key:

.. code-block:: yaml

   apiVersion: v1
   kind: Secret
   type: Opaque
   metadata:
     name: openstack-lightspeed-apitoken
     namespace: openstack-lightspeed
   stringData:
     apitoken: <your-llm-api-key>

Save as ``cr.yaml``, with your own endpoint and model:

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

Then apply both:

.. code-block:: bash

   oc apply -f secret.yaml
   oc apply -f cr.yaml

Self-hosted endpoint with a self-signed certificate, or a different
provider? See :doc:`install_guide` and :doc:`configuration` for the full
field reference.

Open the console
---------------------

.. code-block:: bash

   oc whoami --show-console

Open that URL and use the Lightspeed widget (lower-right corner).
