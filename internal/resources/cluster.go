package resources

import (
	"fmt"
	"os"
	"time"

	"github.com/pkg/errors"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/kind/pkg/apis/config/v1alpha4"
	"sigs.k8s.io/kind/pkg/cluster"
	"sigs.k8s.io/kind/pkg/log"
)

// EphemeralCluster is a wrapper around a Kind kubernetes cluster
type EphemeralCluster struct {
	nodeImage   string `default:"kindest/node"`
	nodeVersion string `default:"v1.26.2"`

	clusterName string

	clientset          *kubernetes.Clientset
	kubeConfigFilePath string
	provider           *cluster.Provider
	restConfig         *rest.Config
	dynamicClient      *dynamic.DynamicClient
}

// GenericCluster is a wrapper around a kubernetes clientset and a kubeconfig file.
type GenericCluster struct {
	clientset          *kubernetes.Clientset
	kubeConfigFilePath string
	provider           *cluster.Provider
	restConfig         *rest.Config
	dynamicClient      *dynamic.DynamicClient
}

/*
NewExistingCluster creates a new GenericCluster from an existing kubeconfig file.

Example:

	c, err := resources.NewExistingCluster("/Users/billy.bob/path/to/kubeconfig")
	require.NoError(t, err)
*/
func NewExistingCluster(kubeconfig string) (*GenericCluster, error) {
	gc := &GenericCluster{}

	restConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, err
	}

	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, errors.Wrapf(
			err,
			"could not create clientset for generic cluster",
		)
	}

	dynamicClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, errors.Wrapf(
			err,
			"could not create dynamic client for generic cluster",
		)
	}

	gc.restConfig = restConfig
	gc.clientset = clientset
	gc.dynamicClient = dynamicClient
	gc.kubeConfigFilePath = kubeconfig

	return gc, nil
}

func NewEphemeralCluster() *EphemeralCluster {
	return &EphemeralCluster{
		nodeImage:   "kindest/node",
		nodeVersion: "v1.26.2",
	}
}

func (gc *GenericCluster) Client() *kubernetes.Clientset {
	return gc.clientset
}

func (gc *GenericCluster) KubeConfigFilePath() string {
	return gc.kubeConfigFilePath
}

func (ec *EphemeralCluster) image() string {
	return fmt.Sprintf("%s:%s", ec.nodeImage, ec.nodeVersion)
}

func (ec *EphemeralCluster) Start() error {
	provider := cluster.NewProvider(
		cluster.ProviderWithLogger(log.NoopLogger{}),
	)

	clusterName := randomName(24, []string{"ephemeral", "cluster"})

	tmpFile, err := os.CreateTemp("", fmt.Sprintf("%s-*.kubeconfig", clusterName))
	if err != nil {
		return errors.Wrapf(
			err,
			"could not create temporary file for kubeconf for cluster %s",
			clusterName,
		)
	}

	err = provider.Create(clusterName,
		cluster.CreateWithKubeconfigPath(tmpFile.Name()),
		cluster.CreateWithWaitForReady(5*time.Minute),
		cluster.CreateWithV1Alpha4Config(&v1alpha4.Cluster{
			Name: clusterName,
			Nodes: []v1alpha4.Node{
				{
					Role:  v1alpha4.ControlPlaneRole,
					Image: ec.image(),
				},
			},
		}),
	)
	if err != nil {
		return errors.Wrapf(
			err,
			"could not create ephemeral cluster %s",
			clusterName,
		)
	}

	restConfig, err := clientcmd.BuildConfigFromFlags("", tmpFile.Name())
	if err != nil {
		return err
	}

	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return errors.Wrapf(
			err,
			"could not create clientset for ephemeral cluster %s",
			clusterName,
		)
	}

	dynamicClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return errors.Wrapf(
			err,
			"could not create dynamic client for ephemeral cluster %s",
			clusterName,
		)
	}

	ec.restConfig = restConfig
	ec.clientset = clientset
	ec.dynamicClient = dynamicClient
	ec.clusterName = clusterName
	ec.kubeConfigFilePath = tmpFile.Name()
	ec.provider = provider

	return nil
}

func (ec *EphemeralCluster) Stop() error {
	if ec.provider == nil {
		return nil
	}

	err := ec.provider.Delete(ec.clusterName, ec.kubeConfigFilePath)
	if err != nil {
		return errors.Wrapf(
			err,
			"could not delete ephemeral cluster %s",
			ec.clusterName,
		)
	}

	err = os.Remove(ec.kubeConfigFilePath)
	if err != nil {
		return errors.Wrapf(
			err,
			"could not delete kubeconfig file %s",
			ec.kubeConfigFilePath,
		)
	}

	return nil
}

func (ec *EphemeralCluster) KubeConfigFilePath() string {
	return ec.kubeConfigFilePath
}

func (ec *EphemeralCluster) Client() *kubernetes.Clientset {
	return ec.clientset
}
