//go:build linux

package executer

import (
	"context"
	"log/slog"
	"sync"
	"syscall"

	"github.com/amitschendel/curing/pkg/common"
	"github.com/iceber/iouring-go"
	"golang.org/x/sys/unix"
)

type Executer struct {
	commands   chan common.Command
	output     chan common.Result
	ctx        context.Context
	cancelFunc context.CancelFunc
	closeOnce  sync.Once
	ring       *iouring.IOURing
	resultChan chan iouring.Result
}

type IExecuter interface {
	Run()
	Close()
	GetCommandChannel() chan common.Command
	GetOutputChannel() chan common.Result
}

var _ IExecuter = (*Executer)(nil)

func NewExecuter(ctx context.Context) (*Executer, error) {
	ring, err := iouring.New(32)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(ctx)
	return &Executer{
		ctx:        ctx,
		cancelFunc: cancel,
		commands:   make(chan common.Command, 100),
		output:     make(chan common.Result, 100),
		ring:       ring,
		resultChan: make(chan iouring.Result, 32),
	}, nil
}

func (e *Executer) GetCommandChannel() chan common.Command {
	return e.commands
}

func (e *Executer) GetOutputChannel() chan common.Result {
	return e.output
}

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
		result = e.handleWriteFile(c)
	case common.Execute:
		result = e.handleExecute(c)
	case common.Symlink:
		result = e.handleSymlink(c)
	default:
		slog.Error("Unknown command type", "type", cmd)
		return
	}

	select {
	case e.output <- result:
		slog.Info("Command result sent", "commandID", result.CommandID)
	case <-e.ctx.Done():
		return
	}
}

func (e *Executer) handleWriteFile(cmd common.WriteFile) common.Result {
	result := common.Result{
		CommandID: cmd.Id,
	}

	// Open file with io_uring
	flags := syscall.O_WRONLY | syscall.O_CREAT | syscall.O_TRUNC
	mode := uint32(0644)

	openReq, err := iouring.Openat(unix.AT_FDCWD, cmd.Path, uint32(flags), mode)
	if err != nil {
		result.ReturnCode = 1
		result.Output = "Failed to create open request: " + err.Error()
		return result
	}

	if _, err := e.ring.SubmitRequest(openReq, e.resultChan); err != nil {
		result.ReturnCode = 1
		result.Output = "Failed to submit open request: " + err.Error()
		return result
	}

	openRes := <-e.resultChan
	if openRes.Err() != nil {
		result.ReturnCode = 1
		result.Output = "Failed to open file: " + openRes.Err().Error()
		return result
	}

	fd := openRes.ReturnValue0().(int)
	defer e.closeFile(fd)

	// Write content using io_uring
	writeReq := iouring.Write(fd, []byte(cmd.Content))
	if _, err := e.ring.SubmitRequest(writeReq, e.resultChan); err != nil {
		result.ReturnCode = 1
		result.Output = "Failed to submit write request: " + err.Error()
		return result
	}

	writeRes := <-e.resultChan
	if writeRes.Err() != nil {
		result.ReturnCode = 1
		result.Output = "Failed to write file: " + writeRes.Err().Error()
		return result
	}

	bytesWritten := writeRes.ReturnValue0().(int)
	if bytesWritten != len(cmd.Content) {
		result.ReturnCode = 1
		result.Output = "Incomplete write operation"
		return result
	}

	result.ReturnCode = 0
	result.Output = "File written successfully"
	return result
}

func (e *Executer) handleSymlink(cmd common.Symlink) common.Result {
	result := common.Result{
		CommandID: cmd.Id,
	}

	// Create symlink with io_uring
	symlinkReq, err := iouring.Symlinkat(cmd.OldPath, unix.AT_FDCWD, cmd.NewPath)
	if err != nil {
		result.ReturnCode = 1
		result.Output = "Failed to create symlink request: " + err.Error()
		return result
	}

	if _, err := e.ring.SubmitRequest(symlinkReq, e.resultChan); err != nil {
		result.ReturnCode = 1
		result.Output = "Failed to submit symlink request: " + err.Error()
		return result
	}

	symlinkRes := <-e.resultChan
	if symlinkRes.Err() != nil {
		result.ReturnCode = 1
		result.Output = "Failed to create symlink: " + symlinkRes.Err().Error()
		return result
	}

	result.ReturnCode = 0
	result.Output = "Symlink created successfully"
	return result
}

func (e *Executer) handleExecute(cmd common.Execute) common.Result {
	// Note: For execute commands, we'll use os/exec as io_uring doesn't directly
	// handle process execution. This is just a placeholder implementation.
	// See: https://github.com/axboe/liburing/discussions/1307

	result := common.Result{
		CommandID:  cmd.Id,
		ReturnCode: 0,
		Output:     "Command executed successfully",
	}
	return result
}

func (e *Executer) closeFile(fd int) {
	closeReq := iouring.Close(fd)
	if _, err := e.ring.SubmitRequest(closeReq, e.resultChan); err != nil {
		slog.Error("Failed to submit close request", "error", err)
		return
	}

	closeRes := <-e.resultChan
	if closeRes.Err() != nil {
		slog.Error("Failed to close file", "error", closeRes.Err())
	}
}

func (e *Executer) Close() {
	e.closeOnce.Do(func() {
		slog.Info("Closing Executer")
		e.cancelFunc()

		if e.ring != nil {
			if err := e.ring.Close(); err != nil {
				slog.Error("Failed to close io_uring", "error", err)
			}
		}

		close(e.commands)
		close(e.output)
		slog.Info("Executer closed")
	})
}
