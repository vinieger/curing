//go:build linux

package executer

import (
	"context"
	"testing"
	"time"

	"github.com/amitschendel/curing/pkg/config"
	"github.com/amitschendel/curing/pkg/server"
	"github.com/stretchr/testify/assert"
)

func TestExecuterWithRealServer(t *testing.T) {
	// Start server
	testPort := 8089
	srv := server.NewServer(testPort)
	go srv.Run()

	// Give the server time to start
	time.Sleep(500 * time.Millisecond)

	// Create config
	cfg := &config.Config{
		ConnectIntervalSec: 1,
		Server: config.ServerDetails{
			Host: "127.0.0.1",
			Port: testPort,
		},
	}

	// Create components
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	executer := NewExecuter(ctx)
	puller, err := NewCommandPuller(cfg, ctx, executer)
	assert.NoError(t, err)
	assert.NotNil(t, puller)

	// Start components
	go executer.Run()
	go puller.Run()

	// Create channels to track command completion
	writeFileDone := make(chan struct{})
	executeDone := make(chan struct{})

	// Start goroutine to collect and validate command results
	go func() {
		outputChan := executer.GetOutputChannel()
		seenResults := make(map[string]bool)

		for {
			select {
			case result := <-outputChan:
				if result.CommandID == "command1" && !seenResults["command1"] {
					seenResults["command1"] = true
					close(writeFileDone)
				}
				if result.CommandID == "command2" && !seenResults["command2"] {
					seenResults["command2"] = true
					close(executeDone)
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Wait for both command results with separate timeouts
	select {
	case <-writeFileDone:
		// WriteFile command completed
	case <-time.After(15 * time.Second):
		t.Fatal("Timeout waiting for WriteFile command result")
	}

	select {
	case <-executeDone:
		// Execute command completed
	case <-time.After(15 * time.Second):
		t.Fatal("Timeout waiting for Execute command result")
	}

	// Clean up
	cancel()
	puller.Close()
	executer.Close()
}
