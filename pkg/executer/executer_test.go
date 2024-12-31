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

	// Create executer config
	cfg := &config.Config{
		ConnectIntervalSec: 1,
		Server: config.ServerDetails{
			Host: "127.0.0.1",
			Port: testPort,
		},
	}

	// Create executer
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	executer, err := NewExecuter(cfg, ctx)
	assert.NoError(t, err)
	assert.NotNil(t, executer)

	// Create channels to collect commands
	commandsChan := make(chan common.Command, 10)
	executer.SetCommandsChannel(commandsChan)
	outputChan := executer.GetOutputChannel()

	// Start executer in background
	go executer.Run()

	// Wait for commands from server
	var receivedCommands []common.Command
	commandTimeout := time.After(5 * time.Second)

	slog.Info("Waiting for commands...")
	for i := 0; i < 2; { // We expect 2 commands from the server
		select {
		case cmd := <-commandsChan:
			slog.Info("Received command", "type", fmt.Sprintf("%T", cmd), "command", cmd)
			receivedCommands = append(receivedCommands, cmd)
			i++
		case <-commandTimeout:
			t.Fatal("Timeout waiting for commands")
		}
	}

	// Verify received commands
	assert.Len(t, receivedCommands, 2, "Should receive exactly 2 commands")

	// Verify first command (WriteFile)
	cmd1 := receivedCommands[0]
	slog.Info("Examining command 1", "type", fmt.Sprintf("%T", cmd1), "value", fmt.Sprintf("%+v", cmd1))
	if writeCmd, ok := cmd1.(common.WriteFile); ok {
		assert.Equal(t, "command1", writeCmd.Id)
		assert.Equal(t, "/tmp/bad", writeCmd.Path)
	} else {
		t.Fatalf("Expected first command to be WriteFile, got %T", cmd1)
	}

	// Verify second command (Execute)
	cmd2 := receivedCommands[1]
	slog.Info("Examining command 2", "type", fmt.Sprintf("%T", cmd2), "value", fmt.Sprintf("%+v", cmd2))
	if execCmd, ok := cmd2.(common.Execute); ok {
		assert.Equal(t, "command2", execCmd.Id)
		assert.Equal(t, "ls -l /tmp", execCmd.Command)
	} else {
		t.Fatalf("Expected second command to be Execute, got %T", cmd2)
	}

	// Send results back
	results := []common.Result{
		{
			CommandID:  "command1",
			ReturnCode: 0,
			Output:     "File written successfully",
		},
		{
			CommandID:  "command2",
			ReturnCode: 0,
			Output:     "Command executed successfully",
		},
	}

	// Send results
	for _, result := range results {
		outputChan <- result
	}

	// Give time for results to be processed
	time.Sleep(500 * time.Millisecond)

	// Clean up
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

	// Create executer with short interval
	cfg := &config.Config{
		ConnectIntervalSec: 1,
		Server: config.ServerDetails{
			Host: "127.0.0.1",
			Port: testPort,
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	executer, err := NewExecuter(cfg, ctx)
	assert.NoError(t, err)

	commandsChan := make(chan common.Command, 10)
	executer.SetCommandsChannel(commandsChan)

	// Start executer
	go executer.Run()

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
	cancel()
	time.Sleep(100 * time.Millisecond)
	executer.Close()
	newListener.Close()
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

	// Start multiple executers
	numClients := 3
	executers := make([]*Executer, numClients)
	commandsChans := make([]chan common.Command, numClients)

	for i := 0; i < numClients; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		executer, err := NewExecuter(baseConfig, ctx)
		assert.NoError(t, err)

		commandsChan := make(chan common.Command, 10)
		executer.SetCommandsChannel(commandsChan)

		executers[i] = executer
		commandsChans[i] = commandsChan

		go executer.Run()
	}

	// Wait and verify that all clients receive commands
	for i := 0; i < numClients; i++ {
		select {
		case cmd := <-commandsChans[i]:
			assert.NotNil(t, cmd)
		case <-time.After(2 * time.Second):
			t.Fatalf("Timeout waiting for commands on client %d", i)
		}
	}

	// Clean up
	for _, executer := range executers {
		executer.Close()
	}
}
