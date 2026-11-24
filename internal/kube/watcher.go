package kube

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"dual-era-mcp-gateway/internal/controlplane"
)

// Applier applies a compiled configuration.
type Applier interface {
	Apply(context.Context, controlplane.Config) error
}

// Watcher lists and watches CRDs and applies changed configs.
type Watcher struct {
	Client   *Client
	Applier  Applier
	Interval time.Duration
	lastHash string
}

// ReconcileOnce lists both CRDs, validates and applies if the content changed.
func (w *Watcher) ReconcileOnce(ctx context.Context) (string, string, error) {
	sources, sourceRV, err := w.Client.List(ctx, SourceRes)
	if err != nil {
		return "", "", err
	}
	consumers, consumerRV, err := w.Client.List(ctx, ConsumRes)
	if err != nil {
		return "", "", err
	}
	cfg, err := ConfigFromObjects(sources, consumers)
	if err != nil {
		return "", "", err
	}
	if err := controlplane.Validate(cfg); err != nil {
		return "", "", err
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256(raw)
	hash := hex.EncodeToString(sum[:])
	if hash != w.lastHash {
		if w.Applier == nil {
			return "", "", errNoApplier
		}
		if err := w.Applier.Apply(ctx, cfg); err != nil {
			return "", "", err
		}
		w.lastHash = hash
	}
	return sourceRV, consumerRV, nil
}

// Run lists/watches until ctx is canceled. Watch errors cause a bounded retry.
func (w *Watcher) Run(ctx context.Context) error {
	interval := w.Interval
	if interval <= 0 {
		interval = 2 * time.Second
	}
	for {
		sourceRV, consumerRV, err := w.ReconcileOnce(ctx)
		if err != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(interval):
				continue
			}
		}
		watchCtx, cancel := context.WithCancel(ctx)
		sourceCh, err1 := w.Client.Watch(watchCtx, SourceRes, sourceRV)
		consumerCh, err2 := w.Client.Watch(watchCtx, ConsumRes, consumerRV)
		if err1 != nil || err2 != nil {
			cancel()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(interval):
				continue
			}
		}
		done := false
		for !done {
			select {
			case <-ctx.Done():
				cancel()
				return ctx.Err()
			case _, ok := <-sourceCh:
				if !ok {
					done = true
				}
			case _, ok := <-consumerCh:
				if !ok {
					done = true
				}
			}
			if !done {
				if _, _, err := w.ReconcileOnce(ctx); err != nil {
					// Keep the last applied config and relist on the next loop.
					done = true
				}
			}
		}
		cancel()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
}

var errNoApplier = &noApplierError{}

type noApplierError struct{}

func (*noApplierError) Error() string { return "no applier configured" }
