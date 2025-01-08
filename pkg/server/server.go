package server

import (
	"bytes"
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

// In server:
func handleRequest(conn net.Conn) {
	defer func(conn net.Conn) {
		_ = conn.Close()
	}(conn)

	decoder := gob.NewDecoder(conn)
	encoder := gob.NewEncoder(conn)

	r := &common.Request{}
	if err := decoder.Decode(r); err != nil {
		slog.Error("Failed to decode request", "error", err)
		return
	}
	slog.Info("Received request", "type", r.Type)

	switch r.Type {
	case common.GetCommands:
		commands := []common.Command{
			common.ReadFile{Id: "command1", Path: "/tmp/bad"},
			common.WriteFile{Id: "command2", Path: "/tmp/bad", Content: "bad"},
			common.Execute{Id: "command3", Command: "ls -l /tmp"},
		}

		slog.Info("About to encode commands", "commands", commands)

		// Try encoding to a buffer first to verify the data
		var buf bytes.Buffer
		tmpEncoder := gob.NewEncoder(&buf)
		if err := tmpEncoder.Encode(commands); err != nil {
			slog.Error("Failed to encode to buffer", "error", err)
			return
		}

		slog.Info("Successfully encoded to buffer", "size", buf.Len())

		if err := encoder.Encode(commands); err != nil {
			slog.Error("Failed to encode commands", "error", err)
			return
		}

		slog.Info("Successfully encoded to connection")
		// Ensure all data is written before closing
		if conn, ok := conn.(*net.TCPConn); ok {
			conn.CloseWrite()
		}

	case common.SendResults:
		for _, r := range r.Results {
			slog.Info("Received result", "result", r)
		}

	default:
		slog.Error("Unknown request type", "type", r.Type)
	}
}
