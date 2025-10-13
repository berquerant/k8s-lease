# k8s-lease

```
❯ ./dist/klock --help
klock -- manage locks from shell scripts within Kubernetes

# Usage

  klock [flags] -- command [arguments]

klock manages the Kubernetes lease locks from within shell scripts or from the command line.

klock runs the provided command (or a command with arguments) with mutual exclusion guaranteed by a lease.
klock acquires a lock via a holder identity from a lease, which is created if it does not already exist.

The following labels are always applied to leases created by klock:

app.kubernetes.io/managed-by=klock

# Exit status

1 if failure.
The exit status of the given command, if klock is executed it.

# Flags
      --cleanup-lease              If true, delete the created lease after processing.
  -E, --conflict-exit-code uint8   The exit status used when the -w option is in use, and the timeout is reached. (default 1)
      --debug                      Enable debug logs.
  -i, --identity string            The id of a lease holder. (default "klock")
      --kubeconfig string
      --labels value               The additional labels of a lease
  -l, --lease string               The name of a lease. (default "klock")
  -n, --namespace string           The namespace of a lease. (default "default")
      --timeout duration           Same as --wait.
  -u, --unlock                     Same as --cleanup-lease.
      --verbose                    Same as --debug.
  -V, --version                    Display version and exit.
  -w, --wait duration              Fail if the lock cannot be acquired within the duration.
                                   0 means wait infinitely.
```

## Development

### Prerequisites

- [direnv](https://github.com/direnv/direnv)
- [kind](https://github.com/kubernetes-sigs/kind)
- go version v1.25.1+
- docker version 28.5.1+
