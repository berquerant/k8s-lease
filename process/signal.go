package process

import (
	"os"
	"strconv"
	"syscall"

	"golang.org/x/sys/unix"
)

func signalFromString(s string) (syscall.Signal, bool) {
	x := unix.SignalNum(s)
	return x, x != syscall.Signal(0)
}

func signalFromInt(i int) (syscall.Signal, bool) {
	x := syscall.Signal(i)
	return x, unix.SignalName(x) != ""
}

// NewSignal returns the Signal for signal named x or numbered x.
//
// Examples:
//
//   - "INT" -> syscall.SIGINT
//   - 2        -> syscall.SIGINT
//   - "2"      -> syscall.SIGINT
func NewSignal[T string | int](x T) (syscall.Signal, bool) {
	switch v := any(x).(type) {
	case string:
		if i, err := strconv.Atoi(v); err == nil {
			if s, ok := signalFromInt(i); ok {
				return s, true
			}
		}
		return signalFromString("SIG" + v)
	case int:
		return signalFromInt(v)
	default:
		panic("unreachable")
	}
}

func SignalIntoInt(s os.Signal) (int, bool) {
	if v, ok := s.(syscall.Signal); ok {
		return int(v), true
	}
	return 0, false
}

func SignalIntoString(s os.Signal) string {
	if v, ok := s.(syscall.Signal); ok {
		return unix.SignalName(v)
	}
	return s.String()
}
