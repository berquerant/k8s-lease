[![Go Report Card](https://goreportcard.com/badge/github.com/berquerant/k8s-lease)](https://goreportcard.com/report/github.com/berquerant/k8s-lease)

# k8s-lease

```
❯ klock --help
klock -- manage locks from shell scripts within Kubernetes

# Usage

  klock [flags] -- command [arguments]

klock manages the Kubernetes lease locks from shell scripts or from the command line.

klock runs the provided command (or a command with arguments) with mutual exclusion guaranteed by a lease.
klock acquires a lock via a holder identity from a lease, which is created if it does not already exist.

The following labels are always applied to leases created by klock:

app.kubernetes.io/managed-by=k8s-lease-klock

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

1 if failure.
The exit status of the given command, if klock executed it.

# Flags

      --cleanup-lease              If true, delete the created lease after processing.
  -E, --conflict-exit-code uint8   The exit status used when the -w option is in use, and the timeout is reached. (default 1)
      --debug                      Enable debug logs.
  -i, --identity string            The id of a lease holder. (default "klock")
  -k, --kill-after duration        Also send a KILL signal if command is still running this long after the initial signal was sent.
      --kubeconfig string
      --labels value               The additional labels of a lease
  -l, --lease string               The name of a lease. (default "klock")
  -n, --namespace string           The namespace of a lease. (default "default")
  -s, --signal value               Specify the signal to be sent on cancel; SIGNAL may be a name like 'HUP' or a number;
                                   default is TERM; see 'kill -l' for a list of signals
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
