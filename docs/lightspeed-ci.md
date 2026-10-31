# OpenStack Lightspeed deployment and functional tests

The deployment job uses this repository's [installer](../ci/playbooks/install-openstack-lightspeed.yml)
to deploy Lightspeed and its mock provider, then runs the official application
tests in an AnsibleTest Pod through CI Framework's existing `test_operator` role.
The job and test settings are in [zuul.d/jobs.yaml](../zuul.d/jobs.yaml).

## Hook ownership

The job loads [ci/vars-lightspeed.yml](../ci/vars-lightspeed.yml) through
`cifmw_extras`. That file selects the installer in the operator's checked-out
repository using `ansible_user_dir` and Zuul's project `src_dir`. The path is
resolved on the CI controller, rather than using a laptop path or the framework's
relative hook lookup. The job retains its parent's multinode and Ceph-backend
scenario files before adding this local settings file.

CI Framework still provides deployment orchestration, hook execution and result
collection. No unmerged CI Framework change or tests fork is required for this
configuration. Installer and settings changes are eligible for the operator's
post-review deployment job; documentation changes use the documentation build.


The job uses the official tests repository and its default branch. Its runner
reads the client token from the standard ServiceAccount token path; the job
projects the token there explicitly. No tests fork is required for this route.

## Job configuration

The core settings are:

```yaml
cifmw_extras:
  - '@scenarios/centos-9/multinode-ci.yml'
  - '@scenarios/centos-9/hci_ceph_backends.yml'
  - "@{{ ansible_user_dir }}/{{ zuul.projects['github.com/openstack-k8s-operators/lightspeed-operator'].src_dir }}/ci/vars-lightspeed.yml"
cifmw_openstack_lightspeed_mock_enabled: true
cifmw_run_tests: true
cifmw_test_operator_fail_on_test_failure: true
cifmw_test_operator_stages:
  - name: lightspeed
    type: ansibletest
cifmw_test_operator_namespace: openstack
cifmw_test_operator_ansibletest_ansible_git_repo: https://github.com/openstack-k8s-operators/lightspeed-tests
cifmw_test_operator_ansibletest_ansible_git_branch: ""
cifmw_test_operator_ansibletest_ansible_playbook_path: playbooks/run_lightspeed_tests.yaml
cifmw_test_operator_ansibletest_ansible_inventory: |
  localhost ansible_connection=local ansible_python_interpreter=python3
cifmw_test_operator_ansibletest_ansible_var_files: |
  test_suite_path: /var/lib/ansible/ansible
  lightspeed_url: https://lightspeed-app-server.openstack-lightspeed.svc.cluster.local:8443
  lightspeed_model: "provider-default-model/default-model"
  pytest_args: "-v --tb=short --color=no -m 'not llm_dependent' tests/integration"
  junit_xml_path: /var/lib/AnsibleTests/external_files/baseline.xml
cifmw_test_operator_ansibletest_extra_mounts:
  - name: lightspeed
    extraVol:
      - propagation: [AnsibleTest]
        extraVolType: Undefined
        volumes:
          - name: lightspeed-client-token
            projected:
              sources:
                - serviceAccountToken:
                    path: token
                    expirationSeconds: 3600
        mounts:
          - name: lightspeed-client-token
            mountPath: /var/run/secrets/kubernetes.io/serviceaccount
            readOnly: true
```

Copy the image, resources, security and result-reader settings from the consuming
job as well. It uses `openstack-ansible-tests:current-podified`, global privilege,
SELinux and storage settings for its single Lightspeed stage, and the existing
`cifmw_test_operator_log_pod_definition` override for the reader. No shared-role
extensions or preparation hook are required. KUTTL runs separately.

The deployment hook installs the catalog, OLM subscription, credential Secret
and OpenStackLightspeed resource. The opt-in mock handles model discovery and
ordinary or streamed completions. Readiness remains bounded by 120 retries at
ten-second intervals, configurable with
`cifmw_openstack_lightspeed_ready_retries`. The operator selects its default
application images when no image override is supplied by the job.

## Source and authentication

The test image clones the configured Git repository into
`/var/lib/ansible/ansible` and executes the relative playbook path above. An empty
branch setting uses the cloned repository's default branch. A nonempty setting
selects a published remote branch; arbitrary commit IDs and PR references are
not supported by the inspected image startup script.

This clone does not use the prepared Zuul checkout. `Depends-On` or adding the
tests repository to `required-projects` does not make its unmerged changes
available to the Pod's independent clone. Check the selected source revision
inside the running Pod when verifying a run. There is no
source archive, ConfigMap transport, generated stage file or bootstrap.

The test-operator disables automatic token mounting and currently uses the
`default` ServiceAccount in the test namespace. The job explicitly projects
that account's token at
`/var/run/secrets/kubernetes.io/serviceaccount/token`; Kubernetes rotates it. The
operator creates the `lightspeed-access` `ClusterRole`, granting GET access to
`/ls-access`. The installer binds that role to the test ServiceAccount with a
namespace-specific `ClusterRoleBinding`. This also grants access to other Pods using the same
account in that namespace. Privileged execution does not supply a token.
The client token is separate from the mock provider credential. The runner
reads the mounted file and passes its contents to the Python client in the
test process environment. Do not put token contents in job variables.

