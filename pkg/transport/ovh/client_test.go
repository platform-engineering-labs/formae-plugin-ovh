// pkg/transport/ovh/client_test.go
package ovh

import (
	"os"
	"testing"
)

func TestNewClient(t *testing.T) {
	// This test requires OVH credentials - skip if not configured
	if os.Getenv("OVH_APPLICATION_KEY") == "" {
		t.Skip("Requires OVH credentials (OVH_APPLICATION_KEY not set)")
	}

	cfg := &OVHConfig{
		Endpoint:          os.Getenv("OVH_ENDPOINT"),
		ApplicationKey:    os.Getenv("OVH_APPLICATION_KEY"),
		ApplicationSecret: os.Getenv("OVH_APPLICATION_SECRET"),
		ConsumerKey:       os.Getenv("OVH_CONSUMER_KEY"),
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	if client == nil {
		t.Fatal("NewClient() returned nil client")
	}
}

func TestRequestOptions(t *testing.T) {
	opts := RequestOptions{
		Method: "GET",
		Path:   "/domain/zone",
	}

	if opts.Method != "GET" {
		t.Errorf("Method = %v, want GET", opts.Method)
	}
	if opts.Path != "/domain/zone" {
		t.Errorf("Path = %v, want /domain/zone", opts.Path)
	}
}

func TestResponseStructure(t *testing.T) {
	resp := &Response{
		StatusCode: 200,
		Body:       map[string]interface{}{"name": "test"},
	}

	if resp.StatusCode != 200 {
		t.Errorf("StatusCode = %v, want 200", resp.StatusCode)
	}
	if resp.Body["name"] != "test" {
		t.Errorf("Body[name] = %v, want test", resp.Body["name"])
	}
}
