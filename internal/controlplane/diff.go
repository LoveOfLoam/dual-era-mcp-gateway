package controlplane

import (
	"encoding/json"
	"fmt"
	"sort"
)

// Diff is a lightweight semantic diff of two configurations. It is intended
// for the dry-run API, not for generating a JSON Patch.
type Diff struct {
	AddedSources      []string `json:"addedSources,omitempty"`
	RemovedSources    []string `json:"removedSources,omitempty"`
	ChangedSources    []string `json:"changedSources,omitempty"`
	AddedTenants      []string `json:"addedTenants,omitempty"`
	RemovedTenants    []string `json:"removedTenants,omitempty"`
	ChangedTenants    []string `json:"changedTenants,omitempty"`
	AddedRateLimits   []string `json:"addedRateLimits,omitempty"`
	RemovedRateLimits []string `json:"removedRateLimits,omitempty"`
}

// HasChanges reports whether the diff is non-empty.
func (d Diff) HasChanges() bool {
	return len(d.AddedSources)+len(d.RemovedSources)+len(d.ChangedSources)+
		len(d.AddedTenants)+len(d.RemovedTenants)+len(d.ChangedTenants)+
		len(d.AddedRateLimits)+len(d.RemovedRateLimits) > 0
}

// DiffConfigs compares two configs by stable keys.
func DiffConfigs(oldCfg, newCfg Config) Diff {
	var d Diff

	oldSources := map[string]string{}
	for _, s := range oldCfg.Sources {
		oldSources[s.Name] = jsonString(s)
	}
	newSources := map[string]string{}
	for _, s := range newCfg.Sources {
		newSources[s.Name] = jsonString(s)
	}
	for name, v := range newSources {
		ov, ok := oldSources[name]
		if !ok {
			d.AddedSources = append(d.AddedSources, name)
		} else if ov != v {
			d.ChangedSources = append(d.ChangedSources, name)
		}
	}
	for name := range oldSources {
		if _, ok := newSources[name]; !ok {
			d.RemovedSources = append(d.RemovedSources, name)
		}
	}

	oldTenants := map[string]string{}
	for _, c := range oldCfg.Consumers {
		oldTenants[c.Tenant] = jsonString(c)
	}
	newTenants := map[string]string{}
	for _, c := range newCfg.Consumers {
		newTenants[c.Tenant] = jsonString(c)
	}
	for tenant, v := range newTenants {
		ov, ok := oldTenants[tenant]
		if !ok {
			d.AddedTenants = append(d.AddedTenants, tenant)
		} else if ov != v {
			d.ChangedTenants = append(d.ChangedTenants, tenant)
		}
	}
	for tenant := range oldTenants {
		if _, ok := newTenants[tenant]; !ok {
			d.RemovedTenants = append(d.RemovedTenants, tenant)
		}
	}

	oldLimits := limitSet(oldCfg)
	newLimits := limitSet(newCfg)
	for k := range newLimits {
		if !oldLimits[k] {
			d.AddedRateLimits = append(d.AddedRateLimits, k)
		}
	}
	for k := range oldLimits {
		if !newLimits[k] {
			d.RemovedRateLimits = append(d.RemovedRateLimits, k)
		}
	}

	sort.Strings(d.AddedSources)
	sort.Strings(d.RemovedSources)
	sort.Strings(d.ChangedSources)
	sort.Strings(d.AddedTenants)
	sort.Strings(d.RemovedTenants)
	sort.Strings(d.ChangedTenants)
	sort.Strings(d.AddedRateLimits)
	sort.Strings(d.RemovedRateLimits)
	return d
}

func limitSet(cfg Config) map[string]bool {
	out := map[string]bool{}
	for _, l := range cfg.RateLimits {
		out[fmt.Sprintf("%s|%s|%s|%d|%d", l.Tenant, l.Source, l.Tool, l.RPM, l.Burst)] = true
	}
	return out
}

func jsonString(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
