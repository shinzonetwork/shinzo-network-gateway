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
	"go.uber.org/zap/zapcore"
	"golang.org/x/sync/errgroup"

	"github.com/shinzonetwork/shinzo-network-gateway/endpoint"
	"github.com/shinzonetwork/shinzo-network-gateway/host"
	"github.com/shinzonetwork/shinzo-network-gateway/router"
)

const shutdownTimeout = 30 * time.Second

// TODO(tzdybal): add configuration option.
const (
	maxLimit               = 100_000
	defaultRefreshInterval = 30 * time.Second
)

func (a *App) newStartCmd() (*cobra.Command, error) {
	cmd := &cobra.Command{
		Use:   "start",
		Short: "starts the Shinzo Network Gateway",
		RunE:  a.startGateway,
	}
	cmd.Flags().String(flagListen, defaultListenAddr, "HTTP listen address for GraphQL endpoint")
	cmd.Flags().Int(flagSample, defaultSampleSize, "number of hosts for query fan out")
	cmd.Flags().String(flagShinzohubURL, "", "Base URL of the Shinzohub API to fetch information from")
	cmd.Flags().String(flagLogLevel, defaultLogLevel, "log level (debug, info, warn, error)")

	err := a.v.BindPFlags(cmd.Flags())
	if err != nil {
		return nil, err
	}

	return cmd, nil
}

func (a *App) startGateway(cmd *cobra.Command, _ []string) error {
	logger, err := a.newLogger()
	if err != nil {
		return fmt.Errorf("creating logger: %w", err)
	}
	defer func() {
		_ = logger.Sync()
	}()

	logger.Info("starting Shinzo Network Gateway",
		zap.String("listen", a.v.GetString(flagListen)),
		zap.Int("sampleSize", a.v.GetInt(flagSample)),
		zap.String("shinzohubURL", a.v.GetString(flagShinzohubURL)),
	)

	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// TODO(tzdybal): config/env/flag for host file
	fileProvider := host.NewFileProvider("hosts.txt", logger)

	shinzoURL := a.v.GetString(flagShinzohubURL)
	shinzohubProvider, err := host.NewShinzohubProvider(shinzoURL, logger)
	if err != nil {
		return fmt.Errorf("creating Shinzohub provider: %w", err)
	}

	connChecker := host.NewHTTPConnectionChecker(defaultTimeout, logger)
	collFetcher, err := host.NewShinzohubCollectionsFetcher(shinzoURL, defaultRefreshInterval, logger)
	if err != nil {
		return fmt.Errorf("creating Shinzohub collections fetcher: %w", err)
	}

	rtr := router.New(logger)
	registry := host.NewRegistry(
		host.Config{
			ConnCheckInterval:          defaultInterval,
			CollectionsRefreshInterval: defaultCollectionsInterval,
		},
		[]host.Provider{shinzohubProvider, fileProvider},
		[]host.Observer{rtr},
		connChecker,
		collFetcher,
		logger,
	)

	validators := []endpoint.Validator{endpoint.NewLimitValidator(maxLimit), &endpoint.OrderValidator{}}
	sampleSize := a.v.GetInt(flagSample)
	handler := endpoint.NewHandler(validators, &endpoint.DefaultCollectionExtractor{}, rtr, sampleSize, logger)
	endp, err := endpoint.New(a.v.GetString(flagListen), handler, logger)
	if err != nil {
		return fmt.Errorf("creating endpoint: %w", err)
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

// newLogger builds a console logger with the level taken from the log-level flag.
func (a *App) newLogger() (*zap.Logger, error) {
	level, err := zapcore.ParseLevel(a.v.GetString(flagLogLevel))
	if err != nil {
		return nil, fmt.Errorf("invalid log level %q: %w", a.v.GetString(flagLogLevel), err)
	}

	cfg := zap.NewDevelopmentConfig()
	cfg.Level = zap.NewAtomicLevelAt(level)
	return cfg.Build()
}
