// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

//go:build testrunmain
// +build testrunmain

package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"testing"

	"github.com/stolostron/managedcluster-import-controller/pkg/features"
)

// start the controller to get the test coverage
func TestRunMain(t *testing.T) {
	if err := features.DefaultMutableFeatureGate.Set(fmt.Sprintf("%s=true", features.KlusterletHostedMode)); err != nil {
		panic(err)
	}
	go main()
	// hacks for handling signals
	signalChannel := make(chan os.Signal, 2)
	signal.Notify(signalChannel, os.Interrupt, syscall.SIGTERM)
	func() {
		sig := <-signalChannel
		switch sig {
		case os.Interrupt:
			fmt.Printf("Signal Interupt: %s", sig.String())
			return
		case syscall.SIGTERM:
			//handle SIGTERM
			fmt.Printf("Signal SIGTERM: %s", sig.String())
			return
		}
	}()
}
