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
	"github.com/anyproto/anytype-cli/core/localstate"
	"github.com/anyproto/anytype-cli/core/output"
)

// GetService creates a service instance with default configuration.
func GetService() (service.Service, error) {
	return GetServiceWithAddress("")
}

// GetServiceWithAddress creates a service instance with a custom API listen address.
func GetServiceWithAddress(apiAddr string) (service.Service, error) {
	return getServiceWithAddressAndLogs(apiAddr, config.GetLogsDir())
}

func getServiceWithAddressAndLogs(apiAddr, logDir string) (service.Service, error) {
	options := service.KeyValue{
		"UserService": true,
	}

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

	// OnStarted, if set, runs once the gRPC servers are listening, before
	// Start returns. It does not run when the server fails to start.
	OnStarted func()

	// startServer starts the gRPC servers; replaceable in tests.
	startServer func(grpcAddr, grpcWebAddr string) error
	// hasStoredAccount reports whether auto-login has a key to use;
	// replaceable in tests.
	hasStoredAccount func() bool
	// accountActivity and lockInstance allow isolated auto-login lifecycle tests.
	accountActivity  func() bool
	lockInstance     func() (func(), error)
	storedAccountKey func() (string, bool, error)
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
	if p.startServer == nil {
		p.startServer = p.server.Start
	}
	if p.hasStoredAccount == nil {
		p.hasStoredAccount = func() bool {
			key, _, err := core.GetStoredAccountKey()
			return err == nil && key != ""
		}
	}

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

	// The JSON API starts when an account logs in: show where it will be and,
	// if there is nothing to auto-login with, how to log in.
	output.Banner(startupBanner(config.APIURL(p.apiListenAddr), p.hasStoredAccount())...)

	if p.OnStarted != nil {
		p.OnStarted()
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

	if err := p.startServer(config.DefaultGRPCAddress, config.DefaultGRPCWebAddress); err != nil {
		p.startErr = err
		return
	}

	// Signal successful start
	p.startCh <- struct{}{}

	// Wait a moment for server to be ready; skip auto-login if stopped meanwhile
	select {
	case <-time.After(2 * time.Second):
	case <-p.ctx.Done():
		return
	}

	go p.attemptAutoLogin()

	<-p.ctx.Done()
}

func (p *Program) attemptAutoLogin() {
	lock := p.lockInstance
	if lock == nil {
		lock = func() (func(), error) { return localstate.Lock(localstate.Dir()) }
	}
	var unlock func()
	for {
		var err error
		unlock, err = lock()
		if err == nil {
			break
		}
		select {
		case <-p.ctx.Done():
			return
		case <-time.After(200 * time.Millisecond):
		}
	}
	defer unlock()
	active := p.accountActivity
	if active == nil {
		active = p.server.AccountActivity
	}
	// Check after taking the shared lock: provisioning may have completed while
	// this delayed auto-login was waiting for it.
	if active() {
		output.Info("Account operation already observed, skipping auto-login")
		return
	}

	readKey := p.storedAccountKey
	if readKey == nil {
		readKey = core.GetStoredAccountKey
	}
	accountKey, _, err := readKey()
	if err != nil || accountKey == "" {
		output.Info("No stored account key found, skipping auto-login")
		return
	}

	networkConfigPath, _ := config.GetNetworkConfigPathFromConfig()

	output.Info("Found stored account key, attempting auto-login...")

	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		if err := core.Authenticate(accountKey, "", p.apiListenAddr, networkConfigPath); err != nil {
			if i < maxRetries-1 {
				time.Sleep(2 * time.Second)
				continue
			}
			output.Info("Failed to auto-login with account key after %d attempts: %v", maxRetries, err)
			output.Banner(autoLoginFailedBanner()...)
		} else {
			output.Success("Successfully logged in using stored account key")
			return
		}
	}
}

// startupBanner is shown once the server is up. With a stored account key the
// server logs in by itself, so the address is enough; without one, the JSON
// API won't start until someone logs in.
func startupBanner(url string, hasStoredAccount bool) []string {
	if hasStoredAccount {
		return []string{"JSON API: " + url}
	}
	return []string{
		"JSON API: " + url + " (starts after login)",
		"",
		"Not logged in. In another terminal, run one of:",
		"  anytype auth login           # existing bot account",
		"  anytype auth create <name>   # new bot account",
	}
}

func autoLoginFailedBanner() []string {
	return []string{
		"Auto-login failed, so the JSON API is not running.",
		"In another terminal, run: anytype auth login",
	}
}
