package commands

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	defaultTimeout             = 60 * time.Second
	defaultInterval            = 10 * time.Second
	defaultCollectionsInterval = 10 * time.Minute
	defaultListenAddr          = ":8080"
	defaultSampleSize          = 3

	flagListen  = "listen"
	flagSample  = "sample-size"
	flagTimeout = "timeout"
)

// App is the main application struct holding configuration state.
type App struct {
	cfgFile string
	v       *viper.Viper
}

// NewApp creates a new App instance.
func NewApp() *App {
	return &App{v: viper.New()}
}

// Execute runs the root command.
func Execute() {
	app := NewApp()
	rootCmd, err := app.newRootCmd()
	if err != nil {
		// TODO(tzdybal): log / panic
		os.Exit(1)
	}
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func (a *App) newRootCmd() (*cobra.Command, error) {
	cmd := &cobra.Command{
		Use:   "gateway",
		Short: "Shinzo Network Gateway",
		Long:  `Shinzo Network Gateway is an entry point to Shinzo Network.`,
	}

	cmd.PersistentFlags().StringVar(&a.cfgFile, "config", "", "config file (default is $HOME/.shinzo-network-gateway.yaml)")
	cmd.PersistentPreRunE = func(_ *cobra.Command, _ []string) error {
		return a.initConfig()
	}

	startCmd, err := a.newStartCmd()
	if err != nil {
		return nil, err
	}
	queryCmd, err := a.newQueryCommand()
	if err != nil {
		return nil, err
	}
	cmd.AddCommand(startCmd)
	cmd.AddCommand(queryCmd)

	return cmd, nil
}

func (a *App) initConfig() error {
	if a.cfgFile != "" {
		a.v.SetConfigFile(a.cfgFile)
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("error finding home directory: %w", err)
		}
		a.v.AddConfigPath(home)
		a.v.SetConfigType("yaml")
		a.v.SetConfigName(".shinzo-network-gateway")
	}

	a.v.AutomaticEnv()

	if err := a.v.ReadInConfig(); err == nil {
		fmt.Fprintln(os.Stderr, "Using config file:", a.v.ConfigFileUsed())
	}

	return nil
}
