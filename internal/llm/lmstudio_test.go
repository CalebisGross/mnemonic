package llm

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestHealth_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"data":[{"id":"model1"}]}`)
	}))
	defer srv.Close()

	p := NewLMStudioProvider(srv.URL, "chat", "embed", "", 5*time.Second, 2)

	if err := p.Health(context.Background()); err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
}

func TestHealth_ServerDown(t *testing.T) {
	// Create a server and immediately close it to get a valid but unreachable URL.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	closedURL := srv.URL
	srv.Close()

	p := NewLMStudioProvider(closedURL, "chat", "embed", "", 2*time.Second, 2)

	err := p.Health(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var provErr *ErrProviderUnavailable
	if !errors.As(err, &provErr) {
		t.Fatalf("expected ErrProviderUnavailable, got: %T: %v", err, err)
	}
}

func TestHealth_500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprint(w, `{"error":"internal"}`)
	}))
	defer srv.Close()

	p := NewLMStudioProvider(srv.URL, "chat", "embed", "", 5*time.Second, 2)

	err := p.Health(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var provErr *ErrProviderUnavailable
	if !errors.As(err, &provErr) {
		t.Fatalf("expected ErrProviderUnavailable, got: %T: %v", err, err)
	}
}

func TestEmbed_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embeddings" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"data":[{"embedding":[0.1,0.2,0.3],"index":0}],"model":"test"}`)
	}))
	defer srv.Close()

	p := NewLMStudioProvider(srv.URL, "chat", "embed", "", 5*time.Second, 2)

	vec, err := p.Embed(context.Background(), "hello")
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}

	expected := []float32{0.1, 0.2, 0.3}
	if len(vec) != len(expected) {
		t.Fatalf("expected %d dimensions, got %d", len(expected), len(vec))
	}
	for i, v := range expected {
		if vec[i] != v {
			t.Errorf("vec[%d] = %f, want %f", i, vec[i], v)
		}
	}
}

func TestEmbed_ServerError(t *testing.T) {
	// The provider retries 500s up to 3 times via doWithRetry, so always return 500.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprint(w, `{"error":"boom"}`)
	}))
	defer srv.Close()

	p := NewLMStudioProvider(srv.URL, "chat", "embed", "", 5*time.Second, 2)

	_, err := p.Embed(context.Background(), "hello")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var provErr *ErrProviderUnavailable
	if !errors.As(err, &provErr) {
		t.Fatalf("expected ErrProviderUnavailable, got: %T: %v", err, err)
	}
}

func TestBatchEmbed_Empty(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	p := NewLMStudioProvider(srv.URL, "chat", "embed", "", 5*time.Second, 2)

	result, err := p.BatchEmbed(context.Background(), []string{})
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("expected empty slice, got %d elements", len(result))
	}
	if called {
		t.Error("expected no HTTP call for empty input, but server was hit")
	}
}

func TestComplete_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"choices":[{"message":{"content":"hello"},"finish_reason":"stop"}],"usage":{"total_tokens":5}}`)
	}))
	defer srv.Close()

	p := NewLMStudioProvider(srv.URL, "chat", "embed", "", 5*time.Second, 2)

	resp, err := p.Complete(context.Background(), CompletionRequest{
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if resp.Content != "hello" {
		t.Errorf("content = %q, want %q", resp.Content, "hello")
	}
	if resp.StopReason != "stop" {
		t.Errorf("stop_reason = %q, want %q", resp.StopReason, "stop")
	}
	if resp.TokensUsed != 5 {
		t.Errorf("tokens_used = %d, want %d", resp.TokensUsed, 5)
	}
}

func TestComplete_NoChoices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"choices":[],"usage":{"total_tokens":0}}`)
	}))
	defer srv.Close()

	p := NewLMStudioProvider(srv.URL, "chat", "embed", "", 5*time.Second, 2)

	_, err := p.Complete(context.Background(), CompletionRequest{
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("expected error for empty choices, got nil")
	}
}

func TestEmbeddingModelName(t *testing.T) {
	p := NewLMStudioProvider("http://localhost:1234", "chat", "test-model", "", 5*time.Second, 2)

	if got := p.EmbeddingModelName(); got != "test-model" {
		t.Errorf("EmbeddingModelName() = %q, want %q", got, "test-model")
	}
}

func TestConcurrencyLimiter(t *testing.T) {
	// A slow handler that blocks until explicitly released.
	requestArrived := make(chan struct{})
	releaseHandler := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Signal that a request has arrived.
		select {
		case requestArrived <- struct{}{}:
		default:
		}
		// Block until released.
		<-releaseHandler
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}],"usage":{"total_tokens":1}}`)
	}))
	defer srv.Close()

	// maxConcurrent=1 means only one inflight request at a time.
	p := NewLMStudioProvider(srv.URL, "chat", "embed", "", 30*time.Second, 1)

	// Start first request in background — it will hold the semaphore.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = p.Complete(context.Background(), CompletionRequest{
			Messages: []Message{{Role: "user", Content: "first"}},
		})
	}()

	// Wait for the first request to actually reach the server.
	<-requestArrived

	// Second request with a very short timeout should fail because the slot is taken.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := p.Complete(ctx, CompletionRequest{
		Messages: []Message{{Role: "user", Content: "second"}},
	})
	if err == nil {
		t.Fatal("expected error from concurrency-limited second request, got nil")
	}

	var provErr *ErrProviderUnavailable
	if !errors.As(err, &provErr) {
		t.Fatalf("expected ErrProviderUnavailable, got: %T: %v", err, err)
	}

	// Unblock the first request and wait for it to finish.
	close(releaseHandler)
	wg.Wait()
}

func TestAuthHeader_Set(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"data":[{"id":"model1"}]}`)
	}))
	defer srv.Close()

	p := NewLMStudioProvider(srv.URL, "chat", "embed", "test-key-123", 5*time.Second, 2)
	if err := p.Health(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotAuth != "Bearer test-key-123" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer test-key-123")
	}
}

func TestAuthHeader_NotSetWhenEmpty(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"data":[{"id":"model1"}]}`)
	}))
	defer srv.Close()

	p := NewLMStudioProvider(srv.URL, "chat", "embed", "", 5*time.Second, 2)
	if err := p.Health(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotAuth != "" {
		t.Errorf("Authorization = %q, want empty (no key configured)", gotAuth)
	}
}
