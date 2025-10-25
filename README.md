[![Go Report Card](https://goreportcard.com/badge/github.com/berquerant/k8s-lease)](https://goreportcard.com/report/github.com/berquerant/k8s-lease)
[![Go Reference](https://pkg.go.dev/badge/github.com/berquerant/k8s-lease.svg)](https://pkg.go.dev/github.com/berquerant/k8s-lease)

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

      --add_dir_header                   If true, adds the file directory to the header of the log messages
      --alsologtostderr                  log to standard error as well as files (no effect when -logtostderr=true)
      --cleanup-lease                    If true, delete the created lease after processing.
  -E, --conflict-exit-code uint8         The exit status used when the -w option is in use, and the timeout is reached. (default 1)
  -i, --identity string                  The id of a lease holder. (default "klock")
  -k, --kill-after duration              Also send a KILL signal if command is still running this long after the initial signal was sent.
      --kubeconfig string
      --labels value                     The additional labels of a lease
  -l, --lease string                     The name of a lease. (default "klock")
      --log_backtrace_at traceLocation   when logging hits line file:N, emit a stack trace (default :0)
      --log_dir string                   If non-empty, write log files in this directory (no effect when -logtostderr=true)
      --log_file string                  If non-empty, use this log file (no effect when -logtostderr=true)
      --log_file_max_size uint           Defines the maximum size a log file can grow to (no effect when -logtostderr=true). Unit is megabytes. If the value is 0, the maximum file size is unlimited. (default 1800)
      --logtostderr                      log to standard error instead of files (default true)
  -n, --namespace string                 The namespace of a lease. (default "default")
      --one_output                       If true, only write logs to their native severity level (vs also writing to each lower severity level; no effect when -logtostderr=true)
  -s, --signal value                     Specify the signal to be sent on cancel; SIGNAL may be a name like 'HUP' or a number;
                                         default is TERM; see 'kill -l' for a list of signals
      --skip_headers                     If true, avoid header prefixes in the log messages
      --skip_log_headers                 If true, avoid headers when opening log files (no effect when -logtostderr=true)
      --stderrthreshold severity         logs at or above this threshold go to stderr when writing to files and stderr (no effect when -logtostderr=true or -alsologtostderr=true) (default 2)
      --timeout duration                 Same as --wait.
  -u, --unlock                           Same as --cleanup-lease.
  -v, --v Level                          number for the log level verbosity
  -V, --version                          Display version and exit.
      --vmodule moduleSpec               comma-separated list of pattern=N settings for file-filtered logging
  -w, --wait duration                    Fail if the lock cannot be acquired within the duration.
                                         0 means wait infinitely.
```

## Development

### Prerequisites

- [direnv](https://github.com/direnv/direnv)
- go version v1.25.1+
- docker version 28.5.1+
