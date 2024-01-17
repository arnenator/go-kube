package kubectl

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/Arneproductions/go-kube/internal/resources"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestApplyFunc(t *testing.T) {
	c := resources.NewEphemeralCluster()
	require.NoError(t, c.Start())
	defer c.Stop()

	t.Run("applyFunc_can_apply_create_namespace", func(t *testing.T) {
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

		err = applyFunc(ctx, c.KubeConfigFilePath(), &applyOptions{}, []string{tmpFile.Name()})
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
}
