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
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

var (
	deleteLock = &sync.Mutex{}
)

type deleteOptions struct {
	IsKustomization bool `default:"false"`
}

func DeleteManifests(ctx context.Context, kubeconfigPath string, opts *deleteOptions, filePaths ...string) error {
	return deleteFunc(ctx, kubeconfigPath, opts, filePaths...)
}

func DeleteKustomization(ctx context.Context, kubeconfigPath string, opts *deleteOptions, filePaths ...string) error {
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
	f := cmdutil.NewFactory(config)

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
			"Fatal error: %s\nError code: %d\nOut stream: %s\nError stream: %s\n",
			msg,
			errCode,
			string(streamOut.Bytes()),
			string(streamErr.Bytes()),
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
