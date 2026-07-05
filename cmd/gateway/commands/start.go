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
	cmd.Flags().String(flagLogFormat, defaultLogFormat, "log format (console for humans, json for log collectors)")

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
		[]host.Provider{shinzohubProvider},
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

// errInvalidLogFormat is returned when the log-format flag value is not recognized.
var errInvalidLogFormat = errors.New("invalid log format")

// newLogger builds a logger with the level and format taken from the log-level and log-format flags.
func (a *App) newLogger() (*zap.Logger, error) {
	level, err := zapcore.ParseLevel(a.v.GetString(flagLogLevel))
	if err != nil {
		return nil, fmt.Errorf("invalid log level %q: %w", a.v.GetString(flagLogLevel), err)
	}

	var cfg zap.Config
	switch format := a.v.GetString(flagLogFormat); format {
	case logFormatConsole:
		cfg = zap.NewDevelopmentConfig()
	case logFormatJSON:
		cfg = zap.NewProductionConfig()
		// keep every log entry; sampling would silently drop repeated messages
		cfg.Sampling = nil
		// use the field names and severity values recognized by Cloud Logging structured logs
		cfg.EncoderConfig.MessageKey = "message"
		cfg.EncoderConfig.TimeKey = "timestamp"
		cfg.EncoderConfig.EncodeTime = zapcore.RFC3339NanoTimeEncoder
		cfg.EncoderConfig.LevelKey = "severity"
		cfg.EncoderConfig.EncodeLevel = encodeSeverity
		cfg.EncoderConfig.EncodeDuration = zapcore.StringDurationEncoder
	default:
		return nil, fmt.Errorf("%w: %q (expected %s or %s)", errInvalidLogFormat, format, logFormatConsole, logFormatJSON)
	}
	cfg.Level = zap.NewAtomicLevelAt(level)
	return cfg.Build()
}

// encodeSeverity maps zap levels to Cloud Logging LogSeverity values.
func encodeSeverity(l zapcore.Level, enc zapcore.PrimitiveArrayEncoder) {
	switch {
	case l == zapcore.WarnLevel:
		enc.AppendString("WARNING")
	case l >= zapcore.DPanicLevel:
		enc.AppendString("CRITICAL")
	default:
		enc.AppendString(l.CapitalString())
	}
}
