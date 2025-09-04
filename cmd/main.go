//go:build linux

package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/amitschendel/curing/pkg/client"
	"github.com/amitschendel/curing/pkg/config"
)

func main() {
	ctx := context.Background()

	// Get the agent ID from the machine-id file
	agentID, err := os.ReadFile("/etc/machine-id")
	if err != nil {
		log.Fatal(err)
	}

	// Load the configuration
	cfg, err := config.LoadConfig("cmd/config.client.json")
	if err != nil {
		log.Fatal(err)
	}
	cfg.AgentID = string(agentID)

	// Create the executer
	commandExecuter, err := client.NewExecuter(ctx, 10)
	if err != nil {
		log.Fatal(err)
	}

	// Create the command puller
	puller, err := client.NewCommandPuller(cfg, ctx, commandExecuter)
	if err != nil {
		log.Fatal(err)
	}

	// Start both components
	go commandExecuter.Run()
	go puller.Run()

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	// Cleanup
	puller.Close()
	commandExecuter.Close()
}
