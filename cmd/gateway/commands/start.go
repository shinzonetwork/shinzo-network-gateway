package commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	"github.com/shinzonetwork/shinzo-network-gateway/endpoint"
	"github.com/shinzonetwork/shinzo-network-gateway/host"
	"github.com/shinzonetwork/shinzo-network-gateway/router"
)

const shutdownTimeout = 30 * time.Second

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

	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// TODO(tzdybal): config/env/flag for host file
	provider := host.NewFileProvider("hosts.txt")
	provider.SetLogger(logger)
	connChecker := host.NewHTTPConnectionChecker(defaultTimeout, logger)
	collFetcher := host.NewHTTPCollectionsFetcher(defaultTimeout, logger)

	rtr := router.New(logger)
	registry := host.NewRegistry(
		host.Config{
			ConnCheckInterval:          defaultInterval,
			CollectionsRefreshInterval: defaultCollectionsInterval,
		},
		[]host.Provider{provider},
		[]host.Observer{rtr},
		connChecker,
		collFetcher,
		logger,
	)

	handler := endpoint.NewHandler(&endpoint.DefaultCollectionExtractor{}, rtr, logger)
	endp, err := endpoint.New(a.v.GetString(flagListen), handler, logger)
	if err != nil {
		return fmt.Errorf("error while creating endpoint: %w", err)
	}

	grp, ctx := errgroup.WithContext(ctx)
	grp.Go(func() error {
		return registry.Run(ctx)
	})
	grp.Go(func() error {
		return endp.ListenAndServe()
	})
	grp.Go(func() error {
		<-ctx.Done()
		// parent ctx is already cancelled here; derive a fresh deadline for graceful shutdown
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
		defer cancel()
		return endp.Close(ctx)
	})

	if err := grp.Wait(); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}
