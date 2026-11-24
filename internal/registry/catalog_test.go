package registry

import (
	"encoding/json"
	"testing"
)

func tool(name, source string) Tool {
	return Tool{
		Name:         name,
		OriginalName: name,
		Source:       source,
		Description:  "desc " + name,
		InputSchema:  json.RawMessage(`{"type":"object"}`),
	}
}

func TestPrefix(t *testing.T) {
	if got := Prefix("weather", "get_weather"); got != "weather__get_weather" {
		t.Fatalf("got %q", got)
	}
}

func TestMergeSortsAndPrefixes(t *testing.T) {
	c, err := Merge(map[string][]Tool{
		"weather": {tool("get_weather", "weather"), tool("list_cities", "weather")},
		"orders":  {tool("create_order", "orders")},
	})
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	list := c.List()
	if len(list) != 3 {
		t.Fatalf("len = %d", len(list))
	}
	want := []string{"orders__create_order", "weather__get_weather", "weather__list_cities"}
	for i, name := range want {
		if list[i].Name != name {
			t.Fatalf("list[%d] = %s, want %s", i, list[i].Name, name)
		}
	}
}

func TestMergeRejectsCollision(t *testing.T) {
	_, err := Merge(map[string][]Tool{
		"a": {tool("x", "a"), tool("x", "a")},
	})
	if err == nil {
		t.Fatal("expected collision error")
	}
}

func TestMergeRejectsSourceWithSeparator(t *testing.T) {
	_, err := Merge(map[string][]Tool{
		"a__b": {tool("x", "a__b")},
	})
	if err == nil {
		t.Fatal("expected source validation error")
	}
}

func TestSingleSourceStillPrefixed(t *testing.T) {
	c, err := Merge(map[string][]Tool{
		"weather": {tool("get_weather", "weather")},
	})
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if got := c.List()[0].Name; got != "weather__get_weather" {
		t.Fatalf("got %q", got)
	}
}

func TestResolve(t *testing.T) {
	c, err := Merge(map[string][]Tool{
		"weather": {tool("get_weather", "weather")},
	})
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	got, ok := c.Resolve("weather__get_weather")
	if !ok || got.OriginalName != "get_weather" || got.Source != "weather" {
		t.Fatalf("Resolve = %+v, %v", got, ok)
	}
	if _, ok := c.Resolve("missing"); ok {
		t.Fatal("unexpected resolve")
	}
}
