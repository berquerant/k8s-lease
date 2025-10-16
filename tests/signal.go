package main

//
// For TestSignal()
//

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{})))
	var (
		exit          = flag.Int("exit", 0, "")
		waitSignal    = flag.Bool("wait-signal", false, "")
		shutdownDelay = flag.Duration("delay", 0, "")
	)
	flag.Parse()
	slog.Info("[signal] Start")
	signalC := make(chan os.Signal)
	signal.Notify(signalC, syscall.SIGINT, syscall.SIGTERM)
	if *waitSignal {
		slog.Info("[signal] Wait signal")
		switch <-signalC {
		case syscall.SIGINT:
			fmt.Println("SIGINT")
		case syscall.SIGTERM:
			fmt.Println("SIGTERM")
		}
	}
	if d := *shutdownDelay; d > 0 {
		slog.Info("[signal] Wait for graceful shutdown", slog.Duration("duration", d))
		time.Sleep(d)
		slog.Info("[signal] Timed out")
	}
	os.Exit(*exit)
}
