package commands

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/shinzonetwork/shinzo-network-gateway/host"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var startCommand = &cobra.Command{
	Use:   "start",
	Short: "starts the Shinzo Network Gateway",
	RunE:  startGateway,
}

func startGateway(cmd *cobra.Command, args []string) error {
	logger, err := zap.NewDevelopment()
	defer func() {
		_ = logger.Sync()
	}()
	if err != nil {
		return fmt.Errorf("error while creating logger: %w", err)
	}

	logger.Sugar().Info("Starting Shinzo Network Gateway")

	// TODO(tzdybal): config/env/flag for host file
	provider := host.NewFileProvider("hosts.txt")
	provider.SetLogger(logger)
	connChecker := host.NewHTTPConnectionChecker(5*time.Second, logger)
	registry := host.NewRegistry(host.Config{ConnCheckInterval: 5 * time.Second}, []host.Provider{provider}, connChecker, logger)

	err = registry.Start(cmd.Context())
	if err != nil {
		return fmt.Errorf("error while starting host registry: %w", err)
	}

	waitCh := make(chan error, 1)
	go func() { waitCh <- registry.Wait() }()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	errCh := make(chan error, 1)
	go func() {
		select {
		case <-c:
			logger.Sugar().Info("received SIGTERM, shutting down")
			err := registry.Close()
			if err != nil {
				errCh <- err
			}
			errCh <- registry.Wait()
		case err := <-waitCh:
			logger.Sugar().Infow("registry exited", "error", err)
			errCh <- err
		}
	}()

	return <-errCh
}
