// Copyright (c) 2020 Red Hat, Inc.

// +build testrunmain

package main

import (
	"os"
	"os/signal"
	"syscall"
	"testing"
)

func TestRunMain(t *testing.T) {
	go main()
	// hacks for handling signals
	signalChannel := make(chan os.Signal, 2)
	signal.Notify(signalChannel, os.Interrupt, syscall.SIGTERM)
	func() {
		sig := <-signalChannel
		switch sig {
		case os.Interrupt:
			//handle SIGINT
			return
		case syscall.SIGTERM:
			//handle SIGTERM
			return
		}
	}()
}
