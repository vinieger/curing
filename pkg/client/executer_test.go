//go:build linux

package client

import (
	"context"
	"log/slog"
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
		ConnectIntervalSec: 10,
		Server: config.ServerDetails{
			Host: "127.0.0.1",
			Port: testPort,
		},
	}

	parentCtx := context.Background()
	ctx, cancel := context.WithCancel(parentCtx)

	executer, err := NewExecuter(ctx)
	assert.NoError(t, err)
	defer executer.Close()

	puller, err := NewCommandPuller(cfg, ctx, executer)
	assert.NoError(t, err)
	assert.NotNil(t, puller)
	defer puller.Close()

	// Create result tracking channels
	results := make(map[string]chan struct{})
	results["command1"] = make(chan struct{})
	results["command2"] = make(chan struct{})
	results["command3"] = make(chan struct{})

	// Start components
	go executer.Run()
	go puller.Run()

	// Start result collection goroutine
	go func() {
		outputChan := executer.GetOutputChannel()
		for {
			select {
			case result := <-outputChan:
				slog.Debug("Received result", "commandID", result.CommandID)
				if ch, exists := results[result.CommandID]; exists {
					close(ch)
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Wait for all results with timeout
	timeout := time.After(15 * time.Second)
	for commandID, ch := range results {
		select {
		case <-ch:
			slog.Debug("Command completed", "commandID", commandID)
		case <-timeout:
			t.Fatalf("Timeout waiting for command %s result", commandID)
		}
	}

	// Clean up in reverse order
	cancel() // Cancel context first

	// Add cleanup delay
	time.Sleep(100 * time.Millisecond)

	// Close components
	puller.Close()
	executer.Close()
}
