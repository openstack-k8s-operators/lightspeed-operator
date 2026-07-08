/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	_ "embed"
	"time"
)

const (
	// Operator Settings
	ResourceCreationTimeout = 60 * time.Second

	// Application Server
	OpenStackLightspeedAppServerServiceAccountName = "lightspeed-app-server"
	OpenStackLightspeedAppServerSARRoleName        = OpenStackLightspeedAppServerServiceAccountName + "-sar-role"
	OpenStackLightspeedAppServerSARRoleBindingName = OpenStackLightspeedAppServerSARRoleName + "-binding"
	OpenStackLightspeedAppServerContainerPort      = 8443
	OpenStackLightspeedAppServerServicePort        = 8443
	OpenStackLightspeedAppServerServiceName        = "lightspeed-app-server"
	OpenStackLightspeedAppServerNetworkPolicyName  = "lightspeed-app-server"
	OpenStackLightspeedDefaultProvider             = "openstack-lightspeed-provider"
	OpenStackLightspeedVectorDBPath                = "/rag/vector_db/os_product_docs"

	ServingCertSecretAnnotationKey = "service.beta.openshift.io/serving-cert-secret-name" // #nosec G101 -- annotation key, not a credential

	// Monitoring
	MetricsReaderServiceAccountTokenSecretName = "metrics-reader-token" // #nosec G101 -- Kubernetes Secret resource name, not a credential
	MetricsReaderServiceAccountName            = "lightspeed-operator-metrics-reader"

	// Postgres
	PostgresDeploymentName                       = "lightspeed-postgres-server"
	PostgresServiceName                          = "lightspeed-postgres-server"
	PostgresSecretName                           = "lightspeed-postgres-secret"
	PostgresBootstrapSecretName                  = "lightspeed-postgres-bootstrap"
	PostgresConfigMapName                        = "lightspeed-postgres-conf"
	PostgresNetworkPolicyName                    = "lightspeed-postgres-server"
	PostgresServicePort                          = int32(5432)
	PostgresLightspeedStackDbName                = "lightspeed-stack"
	PostgresLlamaStackDbName                     = "llamastack"
	PostgresSharedBuffers                        = "256MB"
	PostgresMaxConnections                       = 100
	OpenStackLightspeedComponentPasswordFileName = "password"
	PostgresUsernameSecretKey                    = "username"
	PostgresBootstrapScript                      = "postgres_bootstrap.sh"
	PostgresBootstrapSQLScript                   = "postgres_bootstrap.sql"
	PostgresConfigKey                            = "postgresql.conf.sample"
	PostgresBootstrapVolumeMountPath             = "/usr/share/container-scripts/postgresql/start/postgres_bootstrap.sh"
	PostgresBootstrapSQLVolumeMountPath          = "/usr/share/container-scripts/postgresql/start/postgres_bootstrap.sql"
	PostgresConfigVolumeMountPath                = "/usr/share/pgsql/postgresql.conf.sample"
	PostgresDataVolume                           = "postgres-data"
	PostgresDataVolumeMountPath                  = "/var/lib/pgsql"
	PostgresDataPVCName                          = "openstack-lightspeed-database"
	PostgresDataPVCDefaultSize                   = "1Gi"
	PostgresVarRunVolumeName                     = "lightspeed-postgres-var-run"
	PostgresVarRunVolumeMountPath                = "/var/run/postgresql"

	// Non-admin user that should be used by lightspeed-stack and llama-stack (OGX) to access
	// the PostgreSQL database. This user gets created by the PostgreSQL container by setting
	// the POSTGRESQL_USER and POSTGRESQL_PASSWORD environment variable.
	PostgresSQLUsername = "lightspeed-app-user"

	TmpVolumeName      = "tmp-writable-volume"
	TmpVolumeMountPath = "/tmp"
	// Health probe settings for the PostgreSQL container.
	// Startup probe allows up to 300s for initialization (e.g. WAL recovery).
	// Liveness uses a longer period/timeout to avoid killing a busy-but-healthy instance.
	// Readiness uses a shorter period to quickly pull the pod from traffic when not accepting connections.
	PostgresStartupProbeInitialDelaySeconds = int32(5)
	PostgresStartupProbePeriodSeconds       = int32(10)
	PostgresStartupProbeTimeoutSeconds      = int32(5)
	PostgresStartupProbeFailureThreshold    = int32(30)
	PostgresLivenessProbePeriodSeconds      = int32(30)
	PostgresLivenessProbeTimeoutSeconds     = int32(10)
	PostgresLivenessProbeFailureThreshold   = int32(3)
	PostgresReadinessProbePeriodSeconds     = int32(10)
	PostgresReadinessProbeTimeoutSeconds    = int32(5)
	PostgresReadinessProbeFailureThreshold  = int32(3)

	// LCore specific
	LlamaStackContainerPort  = int32(8321)
	LlamaStackConfigCmName   = "llama-stack-config"
	LCoreConfigCmName        = "lightspeed-stack-config"
	LCoreDeploymentName      = "lightspeed-stack-deployment"
	LCoreConfigMountPath     = "/app-root/lightspeed-stack.yaml"
	LCoreUserDataMountPath   = "/tmp/data"
	ForceReloadAnnotationKey = "ols.openshift.io/force-reload"
	// Health probe settings for the llama-stack/OGX container.
	// The startup probe allows up to 30 failures (300s) for the slow initialization,
	// while liveness and readiness probes use a tighter threshold of 3 failures.
	LlamaStackHealthPath                   = "/v1/health"
	LlamaStackProbePeriodSeconds           = int32(10)
	LlamaStackProbeTimeoutSeconds          = int32(5)
	LlamaStackStartupProbeFailureThreshold = int32(30)
	LlamaStackProbeFailureThreshold        = int32(3)

	// Health probe settings for the lightspeed-stack container.
	// The startup probe allows up to 30 failures (300s) for initialization,
	// while liveness and readiness probes use a tighter threshold of 3 failures.
	LightspeedStackLivenessPath                 = "/liveness"
	LightspeedStackReadinessPath                = "/readiness"
	LightspeedStackProbePeriodSeconds           = int32(10)
	LightspeedStackProbeTimeoutSeconds          = int32(5)
	LightspeedStackStartupProbeFailureThreshold = int32(30)
	LightspeedStackProbeFailureThreshold        = int32(3)

	// Data Exporter
	ExporterConfigVolumeName       = "exporter-config"
	ExporterConfigMountPath        = "/etc/config"
	ExporterConfigFilename         = "config.yaml"
	ExporterConfigCmName           = "lightspeed-exporter-config"
	DataverseExporterContainerName = "lightspeed-to-dataverse-exporter"
	UserDataVolumeName             = "ols-user-data"
	RHOSOLightspeedOwnerIDLabel    = "openstack.org/lightspeed-owner-id"
	ServiceIDRHOSO                 = "rhos-lightspeed"

	// OKP (Offline Knowledge Portal)
	OKPContainerName       = "okp"
	OKPContainerPort       = int32(8080)
	OKPDeploymentName      = "lightspeed-okp-server"
	OKPServiceName         = "lightspeed-okp-server"
	OKPServicePort         = int32(8080)
	OKPAccessKeySecretKey  = "access_key"
	OKPDefaultOCPVersion   = "4.21"
	OKPDefaultRHOSOVersion = "18.0"
	OKPChunkFilterQueryFmt = "((product:*openstack* AND product_version:%s) OR (product:*openshift* AND product_version:%s))"
	ExternalProvidersDir   = "/app-root/providers.d"

	// Console Plugin
	ConsoleUIConfigMapName         = "lightspeed-console-plugin"
	ConsoleUIServiceCertSecretName = "lightspeed-console-plugin-cert"
	ConsoleUIServiceName           = "lightspeed-console-plugin"
	ConsoleUIDeploymentName        = "lightspeed-console-plugin"
	ConsoleUIHTTPSPort             = int32(9443)
	ConsoleUIPluginName            = "lightspeed-console-plugin"
	ConsoleUIServiceAccountName    = "lightspeed-console-plugin"
	ConsoleCRName                  = "cluster"
	ConsoleProxyAlias              = "ols"
	ConsoleUINetworkPolicyName     = "lightspeed-console-plugin"

	// Provider name constants representing valid values for
	// OpenStackLightpseed.Spec.LLMEndpointType (providers available to users)
	RHELAIVLLMProviderName  = "rhelai_vllm"
	RHOAIVLLMProviderName   = "rhoai_vllm"
	GeminiProviderName      = "gemini"
	AzureOpenAIProviderName = "azure_openai"
	OpenAIProviderName      = "openai"
	WatsonXProviderName     = "watsonx"

	// EnvVarSuffixAPIKey is the environment variable suffix for API key credentials
	EnvVarSuffixAPIKey = "_API_KEY"

	// VectorDBVolumeName is the name of the volume used by init containers to
	// store discovered values from vector DB images.
	VectorDBVolumeName = "vector-db-discovered-values"

	// VectorDBVolumeMountPath specifies the mount path for the volume that stores
	// discovered values from vector database images.
	VectorDBVolumeMountPath = "/vector-db-discovered-values"

	// VectorDBVolumeOGXConfigPath specifies the path within the `VectorDBVolumeName` volume
	// where the final OGX configuration file (ogx_config.yaml) is stored. This file is
	// generated by the init container responsible for assembling the final OGX config.
	VectorDBVolumeOGXConfigPath = VectorDBVolumeMountPath + "/ogx_config.yaml"

	// VectorDBVolumeLightspeedStackConfigPath specifies the path within the
	// `VectorDBVolumeName` volume where the final Lightspeed Stack configuration
	// file (lightspeed-stack.yaml) is stored. This file is generated by the
	// init container responsible for assembling the final Lightspeed Stack config.
	VectorDBVolumeLightspeedStackConfigPath = VectorDBVolumeMountPath + "/lightspeed-stack.yaml"

	// OKPEmbeddingModelMountPath specifies the path within the VectorDBVolumeName
	// volume where the OKP embedding model is stored after being extracted from the
	// rag-content image by the vector-database-collect init container.
	OKPEmbeddingModelMountPath = VectorDBVolumeMountPath + "/okp_embeddings_model"

	// OGXConfigInitContainerMountPath specifies the path where the operator-generated
	// OGX config file is mounted in the init container responsible for assembling
	// the final OGX configuration, which includes information about RAG.
	OGXConfigInitContainerMountPath = "/ogx_config.yaml"

	// LightspeedStackInitContainerMountPath specifies the path where the
	// operator-generated Lightspeed Stack config file is mounted in the init
	// container responsible for assembling the final Lightspeed Stack configuration,
	// which includes information about RAG.
	LightspeedStackInitContainerMountPath = "/lightspeed-stack.yaml"

	// OGXConfigVolumeName specifies the name of the volume holding config file for OGX
	// (generated by the operator and passed to init containers)
	OGXConfigVolumeName = "ogx-config"

	// LightspeedStackConfig specifies the name of the volume holding config file for
	// Lightspeed Stack (generated by the operator and passed to init containers)
	LightspeedStackConfig = "lightspeed-stack-config"

	// OGXConfigCMKey is the key in the ConfigMap under which the OGX configuration
	// is stored.
	OGXConfigCMKey = "ogx_config.yaml"

	// LightspeedStackConfigCMKey is the key in the ConfigMap under which the Lightspeed Stack
	// configuration is stored.
	LightspeedStackConfigCMKey = "lightspeed-stack.yaml"

	// VectorDBScriptsConfigMapName is the name of the ConfigMap that contains the
	// initialization scripts used by init containers to collect and build vector database data
	VectorDBScriptsConfigMapName = "vector-db-scripts"

	// VectorDBScriptsVolumeName is the name of the volume that mounts the ConfigMap containing
	// vector database initialization scripts for use by init containers
	VectorDBScriptsVolumeName = "vector-db-scripts"

	// VectorDBScriptsMountPath specifies the path where vector database init scripts
	// should be mounted within the init containers.
	VectorDBScriptsMountPath = "/scripts"

	// VectorDBCollectScriptKey is the ConfigMap key under which the vector_database_collect.sh
	// script is stored in the ConfigMap containing vector database init scripts.
	VectorDBCollectScriptKey = "vector_database_collect.sh"

	// VectorDBBuildScriptKey is the ConfigMap key under which the vector_database_build.py
	// script is stored in the ConfigMap containing vector database init scripts.
	VectorDBBuildScriptKey = "vector_database_build.py"

	// Resource Version Annotation
	// These constants define annotation keys used to track the resource versions of specific ConfigMaps.
	// By recording the resource version of a ConfigMap in a Deployment, StatefulSet, or similar resource,
	// changes to the referenced ConfigMaps can be detected and trigger rollouts or reconciliation in the operator.
	PostgresConfigMapResourceVersionAnnotation   = "ols.openshift.io/postgres-configmap-version"
	PostgresSecretResourceVersionAnnotation      = "ols.openshift.io/postgres-secret-version" // #nosec G101 -- annotation key, not a credential
	VectorDBScriptsConfigMapVersionAnnotation    = "ols.openshift.io/vector-db-scripts-configmap-version"
	LlamaStackConfigMapResourceVersionAnnotation = "ols.openshift.io/llamastack-configmap-version"
	LCoreConfigMapResourceVersionAnnotation      = "ols.openshift.io/lcore-configmap-version"
	CABundleConfigMapVersionAnnotation           = "ols.openshift.io/ca-bundle-configmap-version"

	// Volume Permissions
	// These constants define file permissions for volumes mounted in containers.
	VolumeDefaultMode    = int32(420)
	VolumeRestrictedMode = int32(0600)
	VolumeExecutableMode = int32(0755)

	// CABundleConfigMapName is the name of the ConfigMap that stores the
	// CA certificate bundle. It aggregates certificates from three sources —
	// operator system CAs, the OpenShift service serving CA (for in-cluster
	// service-to-service TLS), and the OpenShift API server CA — along with
	// any user-provided additional CAs.
	CABundleConfigMapName = "openstack-lightspeed-ca-bundle"

	// CABundleKey is the key within the CA bundle ConfigMap under which
	// the PEM-encoded certificate data is stored.
	CABundleKey = "tls-ca-bundle.pem"

	// CABundleVolumeName is the name of the volume used to mount the
	// CA bundle ConfigMap into containers.
	CABundleVolumeName = "ca-bundle"

	// CABundleMountPath is the filesystem path where the CA bundle is
	// mounted inside application containers.
	CABundleMountPath = "/etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem"

	// SystemTLSCABundlePath is the path to the system-wide CA certificate bundle
	// on the operator pod's filesystem. Used to read trusted root certificates
	// when building the CA bundle.
	SystemTLSCABundlePath = "/etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem"

	// KubeRootCAConfigMap is the name of the ConfigMap auto-created by
	// kube-controller-manager in every namespace, containing the CA certificate
	// that signs the API server's serving certificate. Read during CA
	// bundle reconciliation and merged into the bundle.
	KubeRootCAConfigMap = "kube-root-ca.crt"

	// OpenStackLightspeedTLSCertPath is the path to the TLS certificate file
	// inside the lightspeed-service-api container, used to serve HTTPS.
	OpenStackLightspeedTLSCertPath = OpenStackLightspeedAppCertsMountRoot + "/lightspeed-tls/tls.crt"

	// OpenStackLightspeedTLSKeyPath is the path to the TLS private key file
	// inside the lightspeed-service-api container, used to serve HTTPS.
	OpenStackLightspeedTLSKeyPath = OpenStackLightspeedAppCertsMountRoot + "/lightspeed-tls/tls.key"

	// OpenStackLightspeedCertsSecretName is the name of the Secret auto-provisioned
	// by the OpenShift service-ca operator when the lightspeed-app-server Service is
	// annotated with service.beta.openshift.io/serving-cert-secret-name. Contains
	// tls.crt and tls.key used by the lightspeed-service-api container to serve HTTPS.
	OpenStackLightspeedCertsSecretName = "lightspeed-tls" // #nosec G101 -- Kubernetes Secret resource name, not a credential

	// PostgresCertsSecretName is the name of the Secret auto-provisioned by the
	// OpenShift service-ca operator when the lightspeed-postgres-server Service is
	// annotated with service.beta.openshift.io/serving-cert-secret-name. Contains
	// tls.crt and tls.key used by the postgres container to serve TLS connections.
	PostgresCertsSecretName = "lightspeed-postgres-certs"

	// PostgresDefaultSSLMode is the sslmode used when connecting to PostgreSQL.
	// "verify-full" requires a valid server certificate and checks
	// that the server hostname matches the certificate CN/SAN, ensuring both
	// encryption and authentication of the database connection.
	PostgresDefaultSSLMode = "verify-full"

	// OpenStackLightspeedAppCertsMountRoot is the base directory under which
	// all application certificate volumes are mounted inside application containers.
	OpenStackLightspeedAppCertsMountRoot = "/etc/certs"

	// OpenShiftServiceCAConfigMap is the name of the ConfigMap containing the
	// OpenShift service serving CA certificate (public part only). This is the CA
	// that signs TLS certificates auto-provisioned for Services via the
	// service.beta.openshift.io/serving-cert-secret-name annotation.
	OpenShiftServiceCAConfigMap = "openshift-service-ca.crt"
)

