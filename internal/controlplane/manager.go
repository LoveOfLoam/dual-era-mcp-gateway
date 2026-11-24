// Package controlplane validates source/policy configuration and keeps a
// versioned snapshot history for rollback.
package controlplane

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"dual-era-mcp-gateway/internal/policy"
	"dual-era-mcp-gateway/internal/source"
)

// Config is the desired gateway configuration.
type Config struct {
	Sources    []source.SourceSpec `json:"sources" yaml:"sources"`
	Consumers  []policy.Consumer   `json:"consumers" yaml:"consumers"`
	RateLimits []policy.RateLimit  `json:"rateLimits" yaml:"rateLimits"`
}

// Snapshot is an immutable applied configuration.
type Snapshot struct {
	Version   string              `json:"version"`
	Config    Config              `json:"config"`
	CreatedAt time.Time           `json:"createdAt"`
	Sources   []source.SourceSpec `json:"-"`
	Consumers []policy.Consumer   `json:"-"`
	Limits    []policy.RateLimit  `json:"-"`
}

// AuditEntry records who changed the config and which versions were involved.
type AuditEntry struct {
	Action          string    `json:"action"`
	PreviousVersion string    `json:"previousVersion,omitempty"`
	Version         string    `json:"version"`
	Actor           string    `json:"actor,omitempty"`
	Reason          string    `json:"reason,omitempty"`
	At              time.Time `json:"at"`
}

// Manager holds the current snapshot and bounded history.
type Manager struct {
	mu        sync.RWMutex
	current   Snapshot
	history   []Snapshot
	audit     []AuditEntry
	max       int
	maxAudit  int
	applies   uint64
	rollbacks uint64
}

// New validates cfg and creates a manager.
func New(cfg Config) (*Manager, error) {
	snap, err := build(cfg)
	if err != nil {
		return nil, err
	}
	return &Manager{
		current:  snap,
		history:  []Snapshot{snap},
		max:      20,
		maxAudit: 100,
		audit:    []AuditEntry{{Action: "init", Version: snap.Version, At: snap.CreatedAt}},
	}, nil
}

// Current returns the current snapshot.
func (m *Manager) Current() Snapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current
}

// History returns a copy of the applied snapshots, oldest first.
func (m *Manager) History() []Snapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Snapshot, len(m.history))
	copy(out, m.history)
	return out
}

// Apply validates and applies a new config. Invalid config leaves the current
// snapshot untouched.
func (m *Manager) Apply(cfg Config) (Snapshot, error) {
	return m.ApplyWithMeta(cfg, "", "")
}

// ApplyWithMeta applies a config and records actor/reason in the audit log.
func (m *Manager) ApplyWithMeta(cfg Config, actor, reason string) (Snapshot, error) {
	snap, err := build(cfg)
	if err != nil {
		return Snapshot{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if snap.Version == m.current.Version {
		return m.current, nil
	}
	previous := m.current.Version
	m.current = snap
	m.history = append(m.history, snap)
	if len(m.history) > m.max {
		m.history = m.history[len(m.history)-m.max:]
	}
	m.applies++
	m.appendAuditLocked(AuditEntry{Action: "apply", PreviousVersion: previous, Version: snap.Version, Actor: actor, Reason: reason, At: snap.CreatedAt})
	return snap, nil
}

// Rollback restores a historical snapshot by version.
func (m *Manager) Rollback(version string) (Snapshot, error) {
	return m.RollbackWithMeta(version, "", "")
}

// RollbackWithMeta restores a version and records actor/reason.
func (m *Manager) RollbackWithMeta(version, actor, reason string) (Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var found *Snapshot
	for i := range m.history {
		if m.history[i].Version == version {
			found = &m.history[i]
			break
		}
	}
	if found == nil {
		return Snapshot{}, fmt.Errorf("version %q not found", version)
	}
	previous := m.current.Version
	restored := *found
	restored.CreatedAt = time.Now().UTC()
	m.current = restored
	m.history = append(m.history, restored)
	if len(m.history) > m.max {
		m.history = m.history[len(m.history)-m.max:]
	}
	m.rollbacks++
	m.appendAuditLocked(AuditEntry{Action: "rollback", PreviousVersion: previous, Version: restored.Version, Actor: actor, Reason: reason, At: restored.CreatedAt})
	return restored, nil
}

// Audit returns a copy of the bounded audit log, oldest first.
func (m *Manager) Audit() []AuditEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]AuditEntry, len(m.audit))
	copy(out, m.audit)
	return out
}

// Counts returns apply/rollback counters.
func (m *Manager) Counts() (applies, rollbacks uint64) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.applies, m.rollbacks
}

func (m *Manager) appendAuditLocked(e AuditEntry) {
	m.audit = append(m.audit, e)
	if len(m.audit) > m.maxAudit {
		m.audit = m.audit[len(m.audit)-m.maxAudit:]
	}
}

func build(cfg Config) (Snapshot, error) {
	if err := Validate(cfg); err != nil {
		return Snapshot{}, err
	}
	normalized, err := normalize(cfg)
	if err != nil {
		return Snapshot{}, err
	}
	sum := sha256.Sum256(normalized)
	return Snapshot{
		Version:   "sha256:" + hex.EncodeToString(sum[:]),
		Config:    cfg,
		CreatedAt: time.Now().UTC(),
		Sources:   cfg.Sources,
		Consumers: cfg.Consumers,
		Limits:    cfg.RateLimits,
	}, nil
}

func normalize(cfg Config) ([]byte, error) {
	// encoding/json orders struct fields deterministically. Slices retain input
	// order; callers should provide stable order.
	return json.Marshal(struct {
		Sources    []source.SourceSpec `json:"sources"`
		Consumers  []policy.Consumer   `json:"consumers"`
		RateLimits []policy.RateLimit  `json:"rateLimits"`
	}{cfg.Sources, cfg.Consumers, cfg.RateLimits})
}

// Validate checks source specs, policy keys, and cross references.
func Validate(cfg Config) error {
	if err := source.Validate(cfg.Sources); err != nil {
		return err
	}
	if _, err := policy.New(policy.Config{Consumers: cfg.Consumers, RateLimits: cfg.RateLimits}); err != nil {
		return err
	}
	bySource := map[string]map[string]bool{}
	for _, s := range cfg.Sources {
		tools := map[string]bool{}
		for _, t := range s.Tools {
			tools[t.Name] = true
		}
		bySource[s.Name] = tools
	}
	for _, rl := range cfg.RateLimits {
		if rl.Source != "" {
			tools, ok := bySource[rl.Source]
			if !ok {
				return fmt.Errorf("rate limit references unknown source %q", rl.Source)
			}
			if rl.Tool != "" && len(tools) > 0 && !tools[rl.Tool] {
				return fmt.Errorf("rate limit references unknown tool %q on source %q", rl.Tool, rl.Source)
			}
		}
	}
	for _, c := range cfg.Consumers {
		for _, rule := range c.Allow {
			if rule.Source == "*" {
				continue
			}
			tools, ok := bySource[rule.Source]
			if !ok {
				return fmt.Errorf("consumer %q references unknown source %q", c.Name, rule.Source)
			}
			for _, allowed := range rule.Tools {
				if allowed == "*" {
					continue
				}
				if len(tools) > 0 && !tools[allowed] {
					return fmt.Errorf("consumer %q references unknown tool %q on source %q", c.Name, allowed, rule.Source)
				}
			}
		}
	}
	return nil
}
