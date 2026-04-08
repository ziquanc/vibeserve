package router

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// Server wraps an http.Server with a simple lifecycle API.
type Server struct {
	httpServer *http.Server
}

// NewServer creates a Server that will listen on host:port using handler.
// ReadTimeout and WriteTimeout are both set to 10 seconds.
func NewServer(host string, port int, handler http.Handler) *Server {
	return &Server{
		httpServer: &http.Server{
			Addr:         fmt.Sprintf("%s:%d", host, port),
			Handler:      handler,
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
		},
	}
}

// Start calls ListenAndServe on the underlying http.Server.
// It blocks until the server is stopped and returns any error other than
// http.ErrServerClosed (which is returned by a graceful Shutdown).
func (s *Server) Start() error {
	err := s.httpServer.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Shutdown gracefully stops the server using the provided context.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

// Addr returns the address the server is configured to listen on.
func (s *Server) Addr() string {
	return s.httpServer.Addr
}