// PostgreSQL Bootstrap Script - creates database, extensions, and schemas
//
//go:embed assets/postgres_bootstrap.sh
var PostgresBootStrapScriptContent string

// PostgreSQL Bootstrap SQL - creates database, extensions, and schemas
//
//go:embed assets/postgres_bootstrap.sql
var PostgresBootStrapSQLContent string

// PostgreSQL Configuration - SSL and TLS settings
//
//go:embed assets/postgres.conf
var PostgresConfigMapContent string

// vectorDatabaseCollectScript embeds the contents of the vector_database_collect.sh script
// found in the assets directory. This script is used during the initialization of the
// vector database, run as an init container in the deployment. Read
// assets/vector_database_collect.sh for more comprehensive explanation.
//
//go:embed assets/vector_database_collect.sh
var vectorDatabaseCollectScript string

// vectorDatabaseBuildScript embeds the contents of the vector_database_build.py script
// found in the assets directory. This script is responsible for building or processing
// the vector database and is used by an init container during deployment initialization.
// Read assets/vector_database_build.py for more comprehensive explanation.
//
//go:embed assets/vector_database_build.py
var vectorDatabaseBuildScript string

//go:embed assets/console_nginx.conf.tmpl
var consoleNginxConfigTemplate string

// consoleLocalesRewriteAwk is the awk script that performs case-preserving
// OpenShift -> OpenStack replacement only in JSON values (after the first `": `).
//
//go:embed assets/console_locales_rewrite.awk
var consoleLocalesRewriteAwk string
