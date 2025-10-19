package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/berquerant/k8s-lease/kconfig"
	"github.com/berquerant/k8s-lease/lease"
	"github.com/berquerant/k8s-lease/logging"
	"github.com/berquerant/k8s-lease/process"
	versionpkg "github.com/berquerant/k8s-lease/version"
	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/labels"
	clientset "k8s.io/client-go/kubernetes"
)

const exitCodeFailure = 1

func fail(msg any) {
	failWith(exitCodeFailure, msg)
}

func failWith(exitCode int, msg any) {
	slog.Error(fmt.Sprint(msg))
	os.Exit(exitCode)
}

const usage = `klock -- manage locks from shell scripts within Kubernetes

# Usage

  klock [flags] -- command [arguments]

klock manages the Kubernetes lease locks from shell scripts or from the command line.

klock runs the provided command (or a command with arguments) with mutual exclusion guaranteed by a lease.
klock acquires a lock via a holder identity from a lease, which is created if it does not already exist.

The following labels are always applied to leases created by klock:

%s

# Examples

Suppose you have a command, some_cmd, that you want to run regularly but not concurrently.
You can execute multiple instances of some_cmd exclusively using klock.

  klock -l some_cmd_lease -i "$(uuidgen)" -- some_cmd

A unique uuid is associated with the execution of some_cmd as the holder identity.

# Permissions

The execution of klock requires permissions similar to the following role:

apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: klock-role
rules:
  - apiGroups: ["coordination.k8s.io"]
    resources: ["leases"]
    verbs: ["create", "get", "update", "patch"]

If you use --cleanup-lease, please add delete to the verbs.

# Exit status

%d if failure.
The exit status of the given command, if klock executed it.

# Flags

`

func main() {
	fs := pflag.NewFlagSet("main", pflag.ExitOnError)
	fs.Usage = func() {
		fmt.Printf(usage, lease.LabelsIntoString(lease.CommonLabels()), exitCodeFailure)
		fs.PrintDefaults()
	}
	var (
		kubeconfigPath = fs.String("kubeconfig", "", "")
		namespace      = fs.StringP("namespace", "n", "default", "The namespace of a lease.")
		name           = fs.StringP("lease", "l", "klock", "The name of a lease.")
		id             = fs.StringP("identity", "i", "klock", "The id of a lease holder.")
		cleanupLease   = fs.Bool("cleanup-lease", false, "If true, delete the created lease after processing.")
		unlock         = fs.BoolP("unlock", "u", false, "Same as --cleanup-lease.")
		wait           = fs.DurationP("wait", "w", 0,
			`Fail if the lock cannot be acquired within the duration.
0 means wait infinitely.`)
		timeout          = fs.Duration("timeout", 0, "Same as --wait.")
		conflictExitCode = fs.Uint8P("conflict-exit-code", "E", exitCodeFailure,
			`The exit status used when the -w option is in use, and the timeout is reached.`)
		killAfter = fs.DurationP("kill-after", "k", 0,
			"Also send a KILL signal if command is still running this long after the initial signal was sent.")
		debug                      = fs.Bool("debug", false, "Enable debug logs.")
		verbose                    = fs.Bool("verbose", false, "Same as --debug.")
		version                    = fs.BoolP("version", "V", false, "Display version and exit.")
		cancelSignal     os.Signal = syscall.SIGTERM
		additionalLabels labels.Set
	)
	fs.Func("labels", "The additional labels of a lease", func(v string) error {
		x, err := lease.ParseLabelsFromString(v)
		if err != nil {
			return err
		}
		additionalLabels = x
		return nil
	})
	fs.FuncP("signal", "s", `Specify the signal to be sent on cancel; SIGNAL may be a name like 'HUP' or a number;
default is TERM; see 'kill -l' for a list of signals`, func(v string) error {
		if x, ok := process.NewSignal(v); ok {
			cancelSignal = x
			return nil
		}
		return errors.New("UnknownSignal")
	})
	err := fs.Parse(os.Args)
	if errors.Is(err, pflag.ErrHelp) {
		return
	}
	if err != nil {
		fail(fmt.Errorf("%w: failed to parse flags", err))
	}
	if *version {
		versionpkg.Write(os.Stdout)
		return
	}
	logging.Setup(os.Stderr, *debug || *verbose)

	args, err := commandArgs(fs)
	if err != nil {
		fail(fmt.Errorf("%w: invalid program and arguments to be executed", err))
	}

	kubeconfig, err := kconfig.Build(*kubeconfigPath)
	if err != nil {
		fail(fmt.Errorf("%w: failed to build kubeconfig", err))
	}
	client, err := clientset.NewForConfig(kubeconfig)
	if err != nil {
		fail(fmt.Errorf("%w: failed to create client", err))
	}
	locker, err := lease.NewLocker(
		*namespace, *name, *id, client.CoordinationV1(),
		lease.WithCleanupLease(*cleanupLease || *unlock),
		lease.WithLabels(additionalLabels),
	)
	if err != nil {
		fail(fmt.Errorf("%w: failed to create locker", err))
	}
	proc := process.NewProcess(locker, args[0], args[1:]...)
	proc.Stdin = os.Stdin
	proc.Stdout = os.Stdout
	proc.Stderr = os.Stderr
	proc.WaitDelay = *killAfter
	proc.CancelSignal = cancelSignal
	ctx, stop := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGPIPE, syscall.SIGTERM)
	err = proc.Run(ctx, lease.WithLeaderElectTimeout(max(*wait, *timeout)))
	stop()
	if err != nil {
		if errors.Is(err, lease.ErrElectTimedOut) {
			failWith(int(*conflictExitCode), err)
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			failWith(exitErr.ExitCode(), err)
		}
		fail(err)
	}
}

var (
	errNoProgram         = errors.New("NoProgram")
	errProgramBeforeDash = errors.New("ProgramBeforeDash")
)

func commandArgs(fs *pflag.FlagSet) ([]string, error) {
	dashAt := fs.ArgsLenAtDash()
	if dashAt < 0 {
		return nil, errNoProgram
	}
	if dashAt != 1 {
		return nil, errProgramBeforeDash
	}
	args := fs.Args()[dashAt:]
	if len(args) == 0 {
		return nil, errNoProgram
	}
	return args, nil
}
