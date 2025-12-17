package serviceprogram

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/kardianos/service"

	"github.com/anyproto/anytype-cli/core"
	"github.com/anyproto/anytype-cli/core/config"
	"github.com/anyproto/anytype-cli/core/grpcserver"
	"github.com/anyproto/anytype-cli/core/output"
)

// GetService creates a service instance with default configuration.
func GetService() (service.Service, error) {
	return GetServiceWithAddress("")
}

// GetServiceWithAddress creates a service instance with a custom API listen address.
func GetServiceWithAddress(apiAddr string) (service.Service, error) {
	options := service.KeyValue{
		"UserService": true,
	}

	logDir := config.GetLogsDir()
	if logDir != "" {
		if err := os.MkdirAll(logDir, 0755); err == nil {
			options["LogDirectory"] = logDir
		}
	}

	effectiveAddr := apiAddr
	if effectiveAddr == "" {
		effectiveAddr = config.DefaultAPIAddress
	}

	args := []string{"serve"}
	if effectiveAddr != config.DefaultAPIAddress {
		args = append(args, "--listen-address", effectiveAddr)
	}

	svcConfig := &service.Config{
		Name:        "anytype",
		DisplayName: "Anytype",
		Description: "Anytype",
		Arguments:   args,
		Option:      options,
	}

	prg := New(effectiveAddr)
	return service.New(prg, svcConfig)
}

type Program struct {
	server        *grpcserver.Server
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
	startErr      error
	startCh       chan struct{}
	apiListenAddr string
}

func New(apiListenAddr string) *Program {
	return &Program{
		startCh:       make(chan struct{}),
		apiListenAddr: apiListenAddr,
	}
}

func (p *Program) Start(s service.Service) error {
	p.ctx, p.cancel = context.WithCancel(context.Background())
	p.server = grpcserver.NewServer()

	p.wg.Add(1)
	go p.run()

	// Wait for server to start or fail
	select {
	case <-p.startCh:
		if p.startErr != nil {
			p.cancel()
			p.wg.Wait()
			return p.startErr
		}
	case <-time.After(5 * time.Second):
		p.cancel()
		p.wg.Wait()
		return fmt.Errorf("timeout waiting for server to start")
	}

	return nil
}

func (p *Program) Stop(s service.Service) error {
	if p.cancel != nil {
		p.cancel()
	}

	if p.server != nil {
		if err := p.server.Stop(); err != nil {
			output.Info("Error stopping server: %v", err)
		}
	}

	p.wg.Wait()
	return nil
}

func (p *Program) run() {
	defer p.wg.Done()
	defer close(p.startCh)

	if err := p.server.Start(config.DefaultGRPCAddress, config.DefaultGRPCWebAddress); err != nil {
		p.startErr = err
		return
	}

	// Signal successful start
	p.startCh <- struct{}{}

	// Wait a moment for server to be ready
	time.Sleep(2 * time.Second)

	go p.attemptAutoLogin()

	<-p.ctx.Done()
}

func (p *Program) attemptAutoLogin() {
	accountKey, _, err := core.GetStoredAccountKey()
	if err != nil || accountKey == "" {
		output.Info("No stored account key found, skipping auto-login")
		return
	}

	output.Info("Found stored account key, attempting auto-login...")

	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		if err := core.Authenticate(accountKey, "", p.apiListenAddr); err != nil {
			if i < maxRetries-1 {
				time.Sleep(2 * time.Second)
				continue
			}
			output.Info("Failed to auto-login with account key after %d attempts: %v", maxRetries, err)
		} else {
			output.Success("Successfully logged in using stored account key")
			return
		}
	}
}
