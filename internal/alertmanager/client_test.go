package alertmanager

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewClient(t *testing.T) {
	c := NewClient("https://alertmanager.example.com", &http.Client{})
	if c == nil {
		t.Fatal("NewClient returned nil")
	}

	var _ AlertFetcher = c
}

func TestFetchFiringAlerts(t *testing.T) {
	tests := []struct {
		name       string
		handler    http.HandlerFunc
		cancelCtx  bool
		wantCount  int
		wantErr    bool
	}{
		{
			name: "success with valid alerts",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`[
					{
						"labels": {"alertname": "KubePodCrashLooping", "severity": "critical"},
						"annotations": {"summary": "Pod crash looping"},
						"startsAt": "2026-05-21T10:00:00Z",
						"endsAt": "0001-01-01T00:00:00Z",
						"fingerprint": "abc123def456",
						"status": {"state": "firing"}
					},
					{
						"labels": {"alertname": "EtcdHighFsyncDurations", "severity": "warning"},
						"annotations": {"summary": "Etcd fsync slow"},
						"startsAt": "2026-05-21T09:00:00Z",
						"endsAt": "0001-01-01T00:00:00Z",
						"fingerprint": "f9e8d7c6b5a4",
						"status": {"state": "firing"}
					}
				]`))
			},
			wantCount: 2,
		},
		{
			name: "empty array",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`[]`))
			},
			wantCount: 0,
		},
		{
			name: "HTTP 500 error",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			wantErr: true,
		},
		{
			name: "invalid JSON",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`not json`))
			},
			wantErr: true,
		},
		{
			name:      "context cancelled",
			handler:   func(w http.ResponseWriter, r *http.Request) {},
			cancelCtx: true,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(tt.handler)
			defer srv.Close()

			c := NewClient(srv.URL, srv.Client())

			ctx := context.Background()
			if tt.cancelCtx {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}

			alerts, err := c.FetchFiringAlerts(ctx)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(alerts) != tt.wantCount {
				t.Errorf("got %d alerts, want %d", len(alerts), tt.wantCount)
			}
		})
	}
}

func TestFetchFiringAlerts_FieldValues(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Path; got != "/api/v2/alerts" {
			t.Errorf("request path = %q, want /api/v2/alerts", got)
		}
		q := r.URL.Query()
		if got := q.Get("active"); got != "true" {
			t.Errorf("query param active = %q, want %q", got, "true")
		}
		if got := q.Get("silenced"); got != "false" {
			t.Errorf("query param silenced = %q, want %q", got, "false")
		}
		if got := q.Get("inhibited"); got != "false" {
			t.Errorf("query param inhibited = %q, want %q", got, "false")
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[{
			"labels": {"alertname": "TestAlert", "namespace": "test-ns"},
			"annotations": {"summary": "Test summary"},
			"startsAt": "2026-05-21T10:00:00Z",
			"endsAt": "0001-01-01T00:00:00Z",
			"fingerprint": "abc123",
			"status": {"state": "firing"}
		}]`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, srv.Client())
	alerts, err := c.FetchFiringAlerts(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(alerts) != 1 {
		t.Fatalf("got %d alerts, want 1", len(alerts))
	}

	a := alerts[0]
	if got := a.AlertName(); got != "TestAlert" {
		t.Errorf("AlertName() = %q, want %q", got, "TestAlert")
	}
	if got := a.Namespace(); got != "test-ns" {
		t.Errorf("Namespace() = %q, want %q", got, "test-ns")
	}
	if got := a.Summary(); got != "Test summary" {
		t.Errorf("Summary() = %q, want %q", got, "Test summary")
	}
	if got := a.Fingerprint; got != "abc123" {
		t.Errorf("Fingerprint = %q, want %q", got, "abc123")
	}
	if got := a.Status.State; got != "firing" {
		t.Errorf("Status.State = %q, want %q", got, "firing")
	}
}
