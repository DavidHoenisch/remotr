package diagnostics

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUploadClient_usesSeparateClientsForServerAndStore(t *testing.T) {
	t.Parallel()

	var putReceived bool
	store := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			http.Error(w, "method", http.StatusMethodNotAllowed)
			return
		}
		putReceived = true
		if _, err := io.ReadAll(r.Body); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer store.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/diagnostics/upload-url" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"url":"` + store.URL + `","key":"diagnostics/test.tar.gz"}`))
	}))
	defer server.Close()

	client := NewUploadClient(server.URL, nil)
	if client.serverClient == client.storeClient {
		t.Fatal("expected distinct HTTP clients for server and object store")
	}

	err := client.Upload(context.Background(), "req-1", Bundle{Data: []byte("bundle"), Size: 6})
	if err != nil {
		t.Fatal(err)
	}
	if !putReceived {
		t.Fatal("expected PUT to object store URL")
	}
}
