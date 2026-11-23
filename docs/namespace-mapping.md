# Run namespaces

`namespaceMapping` places a run's TaskActions and task pods in a namespace
rendered from its organization, project and domain. It is disabled by default.
It does not select a namespace from the submitting user's identity or accept a
namespace argument from the SDK.

For example, these Flyte binary Helm values place the `training` project in the
`development` domain into the existing `training-development` namespace:

```yaml
configuration:
  namespaceMapping:
    template: "{{ project }}-{{ domain }}"
    createNamespaces: false
```

The same `configuration.namespaceMapping` values apply to the Flyte core chart.
Use `{{ org }}`, `{{ project }}` and `{{ domain }}` in the template. The result
must be a valid Kubernetes namespace name. An empty template uses the configured
default namespace.

With `createNamespaces: false`, provision namespaces, resource quotas, network
policies, service accounts and workload credentials before submitting a run.
With `createNamespaces: true`, Flyte creates missing namespaces; it does not
create those policies or credentials. The Actions service needs permission to
get namespaces, and to create them when creation is enabled. Its TaskAction
permissions and the executor's watch and workload permissions must cover mapped
namespaces. Avoid templates that collapse distinct projects or domains into the
same namespace if their run names can collide.

Children follow their parent's namespace. Changing or removing the template
only changes placement of new runs: existing runs remain in their original
namespace. Lookups first check the currently configured and default namespaces,
then use the persisted project, domain and run labels across namespaces. This
also works after an Actions service restart. Keep cluster-wide TaskAction list
and watch permissions when changing the template. Do not manually duplicate a
run's TaskActions across namespaces.

If a cached namespace has been deleted, automatic creation retries once for a
new root action. A missing pre-provisioned namespace or a terminating namespace
fails the run with `FailedPrecondition`. Flyte does not recreate a namespace for
an orphaned child action.

## Embedded secrets

The Kubernetes secret store remains in its configured namespace. Kubernetes
`SecretKeyRef` cannot reference that store from a pod in another namespace.
Before enabling namespace mapping for tasks that use embedded secrets, choose a
supported injection mode:

- A `FILE` mount works across namespaces using the embedded secret manager's
  init container. The task must read the secret as a file.
- `ANY` and `ENV_VAR` mounts require an explicit opt-in when the pod is outside
  the store namespace. In the executor's secret manager configuration, set
  `webhook.embeddedSecretManagerConfig.envVarsFromValues: true`. This injects the
  secret value into the Pod specification. Users who can read Pods can read that
  value. The `FILE` injection path also carries encoded secret data in the init
  container's environment; it does not hide the value from Pod readers.
- Without that opt-in, cross-namespace environment-variable injection fails.
  Same-namespace Kubernetes secrets continue using `SecretKeyRef`.

Namespace mapping does not enable `envVarsFromValues` automatically. It does not
mirror user secrets into mapped namespaces. Review the chosen injection mode
and namespace permissions before enabling it for a deployment.

## Local verification

The Actions unit tests cover template changes and removal, restart discovery,
parent/child placement, ownership checks, namespace deletion and termination.
`TestEmbeddedSecretsInMappedNamespace` covers the native Kubernetes secret store
and its supported injection modes. The executor envtest integration suite
creates real TaskAction and Pod objects through the Kubernetes API; it does not
start containers or validate cloud workload credentials.

```sh
go test -race ./actions/... ./flytestdlib/namespaceutils/... ./flytestdlib/k8s/...
go test -race ./flyteplugins/go/tasks/pluginmachinery/secret/...
# Set KUBEBUILDER_ASSETS using setup-envtest as in .github/workflows/go-tests.yml.
go test -race ./executor/test/integration/...
```
