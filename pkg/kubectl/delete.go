package kubectl

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/kubectl/pkg/cmd/create"
	"k8s.io/kubectl/pkg/cmd/delete"
	"k8s.io/kubectl/pkg/cmd/util"
)

var (
	deleteLock = &sync.Mutex{}
)

type deleteOptions struct {
	IsKustomization bool `default:"false"`
}

/*
DeleteManifests deletes the resource created by the given manifest files from the cluster that the kubeconfigPath points to.

Example:

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := DeleteManifests(
		ctx,
		"/path/to/kubeconfig",
		[]string{"path/to/file1", "path/to/file2"}...
	)

	if err != nil {
		// Handle error
	}
*/
func DeleteManifests(ctx context.Context, kubeconfigPath string, filePaths ...string) error {

	opts := &deleteOptions{}

	return deleteFunc(ctx, kubeconfigPath, opts, filePaths...)
}

/*
DeleteKustomization deletes the resources created by the given kustomization files from the cluster that the kubeconfigPath points to.

Example:

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := DeleteKustomization(
		ctx,
		"/path/to/kubeconfig",
		[]string{"path/to/kustomization1", "path/to/kustomization2"}...
	)

	if err != nil {
		// Handle error
	}
*/
func DeleteKustomization(ctx context.Context, kubeconfigPath string, filePaths ...string) error {

	opts := &deleteOptions{}
	opts.IsKustomization = true // Force IsKustomization to true

	return deleteFunc(ctx, kubeconfigPath, opts, filePaths...)
}

func deleteFunc(ctx context.Context, kubeconfigPath string, opts *deleteOptions, filePaths ...string) error {
	deleteLock.Lock()
	defer deleteLock.Unlock()

	if kubeconfigPath == "" {
		return fmt.Errorf("kubeconfig path cannot be empty")
	}

	if opts == nil {
		return fmt.Errorf("options cannot be nil")
	}

	if len(filePaths) == 0 {
		return fmt.Errorf("no files to delete")
	}

	ioStreams, streamOut, _, streamErr := genericiooptions.NewTestIOStreams()

	config := genericclioptions.
		NewConfigFlags(true).
		WithDeprecatedPasswordFlag().
		WithDiscoveryBurst(300).
		WithDiscoveryQPS(50.0)

	config.KubeConfig = &kubeconfigPath
	f := util.NewFactory(config)

	errChan := make(chan error)

	// We find out if the context have a deadline, from there we derive amount of time left
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(15 * time.Second) // This deadline is arbitary
	}
	timeLeft := deadline.Sub(time.Now())

	// Send a timeout error to the error channel when the deadline is reached
	time.AfterFunc(timeLeft, func() {
		errChan <- context.DeadlineExceeded
	})

	util.BehaviorOnFatal(func(msg string, errCode int) {
		err := fmt.Errorf(
			"fatal error: %s\nerror code: %d\nout stream: %s\nerror stream: %s\n",
			msg,
			errCode,
			streamOut.String(),
			streamErr.String(),
		)
		errChan <- err
	})

	// We restore the default behavior for fatal errors when we are done
	defer util.DefaultBehaviorOnFatal()

	createCmd := create.NewCmdCreate(f, ioStreams)
	deleteCmd := delete.NewCmdDelete(f, ioStreams)

	if opts.IsKustomization {
		deleteCmd.Flags().Set("kustomize", strings.Join(filePaths, ","))
	} else {
		deleteCmd.Flags().Set("filename", strings.Join(filePaths, ","))
	}

	go func() {
		// deleteCmd is blocking. Should it fail it should have called the fatal error handler which
		// we override earlier to send an error to errChan
		deleteCmd.Run(createCmd, []string{})
		errChan <- nil
	}()

	return <-errChan
}
