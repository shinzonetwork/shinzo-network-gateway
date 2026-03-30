package host

import (
	"bufio"
	"context"
	"os"
)

type Provider interface {
	Start(ctx context.Context, updatesCH chan<- Event) error
	Close() error
}

type FileProvider struct {
	filename string
}

var _ Provider = &FileProvider{}

func NewFileProvider(filename string) *FileProvider {
	return &FileProvider{
		filename: filename,
	}
}

func (d *FileProvider) Start(ctx context.Context, updatesCH chan<- Event) error {
	f, err := os.Open(d.filename)
	if err != nil {
		return err
	}
	defer func() {
		err := f.Close()
		if err != nil {
			// TODO(tzdybal): log error
		}
	}()
	sc := bufio.NewScanner(f)

	for sc.Scan() {
		updatesCH <- Event{
			Type: HostRegistered,
			Host: Host(sc.Text()),
		}
	}

	return nil
}
func (d *FileProvider) Close() error {
	return nil
}

type ShinzohubProvider struct {
}

var _ Provider = &ShinzohubProvider{}

func (d *ShinzohubProvider) Start(ctx context.Context, updatesCH chan<- Event) error {
	panic("not implemented") // TODO: Implement
}
func (d *ShinzohubProvider) Close() error {
	panic("not implemented") // TODO: Implement
}
