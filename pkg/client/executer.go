//go:build linux

package client

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
	workerPool chan struct{} // Semaphore for limiting concurrent workers
	numWorkers int           // Number of workers in the pool
}

type IExecuter interface {
	Run()
	Close()
	GetCommandChannel() chan common.Command
	GetOutputChannel() chan common.Result
}

var _ IExecuter = (*Executer)(nil)

func NewExecuter(ctx context.Context, numWorkers int) (*Executer, error) {
	if numWorkers <= 0 {
		numWorkers = 10 // Default to 10 workers if not specified
	}

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
		workerPool: make(chan struct{}, numWorkers), // Semaphore with capacity numWorkers
		numWorkers: numWorkers,
	}, nil
}

func (e *Executer) GetCommandChannel() chan common.Command {
	return e.commands
}

func (e *Executer) GetOutputChannel() chan common.Result {
	return e.output
}

func (e *Executer) Run() {
	slog.Debug("Starting Executer", "workers", e.numWorkers)

	// Start the worker pool
	var wg sync.WaitGroup

	// Launch worker goroutines
	for i := 0; i < e.numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			e.worker(workerID)
		}(i)
	}

	// Wait for context cancellation then wait for all workers to finish
	<-e.ctx.Done()
	wg.Wait()
	slog.Debug("Executer context cancelled, all workers stopped")
}

func (e *Executer) worker(workerID int) {
	slog.Debug("Starting worker", "workerID", workerID)

	for {
		select {
		case <-e.ctx.Done():
			slog.Debug("Worker exiting due to context cancellation", "workerID", workerID)
			return
		case cmd, ok := <-e.commands:
			if !ok {
				slog.Debug("Command channel closed, worker exiting", "workerID", workerID)
				return // Channel closed
			}

			// Acquire a token from the worker pool
			e.workerPool <- struct{}{}

			// Create a context that's cancelled when the parent context is cancelled
			cmdCtx, cancel := context.WithCancel(e.ctx)

			// Log which command is being processed
			cmdType := "unknown"
			cmdID := "unknown"

			switch cmd := cmd.(type) {
			case common.WriteFile:
				cmdType = "WriteFile"
				cmdID = cmd.Id
			case common.Execute:
				cmdType = "Execute"
				cmdID = cmd.Id
			case common.Symlink:
				cmdType = "Symlink"
				cmdID = cmd.Id
			case common.ReadFile:
				cmdType = "ReadFile"
				cmdID = cmd.Id
			}

			slog.Info("Worker processing command", "workerID", workerID, "commandType", cmdType, "commandID", cmdID)

			// Execute the command and send the result
			result := e.executeCommand(cmdCtx, cmd)

			// Release the token back to the pool
			<-e.workerPool

			cancel() // Clean up the command context

			// Log that we're about to send the result
			slog.Info("Worker sending result", "workerID", workerID, "commandID", result.CommandID)

			// Only send the result if we haven't been cancelled
			select {
			case e.output <- result:
				slog.Debug("Command result sent", "workerID", workerID, "commandID", result.CommandID)
			case <-e.ctx.Done():
				slog.Debug("Context cancelled while sending result", "workerID", workerID)
				return
			}
		}
	}
}

func (e *Executer) executeCommand(ctx context.Context, cmd common.Command) common.Result {
	var result common.Result

	switch c := cmd.(type) {
	case common.WriteFile:
		result = e.handleWriteFile(ctx, c)
	case common.Execute:
		result = e.handleExecute(ctx, c)
	case common.Symlink:
		result = e.handleSymlink(ctx, c)
	case common.ReadFile:
		result = e.handleReadFile(ctx, c)
		// For debugging purposes
		slog.Info("Command executed", "commandID", result.CommandID, "outputLength", len(result.Output))
	default:
		slog.Error("Unknown command type", "type", cmd)
		return common.Result{
			CommandID:  getCommandID(cmd),
			ReturnCode: 1,
			Output:     []byte("Unknown command type"),
		}
	}

	return result
}

// Helper function to extract command ID from any command type
func getCommandID(cmd common.Command) string {
	switch c := cmd.(type) {
	case common.WriteFile:
		return c.Id
	case common.Execute:
		return c.Id
	case common.Symlink:
		return c.Id
	case common.ReadFile:
		return c.Id
	default:
		return "unknown"
	}
}

