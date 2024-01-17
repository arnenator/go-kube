package kubectl

import (
	"context"
	"fmt"
	"os"
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

func TestApplyFunc(t *testing.T) {
	c := resources.NewEphemeralCluster()
	require.NoError(t, c.Start())

	t.Cleanup(func() {
		err := c.Stop()
		require.NoError(t, err)
	})

	t.Run("applyFunc_can_apply_create_namespace", func(t *testing.T) {
		t.Parallel()

		randomNS := fmt.Sprintf("test-ns-%s", uuid.New().String())

		manifest := fmt.Sprintf(`
apiVersion: v1
kind: Namespace
metadata:
  name: %s
`, randomNS)

		tmpFile, err := os.CreateTemp("", "test-apply-*.yaml")
		require.NoError(t, err)
		t.Cleanup(func() {
			os.Remove(tmpFile.Name())
		})

		_, err = tmpFile.WriteString(manifest)
		require.NoError(t, err)

		err = tmpFile.Close()
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()

		err = applyFunc(ctx, c.KubeConfigFilePath(), &applyOptions{}, tmpFile.Name())
		require.NoError(t, err)

		ns, err := c.Client().CoreV1().Namespaces().Get(ctx, randomNS, metav1.GetOptions{})
		require.NoError(t, err)
		require.Equal(t, randomNS, ns.Name)

		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()

			err = c.Client().CoreV1().Namespaces().Delete(ctx, randomNS, metav1.DeleteOptions{})
			require.NoError(t, err)
		})
	})

	t.Run("applyFunc_can_apply_kustomization", func(t *testing.T) {
		t.Parallel()

		configMapName := fmt.Sprintf("test-cm-%s", uuid.New().String())

		// The configMapGenerator will create a config map where only our `configMapName`
		// is a prefix of the name. We need to be careful later...
		kustomization := fmt.Sprintf(`
configMapGenerator:
- name: %s
  literals:
  - foo=bar`, configMapName)

		tmpDir, err := os.MkdirTemp("", "test-apply-*")
		require.NoError(t, err)

		t.Cleanup(func() {
			os.RemoveAll(tmpDir)
		})

		kustomizationFile := fmt.Sprintf("%s/kustomization.yaml", tmpDir)

		err = os.WriteFile(kustomizationFile, []byte(kustomization), 0644)
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		err = applyFunc(ctx, c.KubeConfigFilePath(), &applyOptions{IsKustomization: true}, tmpDir)
		require.NoError(t, err)

		err = wait.PollUntilContextCancel(ctx, 2*time.Second, false, func(ctx context.Context) (done bool, err error) {
			cmList, err := c.Client().CoreV1().ConfigMaps("default").List(ctx, metav1.ListOptions{})
			if err != nil {
				return false, nil
			}

			for _, cm := range cmList.Items {
				if strings.HasPrefix(cm.Name, configMapName) {
					return true, nil
				}
			}

			return false, nil
		})
		assert.NoError(t, err)

		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()

			cmList, err := c.Client().CoreV1().ConfigMaps("default").List(ctx, metav1.ListOptions{})
			require.NoError(t, err)

			for _, cm := range cmList.Items {
				if strings.HasPrefix(cm.Name, configMapName) {
					err = c.Client().CoreV1().ConfigMaps("default").Delete(ctx, cm.Name, metav1.DeleteOptions{})
					require.NoError(t, err)
				}
			}
		})
	})
}
