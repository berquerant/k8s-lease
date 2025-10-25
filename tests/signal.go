package main

//
// For TestSignal()
//

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"k8s.io/klog/v2"
)

func main() {
	klog.InitFlags(nil)
	var (
		exit          = flag.Int("exit", 0, "")
		waitSignal    = flag.Bool("wait-signal", false, "")
		shutdownDelay = flag.Duration("delay", 0, "")
	)
	flag.Parse()
	logger := klog.NewKlogr().WithName("signal")
	logger.V(0).Info("start")
	signalC := make(chan os.Signal, 1)
	signal.Notify(signalC, syscall.SIGINT, syscall.SIGTERM)
	if *waitSignal {
		logger.V(0).Info("wait signal")
		switch <-signalC {
		case syscall.SIGINT:
			fmt.Println("SIGINT")
		case syscall.SIGTERM:
			fmt.Println("SIGTERM")
		}
	}
	if d := *shutdownDelay; d > 0 {
		logger.V(0).Info("wait for graceful shutdown", "duration", d)
		time.Sleep(d)
		logger.V(0).Info("timed out")
	}
	os.Exit(*exit)
}
