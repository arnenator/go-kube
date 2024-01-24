package kubectl

import (
	"context"
	"os"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/Arneproductions/go-kube/internal/resources"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

func TestDeleteFunc(t *testing.T) {

	c := resources.NewEphemeralCluster()
	err := c.Start()
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, c.Stop())
	})

	t.Run("deleteFunc_should_error_when_no_manifests_are_given", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()

		err := deleteFunc(ctx, c.KubeConfigFilePath(), &deleteOptions{})
		assert.Error(t, err)
	})

	t.Run("deleteFunc_should_error_when_no_kubeconfig_is_given", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()

		err := deleteFunc(ctx, "", &deleteOptions{}, "some-file.yaml")
		assert.Error(t, err)
	})

	t.Run("deleteFunc_should_delete_single_manifest_cluster_wide_resource", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.TODO(), time.Minute)
		defer cancel()

		dpl, ns, err := genNamespaceManifest()
		require.NoError(t, err)

		t.Cleanup(func() {
			assert.NoError(t, os.Remove(dpl))
		})

		err = applyFunc(ctx, c.KubeConfigFilePath(), &applyOptions{}, dpl)
		require.NoError(t, err)

		err = deleteFunc(ctx, c.KubeConfigFilePath(), &deleteOptions{}, dpl)
		assert.NoError(t, err)

		// Assert that the namespace does not exist anymore
		_, err = c.Client().CoreV1().Namespaces().Get(ctx, ns, metav1.GetOptions{})
		assert.Error(t, err)
	})

	t.Run("deleteFunc_should_delete_multiple_manifest_cluster_wide_resource", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()

		manifestCount := 5

		dpls := make([]string, manifestCount)
		nss := make([]string, manifestCount)
		errs := make([]error, manifestCount)

		for i := 0; i < manifestCount; i++ {
			dpl, ns, err := genNamespaceManifest()
			require.NoError(t, err)

			t.Cleanup(func() {
				assert.NoError(t, os.Remove(dpl))
			})

			dpls[i] = dpl
			nss[i] = ns
			errs[i] = err
		}

		for i := 0; i < manifestCount; i++ {
			err := applyFunc(ctx, c.KubeConfigFilePath(), &applyOptions{}, dpls[i])
			require.NoError(t, err)
		}

		err := deleteFunc(ctx, c.KubeConfigFilePath(), &deleteOptions{}, dpls...)
		assert.NoError(t, err)

		for i := 0; i < manifestCount; i++ {
			// Assert that the namespace does not exist anymore
			_, err = c.Client().CoreV1().Namespaces().Get(ctx, nss[i], metav1.GetOptions{})
			assert.Error(t, err)
		}
	})

	t.Run("deleteFunc_should_delete_manifest_namespace_scoped_resource", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()

		dpl, dplName, err := genDeploymentManifest()
		require.NoError(t, err)

		t.Cleanup(func() {
			require.NoError(t, os.Remove(dpl))
		})

		err = applyFunc(ctx, c.KubeConfigFilePath(), &applyOptions{}, dpl)
		require.NoError(t, err)

		err = wait.PollUntilContextCancel(ctx, time.Second, true, func(ctx context.Context) (bool, error) {
			_, err := c.Client().AppsV1().Deployments("default").Get(ctx, dplName, metav1.GetOptions{})
			if err != nil {
				return false, nil
			}

			return true, nil
		})
		require.NoError(t, err)

		err = deleteFunc(ctx, c.KubeConfigFilePath(), &deleteOptions{}, dpl)
		assert.NoError(t, err)

		// Assert that the namespace does not exist anymore
		_, err = c.Client().AppsV1().Deployments("default").Get(ctx, dplName, metav1.GetOptions{})
		assert.Error(t, err)
	})

	t.Run("deleteFunc_should_delete_kustomization", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()

		kustomizationPath, configMapName, err := genKustomizationManifest()
		require.NoError(t, err)

		t.Cleanup(func() {
			//require.NoError(t, os.Remove(kustomizationPath))
		})

		err = applyFunc(ctx, c.KubeConfigFilePath(), &applyOptions{IsKustomization: true}, kustomizationPath)
		require.NoError(t, err)

		err = wait.PollUntilContextCancel(ctx, time.Second, true, func(ctx context.Context) (bool, error) {
			confMapList, err := c.Client().CoreV1().ConfigMaps("default").List(ctx, metav1.ListOptions{})
			if err != nil {
				return false, nil
			}

			for _, cm := range confMapList.Items {
				if strings.HasPrefix(cm.Name, configMapName) {
					return true, nil
				}
			}

			return false, nil
		})
		require.NoError(t, err, "waiting for generated configmap to exist failed")

		err = deleteFunc(ctx, c.KubeConfigFilePath(), &deleteOptions{IsKustomization: true}, kustomizationPath)
		assert.NoError(t, err)

		configMaps, err := c.Client().CoreV1().ConfigMaps("default").List(ctx, metav1.ListOptions{})
		require.NoError(t, err)

		for _, cm := range configMaps.Items {
			if strings.HasPrefix(cm.Name, configMapName) {
				assert.Fail(t, "configMap still exists")
			}
		}
	})
}

func genKustomizationManifest() (string, string, error) {
	kustomizationName := uuid.New().String()

	manifest := strings.ReplaceAll(`
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
configMapGenerator:
- name: %s
  literals:
  - foo=bar
`, "%s", kustomizationName)

	tmpDir, err := os.MkdirTemp("", "create-kustomization-*")
	if err != nil {
		return "", "", err
	}

	tmpFile, err := os.Create(path.Join(tmpDir, "kustomization.yaml"))
	if err != nil {
		return "", "", err
	}

	_, err = tmpFile.WriteString(manifest)
	if err != nil {
		return "", "", err
	}

	return tmpDir, kustomizationName, nil
}

func genNamespaceManifest() (string, string, error) {
	namespaceName := uuid.New().String()

	manifest := strings.ReplaceAll(`
apiVersion: v1
kind: Namespace
metadata:
  name: %s
`, "%s", namespaceName)

	tmpFile, err := os.CreateTemp("", "create-ns-*.yaml")
	if err != nil {
		return "", "", err
	}

	_, err = tmpFile.WriteString(manifest)
	if err != nil {
		return "", "", err
	}

	return tmpFile.Name(), namespaceName, nil
}

func genDeploymentManifest() (string, string, error) {
	deplName := uuid.New().String()

	manifest := strings.ReplaceAll(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: %s
  labels:
    app: %s
spec:
  replicas: 1
  selector:
    matchLabels:
      app: %s
  template:
    metadata:
      labels:
        app: %s
    spec:
      containers:
      - name: nginx
        image: nginx:1.14.2
        ports:
        - containerPort: 80
`, "%s", deplName)

	tmpFile, err := os.CreateTemp("", "create-depl-*.yaml")
	if err != nil {
		return "", "", err
	}

	_, err = tmpFile.WriteString(manifest)
	if err != nil {
		return "", "", err
	}

	return tmpFile.Name(), deplName, nil
}
