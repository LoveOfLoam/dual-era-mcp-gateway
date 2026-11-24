package kube

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// Lease is the coordination.k8s.io/v1 Lease subset used for leader election.
type Lease struct {
	Metadata ObjectMeta `json:"metadata"`
	Spec     LeaseSpec  `json:"spec"`
}

// LeaseSpec is the leader-election payload.
type LeaseSpec struct {
	HolderIdentity       string `json:"holderIdentity"`
	LeaseDurationSeconds int    `json:"leaseDurationSeconds"`
	AcquireTime          string `json:"acquireTime,omitempty"`
	RenewTime            string `json:"renewTime,omitempty"`
}

// LeaderElector uses a Kubernetes Lease for single-writer leadership.
type LeaderElector struct {
	Client   *Client
	Name     string
	Identity string
	TTL      time.Duration
	Now      func() time.Time
}

// Run acquires leadership, runs fn until the lease is lost, and retries.
func (e *LeaderElector) Run(ctx context.Context, fn func(context.Context)) error {
	ttl := e.ttl()
	for {
		ok, err := e.acquire(ctx)
		if err == nil && ok {
			leaderCtx, cancel := context.WithCancel(ctx)
			done := make(chan struct{})
			go func() {
				defer close(done)
				fn(leaderCtx)
			}()
			ticker := time.NewTicker(ttl / 3)
			lost := false
			for !lost {
				select {
				case <-ctx.Done():
					cancel()
					ticker.Stop()
					<-done
					return ctx.Err()
				case <-ticker.C:
					ok, err := e.renew(ctx)
					if err != nil || !ok {
						lost = true
					}
				}
			}
			ticker.Stop()
			cancel()
			<-done
			continue
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(ttl / 3):
		}
	}
}

func (e *LeaderElector) ttl() time.Duration {
	if e.TTL <= 0 {
		return 15 * time.Second
	}
	return e.TTL
}

func (e *LeaderElector) now() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now()
}

func (e *LeaderElector) path() string {
	return fmt.Sprintf("%s/apis/coordination.k8s.io/v1/namespaces/%s/leases/%s",
		e.Client.BaseURL, e.Client.Namespace, pathEscape(e.Name))
}

func (e *LeaderElector) acquire(ctx context.Context) (bool, error) {
	lease, status, err := e.get(ctx)
	if err != nil {
		return false, err
	}
	now := e.now().UTC().Format(time.RFC3339)
	if status == http.StatusNotFound {
		l := Lease{Metadata: ObjectMeta{Name: e.Name, Namespace: e.Client.Namespace}, Spec: LeaseSpec{
			HolderIdentity: e.Identity, LeaseDurationSeconds: int(e.ttl().Seconds()),
			AcquireTime: now, RenewTime: now,
		}}
		return e.put(ctx, l, true)
	}
	if lease.Spec.HolderIdentity == e.Identity || e.expired(lease) {
		lease.Spec.HolderIdentity = e.Identity
		lease.Spec.LeaseDurationSeconds = int(e.ttl().Seconds())
		if lease.Spec.AcquireTime == "" || e.expired(lease) {
			lease.Spec.AcquireTime = now
		}
		lease.Spec.RenewTime = now
		return e.put(ctx, lease, false)
	}
	return false, nil
}

func (e *LeaderElector) renew(ctx context.Context) (bool, error) {
	lease, status, err := e.get(ctx)
	if err != nil {
		return false, err
	}
	if status == http.StatusNotFound || lease.Spec.HolderIdentity != e.Identity {
		return false, nil
	}
	lease.Spec.RenewTime = e.now().UTC().Format(time.RFC3339)
	return e.put(ctx, lease, false)
}

func (e *LeaderElector) expired(lease Lease) bool {
	t, err := time.Parse(time.RFC3339, lease.Spec.RenewTime)
	if err != nil {
		return true
	}
	return e.now().After(t.Add(time.Duration(lease.Spec.LeaseDurationSeconds) * time.Second))
}

func (e *LeaderElector) get(ctx context.Context) (Lease, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.path(), nil)
	if err != nil {
		return Lease{}, 0, err
	}
	e.Client.auth(req)
	resp, err := e.Client.client().Do(req)
	if err != nil {
		return Lease{}, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return Lease{}, resp.StatusCode, nil
	}
	if resp.StatusCode != http.StatusOK {
		return Lease{}, resp.StatusCode, fmt.Errorf("lease get: %d", resp.StatusCode)
	}
	var l Lease
	if err := json.NewDecoder(resp.Body).Decode(&l); err != nil {
		return Lease{}, resp.StatusCode, err
	}
	return l, resp.StatusCode, nil
}

func (e *LeaderElector) put(ctx context.Context, lease Lease, create bool) (bool, error) {
	raw, _ := json.Marshal(lease)
	method := http.MethodPut
	if create {
		method = http.MethodPost
	}
	req, err := http.NewRequestWithContext(ctx, method, e.path(), bytes.NewReader(raw))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/json")
	e.Client.auth(req)
	resp, err := e.Client.client().Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return true, nil
	}
	// 409 means someone else changed the lease first; another loop will retry.
	if resp.StatusCode == http.StatusConflict {
		return false, nil
	}
	return false, fmt.Errorf("lease put: %d", resp.StatusCode)
}

func pathEscape(s string) string { return url.PathEscape(s) }
