package host

import (
	"bufio"
	"context"
	"os"

	"go.uber.org/zap"
)

type Provider interface {
	Start(ctx context.Context, updatesCH chan<- Event) error
	Close() error

	SetLogger(logger *zap.Logger)
}

type FileProvider struct {
	logger   *zap.Logger
	filename string
}

var _ Provider = &FileProvider{}

func NewFileProvider(filename string) *FileProvider {
	return &FileProvider{
		filename: filename,
	}
}

func (p *FileProvider) Start(ctx context.Context, updatesCH chan<- Event) error {
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

func (p *FileProvider) Close() error {
	return nil
}

func (p *FileProvider) SetLogger(logger *zap.Logger) {
	p.logger = logger.Named("file-provider")
}
