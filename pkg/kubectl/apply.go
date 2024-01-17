package kubectl

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/kubectl/pkg/cmd/apply"
	"k8s.io/kubectl/pkg/cmd/create"
	"k8s.io/kubectl/pkg/cmd/util"

	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

var (
	applyMutex = &sync.Mutex{}
)

type ApplyManifestsOptions struct {
	DryRun    bool `default:"false"`
	Recursive bool `default:"false"`
}

type ApplyKustomizationOptions struct {
	DryRun    bool `default:"false"`
	Recursive bool `default:"false"`
}

/*
Options to how the Apply functions should behave
*/
type applyOptions struct {
	/*
		Runs a dry-run on all resources. This is the server-side dry-run i.e. kubectl apply --dry-run=server
	*/
	DryRun          bool `default:"false"`
	Recursive       bool `default:"false"`
	IsKustomization bool `default:"false"`
}

/*
ApplyManifests applies the given files to the cluster that the kubeconfigPath points to with the given ApplyManifestsOptions.

Example:

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := ApplyManifests(
		ctx,
		"/path/to/kubeconfig",
		&ApplyManifestsOptions{
		},
		[]string{
			"/path/to/manifest1.yaml",
			"/path/to/manifest2.yaml",
		},
	)
	if err != nil {
		// Handle error
	}
*/
func ApplyManifests(ctx context.Context, kubeconfigPath string, opts *ApplyManifestsOptions, filePaths []string) error {
	// Translate ApplyManifestsOptions to ApplyOptions
	applyOpts := &applyOptions{
		DryRun:          opts.DryRun,
		Recursive:       opts.Recursive,
		IsKustomization: false,
	}

	return applyFunc(ctx, kubeconfigPath, applyOpts, filePaths)
}

/*
ApplyKustomization applies the given files to the cluster that the kubeconfigPath points to with the given ApplyKustomizationOptions.

Example:

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := ApplyKustomization(
		ctx,
		"/path/to/kubeconfig",
		&ApplyKustomizationOptions{
		},
		[]string{
			"/path/to/kustomization",
		},
	)
	if err != nil {
		// Handle error
	}
*/
func ApplyKustomization(ctx context.Context, kubeconfigPath string, opts *ApplyKustomizationOptions, filePaths []string) error {
	// Translate ApplyKustomizationOptions to ApplyOptions
	applyOpts := &applyOptions{
		DryRun:          opts.DryRun,
		Recursive:       opts.Recursive,
		IsKustomization: true,
	}

	return applyFunc(ctx, kubeconfigPath, applyOpts, filePaths)
}

/*
Apply applies the given files to the cluster that the kubeconfigPath points to with the given ApplyOptions.
*/
func applyFunc(ctx context.Context, kubeconfigPath string, opts *applyOptions, filePaths []string) error {
	// We lock the mutex as we need to change the global behaviour when
	// the `kubectl apply` function encounters a fatal error
	applyMutex.Lock()
	defer applyMutex.Unlock()

	if kubeconfigPath == "" {
		return fmt.Errorf("kubeconfig path cannot be empty")
	}

	if opts == nil {
		return fmt.Errorf("options cannot be nil")
	}

	if len(filePaths) == 0 {
		return fmt.Errorf("no files to apply")
	}

	// We create empty streams - we don't want to see output from the apply command
	ioStreams, streamOut, _, streamErr := genericiooptions.NewTestIOStreams()

	config := genericclioptions.
		NewConfigFlags(true).
		WithDeprecatedPasswordFlag().
		WithDiscoveryBurst(300).
		WithDiscoveryQPS(50.0)

	config.KubeConfig = &kubeconfigPath
	f := cmdutil.NewFactory(config)

	// We create a "parent" command for the apply command,
	// for it to inherit flags from
	createCmd := create.NewCmdCreate(f, ioStreams)
	util.AddServerSideApplyFlags(createCmd)

	// We use an error channel to communicate if the apply command finished successfully or not
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

	// We set a custom handler for when the apply command encounters a fatal error
	// in kubectl, this is executed when a command fails - it prints the error message
	// and exits with the given error code
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

	applyCmd := apply.NewCmdApply("kubectl", f, ioStreams)
	applyCmd.Flags().Set("request-timeout", fmt.Sprint(int(timeLeft.Seconds())))

	if opts.DryRun {
		applyCmd.Flags().Set("dry-run", "server")
	}

	if opts.Recursive {
		applyCmd.Flags().Set("recursive", "true")
	}

	if opts.IsKustomization {
		applyCmd.Flags().Set("kustomize", strings.Join(filePaths, ","))
	} else {
		applyCmd.Flags().Set("filename", strings.Join(filePaths, ","))
	}

	go func() {
		// applyCmd is blocking. Should it fail it should have called the fatal error handler which
		// we override earlier to send an error to errChan
		applyCmd.Run(createCmd, []string{})
		errChan <- nil
	}()

	// We return the first item in the error channel
	return <-errChan
}
