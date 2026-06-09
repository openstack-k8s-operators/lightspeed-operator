#!/bin/bash
export RELATED_IMAGE_LCORE_IMAGE_URL_DEFAULT="quay.io/lightspeed-core/lightspeed-stack:dev-latest"
export RELATED_IMAGE_EXPORTER_IMAGE_URL_DEFAULT="quay.io/lightspeed-core/lightspeed-to-dataverse-exporter:latest"
export RELATED_IMAGE_POSTGRES_IMAGE_URL_DEFAULT="registry.redhat.io/rhel9/postgresql-16:latest"
# TODO(lpiwowar): Replace this with a stable (non-alpha) image version once
# the automated pipeline for building OGX-compatible vector database images
# is ready.
export RELATED_IMAGE_OPENSTACK_LIGHTSPEED_IMAGE_URL_DEFAULT="quay.io/openstack-lightspeed/rag-content:os-docs-2026.1-ogx"
export RELATED_IMAGE_OKP_IMAGE_URL_DEFAULT="registry.redhat.io/offline-knowledge-portal/rhokp-rhel9:latest"
export RELATED_IMAGE_CONSOLE_IMAGE_URL_DEFAULT="registry.redhat.io/openshift-lightspeed/lightspeed-console-plugin-rhel9:1.0.12"
export RELATED_IMAGE_CONSOLE_PF5_IMAGE_URL_DEFAULT="registry.redhat.io/openshift-lightspeed/lightspeed-console-plugin-pf5-rhel9:1.0.12"
export RELATED_IMAGE_MCP_SERVER_IMAGE_URL_DEFAULT="quay.io/openstack-lightspeed/rhos-mcps:latest"
export WATCH_NAMESPACE="openstack-lightspeed"
