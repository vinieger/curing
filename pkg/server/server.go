package server

import (
	"encoding/gob"
	"fmt"
	"log/slog"
	"net"
	"os"

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

	encoder := gob.NewEncoder(conn)
	decoder := gob.NewDecoder(conn)

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
		c := []common.Command{
			&common.WriteFile{Id: "command1", Path: "/tmp/bad"},
			&common.Execute{Id: "command2", Command: "ls -l /tmp"},
		}
		if err := encoder.Encode(c); err != nil {
			slog.Error("Failed to encode commands", "error", err)
			return
		}
		// Ensure the commands are sent before closing
		if f, ok := conn.(interface{ Flush() error }); ok {
			_ = f.Flush()
		}
	case common.SendResults:
		for _, r := range r.Results {
			slog.Info("Received result", "result", r)
		}
	default:
		slog.Error("Unknown request type", "type", r.Type)
	}
}
