package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestHealthEndpoint(t *testing.T) {
	// Create a gateway server
	server := httptest.NewServer(http.HandlerFunc(healthHandler))
	defer server.Close()

	// Make a request to the health endpoint
	resp, err := http.Get(server.URL + "/health")
	assert.NoError(t, err, "health endpoint should not error")
	defer resp.Body.Close()

	// Verify status code
	assert.Equal(t, http.StatusOK, resp.StatusCode, "health endpoint should return 200")

	// Verify content type
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"), "response should be JSON")

	// Verify response body
	var result map[string]string
	err = json.NewDecoder(resp.Body).Decode(&result)
	assert.NoError(t, err, "response body should be valid JSON")
	assert.Equal(t, "ok", result["status"], "status should be 'ok'")
}

func TestServerStartStop(t *testing.T) {
	cfg := ServerConfig{
		Host: "127.0.0.1",
		Port: 19999, // Use a specific port to test lifecycle
	}

	gw := New(cfg)

	// Start the server in a goroutine
	done := make(chan error, 1)
	go func() {
		done <- gw.Start(context.Background())
	}()

	// Give the server time to start
	time.Sleep(100 * time.Millisecond)

	// Verify the server is running by making a request
	client := &http.Client{
		Timeout: time.Second,
	}
	resp, err := client.Get("http://127.0.0.1:19999/health")
	assert.NoError(t, err, "should be able to connect to running server")
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Stop the server gracefully
	stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = gw.Stop(stopCtx)
	assert.NoError(t, err, "stop should not error")

	// Wait for the server goroutine to finish
	select {
	case err := <-done:
		// Server should exit cleanly (either nil or ErrServerClosed)
		if err != nil && err != http.ErrServerClosed {
			t.Fatalf("unexpected server error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for server to stop")
	}
}

func TestGatewayCreation(t *testing.T) {
	cfg := ServerConfig{
		Host: "localhost",
		Port: 8080,
	}

	gw := New(cfg)

	assert.NotNil(t, gw, "gateway should be created")
	assert.NotNil(t, gw.server, "server should be initialized")
	assert.Equal(t, "localhost:8080", gw.server.Addr, "server address should match config")
}
