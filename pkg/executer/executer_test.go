//go:build linux

package executer

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/amitschendel/curing/pkg/common"
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

	// Create channels to track command completion
	command1Done := make(chan struct{})
	command2Done := make(chan struct{})

	// Start components
	go executer.Run()
	go puller.Run()

	// Start goroutine to collect and validate commands
	go func() {
		commandsChan := executer.GetCommandChannel()
		seenCommands := make(map[string]bool)

		for {
			select {
			case cmd := <-commandsChan:
				switch c := cmd.(type) {
				case common.WriteFile:
					if c.Id == "command1" && !seenCommands["command1"] {
						seenCommands["command1"] = true
						close(command1Done)
					}
				case common.Execute:
					if c.Id == "command2" && !seenCommands["command2"] {
						seenCommands["command2"] = true
						close(command2Done)
					}
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Wait for both commands with separate timeouts
	select {
	case <-command1Done:
		// Command 1 received
	case <-time.After(15 * time.Second):
		t.Fatal("Timeout waiting for command1")
	}

	select {
	case <-command2Done:
		// Command 2 received
	case <-time.After(15 * time.Second):
		t.Fatal("Timeout waiting for command2")
	}

	// Clean up
	cancel()
	puller.Close()
	executer.Close()
}

func TestExecuterReconnection(t *testing.T) {
	// Start server
	testPort := 8090
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", testPort))
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}

	srv := server.NewServer(testPort)

	// Create a channel to signal when the server has fully stopped
	serverStopped := make(chan struct{})

	go func() {
		srv.Run()
		close(serverStopped)
	}()

	// Give the server time to start
	time.Sleep(500 * time.Millisecond)

	// Create components with short interval
	cfg := &config.Config{
		ConnectIntervalSec: 1,
		Server: config.ServerDetails{
			Host: "127.0.0.1",
			Port: testPort,
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	executer := NewExecuter(ctx)
	puller, err := NewCommandPuller(cfg, ctx, executer)
	assert.NoError(t, err)

	// Start components
	go executer.Run()
	go puller.Run()

	commandsChan := executer.GetCommandChannel()

	// Wait for first command
	var firstCommand common.Command
	select {
	case cmd := <-commandsChan:
		slog.Info("Received first command", "command", cmd)
		firstCommand = cmd
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for first command")
	}

	// Verify first command was received
	assert.NotNil(t, firstCommand, "Should receive a command")

	// Stop server and wait for it to fully stop
	listener.Close()
	<-serverStopped
	time.Sleep(200 * time.Millisecond) // Give OS time to release the port

	// Start server again with new listener
	newListener, err := net.Listen("tcp", fmt.Sprintf(":%d", testPort))
	if err != nil {
		t.Fatalf("Failed to create new listener: %v", err)
	}
	defer newListener.Close()

	srv = server.NewServer(testPort)
	go srv.Run()

	time.Sleep(200 * time.Millisecond) // Give new server time to start

	// Wait for command after reconnection
	select {
	case cmd := <-commandsChan:
		slog.Info("Received command after reconnection", "command", cmd)
		assert.NotNil(t, cmd, "Should receive a command after reconnection")
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for command after reconnection")
	}

	// Cleanup
	puller.Close()
	executer.Close()
}

func TestExecuterMultipleClients(t *testing.T) {
	// Start server
	testPort := 8091
	srv := server.NewServer(testPort)
	go srv.Run()

	// Give the server time to start
	time.Sleep(100 * time.Millisecond)

	// Create base config
	baseConfig := &config.Config{
		ConnectIntervalSec: 1,
		Server: config.ServerDetails{
			Host: "127.0.0.1",
			Port: testPort,
		},
	}

	// Start multiple clients
	numClients := 3
	executers := make([]*Executer, numClients)
	pullers := make([]*CommandPuller, numClients)
	commandChannels := make([]chan common.Command, numClients)

	for i := 0; i < numClients; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		executer := NewExecuter(ctx)
		puller, err := NewCommandPuller(baseConfig, ctx, executer)
		assert.NoError(t, err)

		executers[i] = executer
		pullers[i] = puller
		commandChannels[i] = executer.GetCommandChannel()

		go executer.Run()
		go puller.Run()
	}

	// Wait and verify that all clients receive commands
	for i := 0; i < numClients; i++ {
		select {
		case cmd := <-commandChannels[i]:
			assert.NotNil(t, cmd)
		case <-time.After(2 * time.Second):
			t.Fatalf("Timeout waiting for commands on client %d", i)
		}
	}

	// Clean up
	for i := 0; i < numClients; i++ {
		pullers[i].Close()
		executers[i].Close()
	}
}
