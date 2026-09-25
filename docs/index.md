# OpenStack Lightspeed Operator documentation

OpenStack Lightspeed is an AI-powered assistant, tailored for Red Hat
OpenStack Services on OpenShift (RHOSO), that lives inside the OpenShift
web console and answers questions in plain English — grounded in real
documentation, not guesses.

Ask it something like *"How do I create a VM using the OpenStack CLI?"* or
*"Why would a Nova compute service show as down?"* — see [Usage](usage.md) for
more.

You don't need an existing RHOSO deployment to try it — an OpenShift
cluster and an LLM you can point it at is enough (see [Quickstart](quickstart.md)).

> [!IMPORTANT]
> This is a community release. Support is provided **upstream only**,
> via GitHub Issues — there is no separate commercial support channel
> for this project:
>
> - [lightspeed-operator issues](https://github.com/openstack-k8s-operators/lightspeed-operator/issues)
> - [lightspeed-rag-content issues](https://github.com/openstack-k8s-operators/lightspeed-rag-content/issues)
> - [lightspeed-mcps issues](https://github.com/openstack-k8s-operators/lightspeed-mcps/issues)
>
> See [Troubleshooting](troubleshooting.md) for what to check, and what to include,
> before filing an issue.

:::{toctree}
:maxdepth: 2
:caption: Contents:

quickstart
install_guide
configuration
development
troubleshooting
usage
:::
