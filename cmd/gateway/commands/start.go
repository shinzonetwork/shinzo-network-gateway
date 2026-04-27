package commands

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/shinzonetwork/shinzo-network-gateway/endpoint"
	"github.com/shinzonetwork/shinzo-network-gateway/host"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

func (a *App) newStartCmd() (*cobra.Command, error) {
	cmd := &cobra.Command{
		Use:   "start",
		Short: "starts the Shinzo Network Gateway",
		RunE:  a.startGateway,
	}
	cmd.Flags().String(flagListen, defaultListenAddr, "HTTP listen address for GraphQL endpoint")
	cmd.Flags().Int(flagSample, defaultSampleSize, "number of hosts for query fan out")

	err := a.v.BindPFlags(cmd.Flags())
	if err != nil {
		return nil, err
	}

	return cmd, nil
}

func (a *App) startGateway(cmd *cobra.Command, _ []string) error {
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
	connChecker := host.NewHTTPConnectionChecker(defaultTimeout, logger)
	registry := host.NewRegistry(host.Config{ConnCheckInterval: defaultInterval}, []host.Provider{provider}, connChecker, logger)

	err = registry.Start(cmd.Context())
	if err != nil {
		return fmt.Errorf("error while starting host registry: %w", err)
	}

	handler := endpoint.NewHandler(&endpoint.DefaultCollectionExtractor{}, nil, logger)
	endp, err := endpoint.New(a.v.GetString(flagListen), handler, logger)
	if err != nil {
		return fmt.Errorf("error while creating endpoint: %w", err)
	}

	endpErrCh := make(chan error, 1)
	go func() {
		endpErrCh <- endp.ListenAndServe()
	}()

	regErrCh := make(chan error, 1)
	go func() { regErrCh <- registry.Wait() }()

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
		case err := <-regErrCh:
			logger.Sugar().Infow("registry exited", "error", err)
			errCh <- err
		case err := <-endpErrCh:
			logger.Sugar().Infow("ednpoint exited", "error", err)
			errCh <- err
		}
	}()

	return <-errCh
}
