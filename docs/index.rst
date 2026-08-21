OpenStack Lightspeed Operator documentation
============================================

OpenStack Lightspeed is an AI-powered assistant, built for anyone running
`OpenStack <https://www.openstack.org/>`_ (an open-source cloud platform)
on OpenShift, that lives inside the OpenShift web console and answers
questions in plain English — grounded in real documentation, not guesses.

Ask it something like *"How do I create a VM using the OpenStack CLI?"* or
*"Why would a Nova compute service show as down?"* — see :doc:`usage` for
more.

You don't need an existing OpenStack deployment to try it — an OpenShift
cluster and an LLM you can point it at is enough (see :doc:`quickstart`).

.. important::

   This is a community release. Support is provided **upstream only**,
   through `GitHub Issues
   <https://github.com/openstack-k8s-operators/lightspeed-operator/issues>`_
   on this repository. There is no separate commercial support channel for
   this project.

.. toctree::
   :maxdepth: 2
   :caption: Contents:

   quickstart
   overview
   install_guide
   configuration
   troubleshooting
   usage
