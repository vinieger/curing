//go:build linux

package executer

import (
	"context"
	"log/slog"
	"sync"

	"github.com/amitschendel/curing/pkg/common"
)

type IExecuter interface {
	Run()
	Close()
	GetCommandChannel() chan common.Command
	GetOutputChannel() chan common.Result
}

type Executer struct {
	commands   chan common.Command
	output     chan common.Result
	ctx        context.Context
	cancelFunc context.CancelFunc
	closeOnce  sync.Once
}

var _ IExecuter = (*Executer)(nil)

func NewExecuter(ctx context.Context) *Executer {
	ctx, cancel := context.WithCancel(ctx)
	return &Executer{
		ctx:        ctx,
		cancelFunc: cancel,
		commands:   make(chan common.Command, 100),
		output:     make(chan common.Result, 100),
	}
}

func (e *Executer) GetCommandChannel() chan common.Command {
	return e.commands
}

func (e *Executer) GetOutputChannel() chan common.Result {
	return e.output
}

// Run handles the main execution loop
func (e *Executer) Run() {
	slog.Info("Starting Executer")
	for {
		select {
		case <-e.ctx.Done():
			slog.Info("Executer context cancelled")
			return
		case cmd := <-e.commands:
			go e.executeCommand(cmd)
		}
	}
}

func (e *Executer) executeCommand(cmd common.Command) {
	var result common.Result

	switch c := cmd.(type) {
	case common.WriteFile:
		slog.Info("Executing WriteFile command", "commandID", c.Id, "path", c.Path)
		// Add execution logic for WriteFile command here
		result = common.Result{
			CommandID:  c.Id,
			ReturnCode: 0,
			Output:     "File written successfully",
		}
		// Make sure to send result
		select {
		case e.output <- result:
			slog.Info("WriteFile command result sent", "commandID", result.CommandID)
		case <-e.ctx.Done():
			return
		}

	case common.Execute:
		slog.Info("Executing Execute command", "commandID", c.Id, "command", c.Command)
		result = common.Result{
			CommandID:  c.Id,
			ReturnCode: 0,
			Output:     "Command executed successfully",
		}
		select {
		case e.output <- result:
			slog.Info("Execute command result sent", "commandID", result.CommandID)
		case <-e.ctx.Done():
			return
		}

	default:
		slog.Error("Unknown command type", "type", cmd)
		return
	}
}

// Close gracefully shuts down the Executer
func (e *Executer) Close() {
	e.closeOnce.Do(func() {
		slog.Info("Closing Executer")
		e.cancelFunc()
		close(e.commands)
		close(e.output)
		slog.Info("Executer closed")
	})
}
