//go:build linux

package executer

import (
	"context"
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
	time.Sleep(100 * time.Millisecond)

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

	// Create channels to collect commands and send results
	commandsChan := make(chan common.Command, 10)
	// _ := make(chan common.Result, 10)

	executer.SetCommandsChannel(commandsChan)
	outputChan := executer.GetOutputChannel()

	// Start executer in background
	go executer.Run()

	// Wait for commands from server
	var receivedCommands []common.Command
	commandTimeout := time.After(2 * time.Second)

	for i := 0; i < 2; { // We expect 2 commands from the server
		select {
		case cmd := <-commandsChan:
			receivedCommands = append(receivedCommands, cmd)
			i++
		case <-commandTimeout:
			t.Fatal("Timeout waiting for commands")
		}
	}

	// Verify received commands
	assert.Len(t, receivedCommands, 2)

	// Type assert and verify command details
	writeCmd, ok := receivedCommands[0].(*common.WriteFile)
	assert.True(t, ok)
	assert.Equal(t, "command1", writeCmd.Id)
	assert.Equal(t, "/tmp/bad", writeCmd.Path)

	execCmd, ok := receivedCommands[1].(*common.Execute)
	assert.True(t, ok)
	assert.Equal(t, "command2", execCmd.Id)
	assert.Equal(t, "ls -l /tmp", execCmd.Command)

	// Send some results back
	results := []common.Result{
		{
			CommandID:  "command1",
			ReturnCode: 1,
			Output:     "File written successfully",
		},
		{
			CommandID:  "command2",
			ReturnCode: 1,
			Output:     "Command executed successfully",
		},
	}

	for _, result := range results {
		outputChan <- result
	}

	// Give some time for results to be processed
	time.Sleep(500 * time.Millisecond)

	// Clean up
	executer.Close()
}

func TestExecuterReconnection(t *testing.T) {
	// Start server
	testPort := 8090
	srv := server.NewServer(testPort)
	_, serverCancel := context.WithCancel(context.Background())
	defer serverCancel()

	go func() {
		srv.Run()
	}()

	// Give the server time to start
	time.Sleep(100 * time.Millisecond)

	// Create executer with short interval
	cfg := &config.Config{
		ConnectIntervalSec: 1,
		Server: config.ServerDetails{
			Host: "127.0.0.1",
			Port: testPort,
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	executer, err := NewExecuter(cfg, ctx)
	assert.NoError(t, err)

	commandsChan := make(chan common.Command, 10)
	executer.SetCommandsChannel(commandsChan)

	// Start executer
	go executer.Run()

	// Wait for first command
	select {
	case <-commandsChan:
		// Got first command successfully
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for first command")
	}

	// Stop server
	serverCancel()
	time.Sleep(100 * time.Millisecond)

	// Start server again
	srv = server.NewServer(testPort)
	go srv.Run()

	// Wait for command after reconnection
	select {
	case <-commandsChan:
		// Got command after reconnection
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for command after reconnection")
	}

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
