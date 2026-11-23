package secret

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/flyteorg/flyte/v2/flyteplugins/go/tasks/pluginmachinery/secret/config"
	"github.com/flyteorg/flyte/v2/gen/go/flyteidl2/core"
)

// Exercise the native Kubernetes store with a pod outside the store namespace,
// as produced by namespaceMapping. Env-var values remain explicitly opt-in.
func TestEmbeddedSecretsInMappedNamespace(t *testing.T) {
	for _, tc := range []struct {
		name       string
		namespace  string
		mount      core.Secret_MountType
		fromValues bool
		wantError  bool
	}{
		{"same namespace keeps secret ref", "flyte", core.Secret_ENV_VAR, false, false},
		{"mapped env var requires opt in", "project-development", core.Secret_ENV_VAR, false, true},
		{"mapped env var value", "project-development", core.Secret_ENV_VAR, true, false},
		{"mapped file", "project-development", core.Secret_FILE, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			secretID := EncodeSecretName("org", "development", "project", "token")
			stored := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: EncodeK8sSecretName(secretID), Namespace: "flyte"},
				Data:       map[string][]byte{secretID: []byte("test-token")},
			}
			scheme := runtime.NewScheme()
			require.NoError(t, corev1.AddToScheme(scheme))
			store := fake.NewClientBuilder().WithScheme(scheme).WithObjects(stored).Build()
			fetcher := NewK8sSecretFetcher(k8sfake.NewClientset(stored).CoreV1().Secrets("flyte"))
			injector := NewEmbeddedSecretManagerInjector(config.EmbeddedSecretManagerConfig{
				Type: config.EmbeddedSecretManagerTypeK8s, K8sConfig: config.K8sConfig{Namespace: "flyte"},
				EnvVarsFromValues:      tc.fromValues,
				FileMountInitContainer: config.FileMountInitContainerConfig{ContainerName: "secret-init"},
			}, []SecretFetcher{fetcher}, store, "flyte", newAlwaysMissCache(t),
				&config.Config{SecretEnvVarPrefix: config.DefaultSecretEnvVarPrefix})
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Namespace: tc.namespace, Labels: map[string]string{
					OrganizationLabel: "org", ProjectLabel: "project", DomainLabel: "development",
				}},
				Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "main"}}},
			}
			result, injected, err := injector.Inject(context.Background(), &core.Secret{Key: "token", MountRequirement: tc.mount}, pod)
			if tc.wantError {
				require.Error(t, err)
				assert.False(t, injected)
				assert.Empty(t, result.Spec.Containers[0].Env)
				return
			}
			require.NoError(t, err)
			require.True(t, injected)
			if tc.mount == core.Secret_FILE {
				require.Len(t, result.Spec.InitContainers, 1)
				assert.NotEmpty(t, result.Spec.Containers[0].VolumeMounts)
				assert.NotEmpty(t, result.Spec.InitContainers[0].Env)
				return
			}
			var env *corev1.EnvVar
			for i := range result.Spec.Containers[0].Env {
				candidate := &result.Spec.Containers[0].Env[i]
				if candidate.Name == config.DefaultSecretEnvVarPrefix+"TOKEN" {
					env = candidate
				}
			}
			require.NotNil(t, env)
			if tc.fromValues {
				assert.Equal(t, "test-token", env.Value)
				assert.Nil(t, env.ValueFrom)
			} else {
				assert.Empty(t, env.Value)
				require.NotNil(t, env.ValueFrom)
				assert.Equal(t, stored.Name, env.ValueFrom.SecretKeyRef.Name)
			}
		})
	}
}