func (e *Executer) handleWriteFile(ctx context.Context, cmd common.WriteFile) common.Result {
	result := common.Result{
		CommandID: cmd.Id,
	}

	// Open file with io_uring
	flags := syscall.O_WRONLY | syscall.O_CREAT | syscall.O_TRUNC
	mode := uint32(0644)

	openReq, err := iouring.Openat(unix.AT_FDCWD, cmd.Path, uint32(flags), mode)
	if err != nil {
		result.ReturnCode = 1
		result.Output = []byte("Failed to create open request: " + err.Error())
		return result
	}

	if _, err := e.ring.SubmitRequest(openReq, e.resultChan); err != nil {
		result.ReturnCode = 1
		result.Output = []byte("Failed to submit open request: " + err.Error())
		return result
	}

	select {
	case openRes := <-e.resultChan:
		if openRes.Err() != nil {
			result.ReturnCode = 1
			result.Output = []byte("Failed to open file: " + openRes.Err().Error())
			return result
		}

		fd := openRes.ReturnValue0().(int)
		defer e.closeFile(fd)

		// Write content using io_uring
		writeReq := iouring.Write(fd, []byte(cmd.Content))
		if _, err := e.ring.SubmitRequest(writeReq, e.resultChan); err != nil {
			result.ReturnCode = 1
			result.Output = []byte("Failed to submit write request: " + err.Error())
			return result
		}

		select {
		case writeRes := <-e.resultChan:
			if writeRes.Err() != nil {
				result.ReturnCode = 1
				result.Output = []byte("Failed to write file: " + writeRes.Err().Error())
				return result
			}

			bytesWritten := writeRes.ReturnValue0().(int)
			if bytesWritten != len(cmd.Content) {
				result.ReturnCode = 1
				result.Output = []byte("Incomplete write operation")
				return result
			}

			result.ReturnCode = 0
			result.Output = []byte("File written successfully")
			return result
		case <-ctx.Done():
			result.ReturnCode = 1
			result.Output = []byte("Operation cancelled")
			return result
		}
	case <-ctx.Done():
		result.ReturnCode = 1
		result.Output = []byte("Operation cancelled")
		return result
	}
}

func (e *Executer) handleSymlink(ctx context.Context, cmd common.Symlink) common.Result {
	result := common.Result{
		CommandID: cmd.Id,
	}

	// Create symlink with io_uring
	symlinkReq, err := iouring.Symlinkat(cmd.OldPath, unix.AT_FDCWD, cmd.NewPath)
	if err != nil {
		result.ReturnCode = 1
		result.Output = []byte("Failed to create symlink request: " + err.Error())
		return result
	}

	if _, err := e.ring.SubmitRequest(symlinkReq, e.resultChan); err != nil {
		result.ReturnCode = 1
		result.Output = []byte("Failed to submit symlink request: " + err.Error())
		return result
	}

	select {
	case symlinkRes := <-e.resultChan:
		if symlinkRes.Err() != nil {
			result.ReturnCode = 1
			result.Output = []byte("Failed to create symlink: " + symlinkRes.Err().Error())
			return result
		}

		result.ReturnCode = 0
		result.Output = []byte("Symlink created successfully")
		return result
	case <-ctx.Done():
		result.ReturnCode = 1
		result.Output = []byte("Operation cancelled")
		return result
	}
}

func (e *Executer) handleExecute(ctx context.Context, cmd common.Execute) common.Result {
	// Note: For execute commands, we'll use os/exec as io_uring doesn't directly
	// handle process execution. This is just a placeholder implementation.
	// See: https://github.com/axboe/liburing/discussions/1307

	result := common.Result{
		CommandID:  cmd.Id,
		ReturnCode: 0,
		Output:     []byte("Command executed successfully"),
	}
	return result
}

