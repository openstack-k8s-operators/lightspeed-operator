#!/bin/bash
export RELATED_IMAGE_LCORE_IMAGE_URL_DEFAULT="quay.io/lightspeed-core/lightspeed-stack:0.7.2"
export RELATED_IMAGE_OGX_IMAGE_URL_DEFAULT="quay.io/lightspeed-core/lightspeed-stack:0.7.2"
export RELATED_IMAGE_EXPORTER_IMAGE_URL_DEFAULT="quay.io/lightspeed-core/lightspeed-to-dataverse-exporter:latest"
export RELATED_IMAGE_POSTGRES_IMAGE_URL_DEFAULT="quay.io/sclorg/postgresql-16-c10s:latest"
# TODO(lpiwowar): Replace this with a stable (non-alpha) image version once
# the automated pipeline for building OGX-compatible vector database images
# is ready.
export RELATED_IMAGE_OPENSTACK_LIGHTSPEED_IMAGE_URL_DEFAULT="quay.io/openstack-lightspeed/rag-content:os-docs-2026.1-ogx"
export RELATED_IMAGE_OKP_IMAGE_URL_DEFAULT="registry.redhat.io/offline-knowledge-portal/rhokp-rhel9:latest"
export WATCH_NAMESPACE="openstack-lightspeed"
