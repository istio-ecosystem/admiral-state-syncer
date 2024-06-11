package server

import (
	"fmt"
	"github.com/istio-ecosystem/admiral-state-syncer/pkg/monitoring"
	"go.opentelemetry.io/otel/attribute"
	api "go.opentelemetry.io/otel/metric"
	"log"
	"net/http"
)

const (
	livenessPath  = "/liveness"
	readinessPath = "/readiness"
)

var (
	syncerServerMeter               = monitoring.NewMeter("admiral_state_syncer_server")
	admiralStateSyncerRequestsTotal = monitoring.NewCounter(
		"requests_total",
		"total number of requests handled by admiral state syncer server",
		monitoring.WithMeter(syncerServerMeter))
)

type server struct {
	mux     *http.ServeMux
	options *options
}

type options struct {
}

func createOptions(opts ...Options) *options {
	o := &options{}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// Options accepts a pointer to options. It is used
// to update the options by calling an array of functions
type Options func(*options)

func NewServer(opts ...Options) (*server, error) {
	httpServer := &server{
		options: createOptions(opts...),
		mux:     http.NewServeMux(),
	}
	httpServer.mux.HandleFunc(livenessPath, httpServer.livenessHandler)
	httpServer.mux.HandleFunc(readinessPath, httpServer.readinessHandler)
	return httpServer, nil
}

func (s *server) Listen(port string) error {
	return http.ListenAndServe(":"+port, s.mux)
}

func (s *server) livenessHandler(responseWriter http.ResponseWriter, request *http.Request) {
	responseWriter.WriteHeader(200)
	_, err := responseWriter.Write([]byte(fmt.Sprintf("OK\n")))
	if err != nil {
		admiralStateSyncerRequestsTotal.Increment(api.WithAttributes(
			attribute.Key("path").String(livenessPath),
			attribute.Key("code").String("503"),
		))
		log.Fatalf("failed to write response")
	}
	admiralStateSyncerRequestsTotal.Increment(api.WithAttributes(
		attribute.Key("path").String(livenessPath),
		attribute.Key("code").String("200"),
	))
}

func (s *server) readinessHandler(responseWriter http.ResponseWriter, request *http.Request) {
	admiralStateSyncerRequestsTotal.Increment(api.WithAttributes(
		attribute.Key("path").String(readinessPath),
	))
	responseWriter.WriteHeader(200)
	_, err := responseWriter.Write([]byte(fmt.Sprintf("OK\n")))
	if err != nil {
		admiralStateSyncerRequestsTotal.Increment(api.WithAttributes(
			attribute.Key("path").String(readinessPath),
			attribute.Key("code").String("503"),
		))
		log.Fatalf("failed to write response")
	}
	admiralStateSyncerRequestsTotal.Increment(api.WithAttributes(
		attribute.Key("path").String(readinessPath),
		attribute.Key("code").String("200"),
	))
}
