package otohttp

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/pkg/errors"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Server handles oto requests.
type Server struct {
	routes map[string]http.Handler
	// NotFound is the http.Handler to use when a resource is
	// not found.
	NotFound http.Handler
	// OnErr is called when there is an error.
	OnErr func(w http.ResponseWriter, r *http.Request, err error)
}

// NewServer makes a new Server.
func NewServer() *Server {
	return &Server{
		routes: make(map[string]http.Handler),
		OnErr: func(w http.ResponseWriter, r *http.Request, err error) {
			errObj := struct {
				Error string `json:"error"`
			}{
				Error: err.Error(),
			}
			if err := Encode(w, r, http.StatusInternalServerError, errObj); err != nil {
				log.Printf("failed to encode error: %s\n", err)
			}
		},
		NotFound: http.NotFoundHandler(),
	}
}

// Register adds a handler for the specified service method.
func (s *Server) Register(service, method string, h http.HandlerFunc) {
	s.routes[fmt.Sprintf("/oto/%s.%s", service, method)] = h
}

// ServeHTTP serves the request.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.NotFound.ServeHTTP(w, r)
		return
	}
	h, ok := s.routes[r.URL.Path]
	if !ok {
		s.NotFound.ServeHTTP(w, r)
		return
	}
	h.ServeHTTP(w, r)
}

func (s *Server) ListenMetrics(port int) <-chan error {
	done := make(chan error, 1)
	go func() {
		http.Handle("/metrics", promhttp.Handler())
		done <- http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
		close(done)
	}()
	return done
}

// Encode writes the response.
func Encode(w http.ResponseWriter, r *http.Request, status int, v interface{}) error {
	b, err := json.Marshal(v)
	if err != nil {
		return errors.Wrap(err, "encode json")
	}
	var out io.Writer = w
	if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		w.Header().Set("Content-Encoding", "gzip")
		gzw := gzip.NewWriter(w)
		out = gzw
		defer gzw.Close()
	}
	w.Header().Set("Content-Type", "application/json; chatset=utf-8")
	w.WriteHeader(status)
	if _, err := out.Write(b); err != nil {
		return err
	}
	return nil
}

// Decode unmarshals the object in the request into v.
func Decode(r *http.Request, v interface{}) error {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		return errors.Wrap(err, "decode json")
	}
	return nil
}
