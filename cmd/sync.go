/*
Copyright © 2024 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"log"
	"net/http"
	"sync"

	"github.com/istio-ecosystem/admiral-state-syncer/pkg/bootstrap"
	"github.com/istio-ecosystem/admiral-state-syncer/pkg/monitoring"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/istio-ecosystem/admiral-state-syncer/pkg/server"

	"github.com/spf13/cobra"
)

const (
	portNumber  = "8080"
	metricsPort = "9090"
	metricsPath = "/metrics"
)

// syncCmd represents the sync command
var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Syncs state from kubernetes API Server to a registry platform",
	Long:  `Syncs state from kubernetes API Server to a registry platform`,
	Run: func(cmd *cobra.Command, args []string) {
		wg := new(sync.WaitGroup)
		wg.Add(1)
		go func() {
			Initialize(
				initializeMonitoring,
				startMetricsServer,
				startStateSyncer,
				startNewServer,
			)
			wg.Done()
		}()
		wg.Wait()
		log.Println("state syncer has been initialized")
	},
}

func init() {
	rootCmd.AddCommand(syncCmd)
}

func startMetricsServer() {
	http.Handle(metricsPath, promhttp.Handler())
	err := http.ListenAndServe(":"+metricsPort, nil)
	if err != nil {
		log.Fatalf("error serving http: %v", err)
	}
}

func startStateSyncer() {
	stateSyncer, err := bootstrap.NewStateSyncer()
	if err != nil {
		log.Fatalf("failed to instantiate state syncer: %v", err)
	}
	err = stateSyncer.Start()
	if err != nil {
		log.Fatalf("error starting state syncer: %v", err)
	}
}

func startNewServer() {
	newServer, err := server.NewServer()
	if err != nil {
		log.Fatalf("failed to instantiate a server: %v", err)
	}
	err = newServer.Listen(portNumber)
	if err != nil {
		log.Fatalf("failed to start server: %v", err)
	}
	log.Printf("started server on port %s", portNumber)
}

func initializeMonitoring() {
	err := monitoring.InitializeMonitoring()
	if err != nil {
		log.Fatalf("failed to initialize monitoring: %v", err)
	}
}

func Initialize(funcs ...func()) {
	wg := new(sync.WaitGroup)
	wg.Add(len(funcs))
	for _, fn := range funcs {
		go func() {
			fn()
			wg.Done()
		}()
	}
	wg.Wait()
}
