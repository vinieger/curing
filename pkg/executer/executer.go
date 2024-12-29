//go:build linux

package executer

import (
	"context"
	"log/slog"
	"net"
	"syscall"

	"github.com/amitschendel/curing/pkg/config"
	"github.com/iceber/iouring-go"
	"github.com/moby/moby/libnetwork/config"
)

type Executer struct {
	commands chan string
	output   chan string

	ring       *iouring.IOURing
	cfg        *config.Config
	resultChan chan iouring.Result

	ctx context.Context
}

var _ IExecuter = &Executer{}

func NewExecuter(cfg config.Config, ctx context.Context) (*Executer, error) {
	ring, err := iouring.New(32)
	if err != nil {
		return nil, err
	}
	return &Executer{
		ring: ring,
		cfg:  cfg,
		ctx:  ctx,
	}, nil
}

// ----- Implementing the IExecuter interface -----
func (e *Executer) SetCommandsChannel(commands chan string) {
	e.commands = commands
}

func (e *Executer) GetOutputChannel() chan string {
	return e.output
}

// -------------------------------------------------

// ----- Implementing the Executer methods -----
func (e *Executer) Run() {
	for {
		select {
		case cmd := <-e.commands:
			slog.Info("Received command", "command", cmd)
			// TODO: Implement the command execution logic.
		case <-e.ctx.Done():
			e.Close()
			return
		}
	}
}

func (e *Executer) Close() {
	if e.ring != nil {
		e.ring.Close()
	}
	slog.Info("Executer closed")
}

// ----- IO_URING related methods -----

// Generic method to submit a request to the ring.
func (e *Executer) SubmitRequest(request iouring.PrepRequest) error {
	_, err := e.ring.SubmitRequest(request, e.resultChan)
	return err
}

// Connect to a server and return a file descriptor.
func (e *Executer) Connect() (int, error) {
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

	return sockfd, nil
}

// Read from a file descriptor.
func (e *Executer) Read(fd int, buf []byte) (int, error) {
	return e.ring.Read(fd, buf)
}

// -------------------------------------------------