Tests normally run in `openstack` and call the Lightspeed Service in
`openstack-lightspeed`. `cifmw_test_operator_namespace` selects the test namespace
and the ServiceAccount namespace used by the installer's access binding; it
defaults to `openstack` for standalone installation. Test access and dummy
prerequisites are grouped after the service reaches Ready.
`cifmw_openstack_lightspeed_namespace` selects the application namespace; update
the runner's `lightspeed_url` accordingly. The consuming job templates this URL.
The parent deployment supplies test-operator, its AnsibleTest API and the
existing OpenStack ConfigMap, configuration Secret and compute SSH Secret used
by the stage. Existing AnsibleTest parameters select those resource names.

## Provider credentials

The mock job sets `cifmw_openstack_lightspeed_api_token` to `dummy-token-for-ci`.
The installer creates the provider Secret with that dummy value. This is separate
from the Kubernetes ServiceAccount token used by the application test client.

For real-provider execution, preserve the credential handoff introduced by
[Jan's PR #87](https://github.com/openstack-k8s-operators/lightspeed-operator/pull/87):

1. Bind Zuul secret `lightspeed-openai-token` as `openai_credentials` in the
   real-provider job, running in the trusted post-review pipeline.
2. Use [create_lightspeed_secret.yml](../ci/playbooks/create_lightspeed_secret.yml)
   as its pre-run. It retains the automation SSH keys and creates the OpenShift
   Secret through stdin with `no_log: true`.
3. Leave `cifmw_openstack_lightspeed_api_token` unset. The installer checks the
   pre-created Secret and references it by name; it does not need the decrypted
   Zuul variable. The default Secret is `openstack-lightspeed-apitoken` in
   `openstack-lightspeed`, with key `apitoken`.
4. Keep [cleanup_lightspeed_secret.yml](../ci/playbooks/cleanup_lightspeed_secret.yml)
   as a post-run entry with `cleanup: true`.

Use the same namespace and Secret name for creation, installation and cleanup.
Do not pass a real token through `cifmw_extras`, ordinary job variables or
published parameter files. Merely moving the installer does not make a protected
Zuul variable available to the nested Ansible invocation.

The current mock job does not bind that real secret or enable its setup/cleanup
playbooks. Real-provider endpoint/model configuration and validation remain
follow-up work; the playbooks and encrypted secret definition are retained.

## Results and failures

The official runner installs dependencies and runs the selected integration
tests. The job excludes `llm_dependent` cases and keeps Goose tests selected.
It writes JUnit to `baseline.xml` on the results PVC; pytest output and setup
details are in the test Pod's console log. It does not create the fork runner's
separate `.log` or `.runner.log` files or its additional missing-authentication
test. The official runner fails when pytest returns a nonzero exit code.

On 31 October 2026, unmodified upstream revision `951f3f20` ran on the current
`current-podified` image in CRC with privilege disabled. All four non-Goose,
non-LLM-dependent cases passed, and JUnit was copied from the results PVC.
An explicit `main` branch setting failed during image startup; the empty
branch setting uses the repository default without that startup conflict.
This is manual runner compatibility evidence, not a full Zuul deployment.

Check that the published JUnit contains the expected executed cases and
matches the Pod result. The official runner does not itself reject every
missing, stale, malformed or entirely skipped report. The stronger report
guards proposed in the tests fork are separate improvements.

The existing role waits for Pods, collects results when its wait has not timed
out, and fails the job when a Pod failed or did not complete. Its usual timeout
is 3600 seconds; this is a controller wait, not a Pod termination deadline.
The reader uses user and group ID 227 and the test Pod's SELinux level to read
the PVC. Its job override retains the reader image, mounts and scheduling settings;
review it if the upstream reader template changes.

Find reports under `controller/ci-framework-data/tests/test_operator/`, checking
the actual nested paths. Read `baseline.xml` for executed cases and failures,
and the test Pod's console log for pytest output and setup failures.
Generated AnsibleTest and reader manifests are under
`artifacts/test-operator-crs`. For application traffic and Pod state, inspect
must-gather, then `namespaces/openstack-lightspeed/pods/` and each Pod's logs.
The direct route does not produce the former bootstrap's `source-revision.json`,
`runner-inputs.json` or outer `runner.log`.

## Validation

For an isolated run, reuse a Ready Lightspeed deployment and installed
test-operator. Ensure the installer has granted the test account access. Copy
the consuming job's test-operator settings into a local variables file, add the
cluster's kubeconfig and storage class, and run a local play containing
`roles: [test_operator]`. No preparation playbook is needed. This creates cluster
resources; preserve reports before removing a completed run's CR and owned
resources to repeat it. Reapplying an identical completed CR does not rerun it.

Local rendering, syntax and runner fixtures do not prove cluster execution.
After the coordinated changes are submitted, run combined Zuul validation.
Verify default images, actual source revision,
executed application cases and published artifacts, plus deliberate failure
propagation. Inspect the non-voting deployment job's own result: a green
buildset can conceal its failure. Mock results establish the tested integration
path; real-provider credentials and model quality require separate validation.
Real OpenAI access using Jan's credentials and a separate Lightspeed results
folder are follow-up work. The current test client disables TLS verification
in this disposable setup.
