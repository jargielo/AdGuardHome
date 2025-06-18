package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/AdguardTeam/AdGuardHome/internal/next/configmgr"
	"github.com/AdguardTeam/golibs/errors"
	"github.com/AdguardTeam/golibs/service"
)

// serviceMgr manages AdGuard Home services.
type serviceMgr struct {
	confMgr *configmgr.Manager
	// confMgrMu protects confMgr.
	confMgrMu *sync.RWMutex

	confMgrConf *configmgr.Config
	logger      *slog.Logger
}

// serviceMgrConfig contains service manager configuration parameters.
type serviceMgrConfig struct {
	// ConfMgrConf is the configuration manager config, it must not be nil.
	ConfMgrConf *configmgr.Config

	// Logger is the logger used to log services activity, it must not be nil.
	Logger *slog.Logger
}

// newServiceMgr creates a new *serviceMgr.
func newServiceMgr(ctx context.Context, conf *serviceMgrConfig) (s *serviceMgr, err error) {
	confMgr, err := configmgr.New(ctx, conf.ConfMgrConf)
	if err != nil {
		return nil, fmt.Errorf("creating config manager: %w", err)
	}

	return &serviceMgr{
		confMgr:     confMgr,
		confMgrMu:   &sync.RWMutex{},
		confMgrConf: conf.ConfMgrConf,
		logger:      conf.Logger,
	}, nil
}

// type check
var _ service.Interface = (*serviceMgr)(nil)

// Start implements the [service.Interface] interface for *serviceMgr.
func (s *serviceMgr) Start(ctx context.Context) (err error) {
	var errs []error

	err = s.confMgr.Web().Start(ctx)
	if err != nil {
		errs = append(errs, fmt.Errorf("starting web: %w", err))
	}

	err = s.confMgr.DNS().Start(ctx)
	if err != nil {
		errs = append(errs, fmt.Errorf("starting dnssvc: %w", err))
	}

	return errors.Join(errs...)
}

// Shutdown implements the [service.Interface] interface for *serviceMgr.
func (s *serviceMgr) Shutdown(ctx context.Context) (err error) {
	var errs []error

	err = s.confMgr.Web().Shutdown(ctx)
	if err != nil {
		errs = append(errs, fmt.Errorf("shutting down web: %w", err))
	}

	err = s.confMgr.DNS().Shutdown(ctx)
	if err != nil {
		errs = append(errs, fmt.Errorf("shutting down dnssvc: %w", err))
	}

	return errors.Join(errs...)
}

// type check
var _ service.Refresher = (*serviceMgr)(nil)

// Refresh implements the [service.Refresher] interface for *serviceMgr.
func (s *serviceMgr) Refresh(ctx context.Context) (err error) {
	s.logger.InfoContext(ctx, "reconfiguring started")

	err = s.Shutdown(ctx)
	if err != nil {
		return fmt.Errorf("shutdown failed: %w", err)
	}

	// TODO(a.garipov):  This is a very rough way to do it.  Some services can
	// be reconfigured without the full shutdown, and the error handling is
	// currently not the best.

	ctx, cancel := context.WithTimeout(ctx, defaultTimeoutStart)
	defer cancel()

	err = s.updConfMgr(ctx)
	if err != nil {
		return fmt.Errorf("updating configuration manager: %w", err)
	}

	err = s.Start(ctx)
	if err != nil {
		return fmt.Errorf("restarting services: %w", err)
	}

	s.logger.InfoContext(ctx, "reconfiguring finished")

	return nil
}

// updConfMgr updates the configuration manager.
func (s *serviceMgr) updConfMgr(ctx context.Context) (err error) {
	confMgr, err := configmgr.New(ctx, s.confMgrConf)
	if err != nil {
		return fmt.Errorf("creating config manager: %w", err)
	}

	s.confMgrMu.Lock()
	defer s.confMgrMu.Unlock()

	s.confMgr = confMgr

	return nil
}
