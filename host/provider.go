package host

import (
	"bufio"
	"context"
	"errors"
	"net/url"
	"os"
	"slices"
	"time"

	shinzohub "github.com/shinzonetwork/shinzo-network-gateway/pkg/shinzohub-api"
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
func (p *FileProvider) Run(ctx context.Context, register func(Host), _ func(Host)) error {
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

// ShinzohubProvider reads host information from shinzohub using REST.
type ShinzohubProvider struct {
	baseURL string
	client  *shinzohub.Client

	hosts []Host

	logger *zap.Logger
}

var _ Provider = &ShinzohubProvider{}

// TODO(tzdybal): make this configurable.
const (
	refreshInterval = 10 * time.Second
	fetchTimeout    = 5 * time.Second
)

// NewShinzohubProvider creates a ShinzohubProvider that reads hosts from given shinzohub endpoint.
func NewShinzohubProvider(baseURL string) (*ShinzohubProvider, error) {
	// ParseRequestURI is more strict and rejects "", relative paths, etc
	if _, err := url.ParseRequestURI(baseURL); err != nil {
		return nil, err
	}
	return &ShinzohubProvider{
		baseURL: baseURL,
		client:  shinzohub.NewClient(baseURL),
	}, nil
}

// Run gets all hosts from shinzohub periodically and fires events when host set changes.
func (p *ShinzohubProvider) Run(ctx context.Context, register func(Host), deregister func(Host)) error {
	p.logger.Sugar().Infow("starting", "baseURL", p.baseURL)

	// update hosts immediately
	p.updateHosts(ctx, register, deregister)

	ticker := time.NewTicker(refreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.Canceled) {
				return nil
			}
			return ctx.Err()
		case <-ticker.C:
			p.updateHosts(ctx, register, deregister)
		}
	}
}

func (p *ShinzohubProvider) updateHosts(ctx context.Context, register func(Host), deregister func(Host)) {
	p.logger.Sugar().Debug("fetching hosts")
	newHosts, err := p.fetchHosts(ctx)
	if err != nil {
		p.logger.Sugar().Errorw("failed to fetch hosts from shinzohub", "error", err)
		return
	}
	p.logger.Sugar().Debugf("fetched %d hosts", len(newHosts))
	added, removed := getSliceDiffs(p.hosts, newHosts)
	p.logger.Sugar().Debugf("%d hosts to add, %d hosts to remove", len(added), len(removed))
	for _, a := range added {
		register(a)
	}
	for _, r := range removed {
		deregister(r)
	}
	p.hosts = newHosts
}

func (p *ShinzohubProvider) fetchHosts(ctx context.Context) ([]Host, error) {
	callCtx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	resp, err := p.client.GetAllHosts(callCtx)
	if err != nil {
		return nil, err
	}

	hosts := make([]Host, 0, len(resp))

	for _, h := range resp {
		hosts = append(hosts, Host(h.EndpointAddress))
	}

	slices.Sort(hosts)
	return hosts, nil
}

// SetLogger sets the logger used by the provider.
func (p *ShinzohubProvider) SetLogger(logger *zap.Logger) {
	p.logger = logger.Named("shinzohub-provider")
}
