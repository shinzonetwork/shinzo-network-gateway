package host

import (
	"bufio"
	"context"
	"os"

	"go.uber.org/zap"
)

// Provider calls register and deregister callbacks to notify when hosts are registered/deregistered.
type Provider interface {
	Run(ctx context.Context, register func(Host), deregister func(Host)) error

	SetLogger(logger *zap.Logger)
}

// FileProvider reads hosts line-by-line from a file and calls register callback.
type FileProvider struct {
	filename string

	logger *zap.Logger
}

var _ Provider = &FileProvider{}

// NewFileProvider creates a FileProvider that reads hosts from the given file.
func NewFileProvider(filename string) *FileProvider {
	return &FileProvider{
		filename: filename,
	}
}

// Run reads the host file and sends a HostRegistered event for each line.
func (p *FileProvider) Run(ctx context.Context, register func(Host), deregister func(Host)) error {
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
		default:
			register(Host(host))
		}
	}

	return nil
}

// SetLogger sets the logger used by the provider.
func (p *FileProvider) SetLogger(logger *zap.Logger) {
	p.logger = logger.Named("file-provider")
}