func (e *Executer) handleReadFile(ctx context.Context, cmd common.ReadFile) common.Result {
	result := common.Result{
		CommandID: cmd.Id,
	}

	// Open file with io_uring
	flags := syscall.O_RDONLY
	openReq, err := iouring.Openat(unix.AT_FDCWD, cmd.Path, uint32(flags), 0)
	if err != nil {
		result.ReturnCode = 1
		result.Output = []byte("Failed to create open request: " + err.Error())
		return result
	}

	if _, err := e.ring.SubmitRequest(openReq, e.resultChan); err != nil {
		result.ReturnCode = 1
		result.Output = []byte("Failed to submit open request: " + err.Error())
		return result
	}

	select {
	case openRes := <-e.resultChan:
		if openRes.Err() != nil {
			result.ReturnCode = 1
			result.Output = []byte("Failed to open file: " + openRes.Err().Error())
			return result
		}

		fd := openRes.ReturnValue0().(int)
		defer e.closeFile(fd)

		// Get file size using io_uring statx
		var statxBuf unix.Statx_t
		statxReq, err := iouring.Statx(fd, "", unix.AT_EMPTY_PATH, unix.STATX_SIZE, &statxBuf)
		if err != nil {
			result.ReturnCode = 1
			result.Output = []byte("Failed to create statx request: " + err.Error())
			return result
		}

		if _, err := e.ring.SubmitRequest(statxReq, e.resultChan); err != nil {
			result.ReturnCode = 1
			result.Output = []byte("Failed to submit statx request: " + err.Error())
			return result
		}

		select {
		case statxRes := <-e.resultChan:
			if statxRes.Err() != nil {
				result.ReturnCode = 1
				result.Output = []byte("Failed to get file size: " + statxRes.Err().Error())
				return result
			}

			// Pre-allocate buffer based on file size
			fileSize := int64(statxBuf.Size)
			output := make([]byte, 0, fileSize)

			// Read file in chunks using io_uring
			const chunkSize = 32 * 1024 // 32KB chunks
			var offset int64 = 0

			for offset < fileSize {
				// Check for context cancellation
				select {
				case <-ctx.Done():
					result.ReturnCode = 1
					result.Output = []byte("Operation cancelled")
					return result
				default:
					// Continue processing
				}

				// Calculate the size of the next chunk
				remaining := fileSize - offset
				currentChunkSize := chunkSize
				if remaining < chunkSize {
					currentChunkSize = int(remaining)
				}

				// Prepare buffer and read request
				buf := make([]byte, currentChunkSize)
				readReq := iouring.Pread(fd, buf, uint64(offset))
				if _, err := e.ring.SubmitRequest(readReq, e.resultChan); err != nil {
					result.ReturnCode = 1
					result.Output = []byte("Failed to submit read request: " + err.Error())
					return result
				}

				select {
				case readRes := <-e.resultChan:
					if readRes.Err() != nil {
						result.ReturnCode = 1
						result.Output = []byte("Failed to read file: " + readRes.Err().Error())
						return result
					}

					bytesRead := readRes.ReturnValue0().(int)
					if bytesRead <= 0 {
						break
					}

					output = append(output, buf[:bytesRead]...)
					offset += int64(bytesRead)
				case <-ctx.Done():
					result.ReturnCode = 1
					result.Output = []byte("Operation cancelled")
					return result
				}
			}

			result.ReturnCode = 0
			result.Output = output
			return result
		case <-ctx.Done():
			result.ReturnCode = 1
			result.Output = []byte("Operation cancelled")
			return result
		}
	case <-ctx.Done():
		result.ReturnCode = 1
		result.Output = []byte("Operation cancelled")
		return result
	}
}

func (e *Executer) closeFile(fd int) {
	closeReq := iouring.Close(fd)
	if _, err := e.ring.SubmitRequest(closeReq, e.resultChan); err != nil {
		slog.Error("Failed to submit close request", "error", err)
		return
	}

	select {
	case closeRes := <-e.resultChan:
		if closeRes.Err() != nil {
			slog.Error("Failed to close file", "error", closeRes.Err())
		}
	case <-e.ctx.Done():
		slog.Error("Failed to close file: context cancelled")
	}
}

func (e *Executer) Close() {
	e.closeOnce.Do(func() {
		slog.Debug("Closing Executer")
		e.cancelFunc()

		if e.ring != nil {
			if err := e.ring.Close(); err != nil {
				slog.Error("Failed to close io_uring", "error", err)
			}
		}

		close(e.commands)
		close(e.output)
		slog.Debug("Executer closed")
	})
}
