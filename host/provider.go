package host

import (
	"bufio"
	"context"
	"os"

	"go.uber.org/zap"
)

// Provider supplies host registration events to the Registry.
type Provider interface {
	Start(ctx context.Context, updatesCH chan<- Event) error
	Close() error

	SetLogger(logger *zap.Logger)
}

// FileProvider reads hosts line-by-line from a file and emits HostRegistered events.
type FileProvider struct {
	logger   *zap.Logger
	filename string
}

var _ Provider = &FileProvider{}

// NewFileProvider creates a FileProvider that reads hosts from the given file.
func NewFileProvider(filename string) *FileProvider {
	return &FileProvider{
		filename: filename,
	}
}

// Start reads the host file and sends a HostRegistered event for each line.
func (p *FileProvider) Start(ctx context.Context, updatesCH chan<- Event) error {
	p.logger.Sugar().Debugw("opening host file", "path", p.filename)
	f, err := os.Open(p.filename)
	if err != nil {
		return err
	}
	defer func() {
		err := f.Close()
		if err != nil {
			p.logger.Sugar().Infow("failed to close file", "path", p.filename, "error", err)
		}
	}()
	sc := bufio.NewScanner(f)

	for sc.Scan() {
		host := sc.Text()
		p.logger.Sugar().Debugw("host found", "address", host)
		select {
		case <-ctx.Done():
			return ctx.Err()

		case updatesCH <- Event{
			Type: HostRegistered,
			Host: Host(host),
		}:
		}
	}

	return nil
}

// Close is a no-op for FileProvider.
func (p *FileProvider) Close() error {
	return nil
}

// SetLogger sets the logger used by the provider.
func (p *FileProvider) SetLogger(logger *zap.Logger) {
	p.logger = logger.Named("file-provider")
}
