package kube

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

type leaseStore struct {
	mu    sync.Mutex
	lease *Lease
}

func (s *leaseStore) handler(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch r.Method {
	case http.MethodGet:
		if s.lease == nil {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(s.lease)
	case http.MethodPost, http.MethodPut:
		var l Lease
		if err := json.NewDecoder(r.Body).Decode(&l); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if s.lease != nil {
			l.Metadata.ResourceVersion = "2"
		} else {
			l.Metadata.ResourceVersion = "1"
		}
		s.lease = &l
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(l)
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

func TestLeaderElector(t *testing.T) {
	store := &leaseStore{}
	srv := httptest.NewServer(http.HandlerFunc(store.handler))
	defer srv.Close()
	client := &Client{BaseURL: srv.URL, Namespace: "default"}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	a := &LeaderElector{Client: client, Name: "gateway", Identity: "a", TTL: 9 * time.Second, Now: func() time.Time { return now }}
	b := &LeaderElector{Client: client, Name: "gateway", Identity: "b", TTL: 9 * time.Second, Now: func() time.Time { return now }}

	ok, err := a.acquire(context.Background())
	if err != nil || !ok {
		t.Fatalf("a acquire = %v %v", ok, err)
	}
	ok, err = b.acquire(context.Background())
	if err != nil || ok {
		t.Fatalf("b must not acquire: %v %v", ok, err)
	}
	ok, err = a.renew(context.Background())
	if err != nil || !ok {
		t.Fatalf("a renew = %v %v", ok, err)
	}
	now = now.Add(20 * time.Second)
	ok, err = b.acquire(context.Background())
	if err != nil || !ok {
		t.Fatalf("b should acquire expired lease: %v %v", ok, err)
	}
}
