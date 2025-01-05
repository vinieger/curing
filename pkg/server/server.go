package server

import (
	"encoding/gob"
	"fmt"
	"log/slog"
	"net"
	"os"
	"time"

	"github.com/amitschendel/curing/pkg/common"
)

type Server struct {
	port int
}

func NewServer(port int) *Server {
	return &Server{
		port: port,
	}
}

func (s *Server) Run() {
	slog.Info("Starting server", "port", s.port)
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", s.port))
	if err != nil {
		slog.Error("Failed to start server", "error", err)
		os.Exit(1)
	}
	defer func(listener net.Listener) {
		_ = listener.Close()
	}(listener)

	for {
		conn, err := listener.Accept()
		if err != nil {
			slog.Error("Failed to accept the connection", "error", err)
			continue
		}
		go handleRequest(conn)
	}
}

func handleRequest(conn net.Conn) {
	defer func(conn net.Conn) {
		_ = conn.Close()
	}(conn)

	decoder := gob.NewDecoder(conn)
	encoder := gob.NewEncoder(conn)

	// receive request
	r := &common.Request{}
	if err := decoder.Decode(r); err != nil {
		slog.Error("Failed to decode request", "error", err)
		return
	}
	slog.Info("Received request", "type", r.Type)

	// send response
	switch r.Type {
	case common.GetCommands:
		commands := []common.Command{
			common.WriteFile{Id: "command1", Path: "/tmp/bad", Content: "bad"},
			common.ReadFile{Id: "command2", Path: "/tmp/bad"},
			common.Execute{Id: "command3", Command: "ls -l /tmp"},
		}

		// Set write deadline
		if conn, ok := conn.(*net.TCPConn); ok {
			if err := conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
				slog.Error("Failed to set write deadline", "error", err)
				return
			}
		}

		if err := encoder.Encode(commands); err != nil {
			slog.Error("Failed to encode commands", "error", err)
			return
		}

		if conn, ok := conn.(*net.TCPConn); ok {
			if err := conn.CloseWrite(); err != nil {
				slog.Error("Failed to close write", "error", err)
			}
		}

	case common.SendResults:
		for _, r := range r.Results {
			slog.Info("Received result", "result", r)
		}

	default:
		slog.Error("Unknown request type", "type", r.Type)
	}
}
