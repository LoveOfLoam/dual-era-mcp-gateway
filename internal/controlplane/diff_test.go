package controlplane

import "testing"

func TestDiffConfigs(t *testing.T) {
	oldCfg := baseConfig()
	newCfg := baseConfig()
	newCfg.Sources[0].Endpoint = "http://weather2:8080/mcp"
	newCfg.Sources = append(newCfg.Sources, newCfg.Sources[1])
	newCfg.Sources[2].Name = "crm2"
	newCfg.Consumers[0].Allow = nil
	d := DiffConfigs(oldCfg, newCfg)
	if !d.HasChanges() {
		t.Fatal("expected changes")
	}
	if len(d.ChangedSources) != 1 || d.ChangedSources[0] != "weather" {
		t.Fatalf("changed = %+v", d.ChangedSources)
	}
	if len(d.AddedSources) != 1 || d.AddedSources[0] != "crm2" {
		t.Fatalf("added = %+v", d.AddedSources)
	}
	if len(d.ChangedTenants) != 1 || d.ChangedTenants[0] != "acme" {
		t.Fatalf("tenants = %+v", d.ChangedTenants)
	}
}

func TestDiffNoChanges(t *testing.T) {
	cfg := baseConfig()
	d := DiffConfigs(cfg, cfg)
	if d.HasChanges() {
		t.Fatalf("unexpected diff: %+v", d)
	}
}
