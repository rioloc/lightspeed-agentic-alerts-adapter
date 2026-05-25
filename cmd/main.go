package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	agentic "github.com/openshift/lightspeed-agentic-operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/rioloc/lightspeed-agentic-alerts-adapter/internal/alertmanager"
	"github.com/rioloc/lightspeed-agentic-alerts-adapter/internal/poller"
)

const (
	pollInterval          = 30 * time.Second
	initialDelay          = 5 * time.Minute
	cooldownWindow        = 1 * time.Hour
	defaultAlertManagerURL = "https://alertmanager-main.openshift-monitoring.svc:9093"
	defaultNamespace      = "openshift-lightspeed"
	defaultAgent          = "default"
	probeAddr             = ":8081"

	saTokenPath  = "/var/run/secrets/kubernetes.io/serviceaccount/token"
	saCACertPath = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	if err := run(ctx); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	k8sClient, err := newK8sClient()
	if err != nil {
		return fmt.Errorf("creating kubernetes client: %w", err)
	}

	amURL := alertManagerURL()
	httpClient, err := newAlertManagerHTTPClient(amURL)
	if err != nil {
		return fmt.Errorf("creating alertmanager http client: %w", err)
	}

	amClient := alertmanager.NewClient(amURL, httpClient)

	p := poller.NewPoller(amClient, k8sClient, poller.Config{
		InitialDelay:     initialDelay,
		CooldownWindow:   cooldownWindow,
		DefaultNamespace: defaultNamespace,
		DefaultAgent:     defaultAgent,
	})

	var ready atomic.Bool

	probeSrv := newProbeServer(&ready)
	go func() {
		slog.Info("starting probe server", "addr", probeAddr)
		if err := probeSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("probe server failed", "error", err)
		}
	}()

	slog.Info("starting poll loop", "interval", pollInterval, "alertmanager", amURL)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		if err := p.PollOnce(ctx); err != nil {
			slog.Error("poll cycle failed", "error", err)
			ready.Store(false)
		} else {
			ready.Store(true)
		}

		select {
		case <-ctx.Done():
			slog.Info("shutting down")
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutdownCancel()
			return probeSrv.Shutdown(shutdownCtx)
		case <-ticker.C:
		}
	}
}

func alertManagerURL() string {
	if v := os.Getenv("ALERTMANAGER_URL"); v != "" {
		return v
	}
	return defaultAlertManagerURL
}

func newProbeServer(ready *atomic.Bool) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		if ready.Load() {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	})
	return &http.Server{
		Addr:    probeAddr,
		Handler: mux,
	}
}

func newK8sClient() (client.Client, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		cfg, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			clientcmd.NewDefaultClientConfigLoadingRules(), nil,
		).ClientConfig()
		if err != nil {
			return nil, fmt.Errorf("loading kubernetes config: %w", err)
		}
	}

	scheme := runtime.NewScheme()
	if err := agentic.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("adding agentic scheme: %w", err)
	}

	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("creating client: %w", err)
	}
	return c, nil
}

func newAlertManagerHTTPClient(amURL string) (*http.Client, error) {
	if strings.HasPrefix(amURL, "http://") {
		return &http.Client{Timeout: 10 * time.Second}, nil
	}

	caCert, err := os.ReadFile(saCACertPath)
	if err != nil {
		return nil, fmt.Errorf("reading CA cert: %w", err)
	}

	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to parse CA certificate")
	}

	return &http.Client{
		Transport: &tokenTransport{
			tokenPath: saTokenPath,
			base: &http.Transport{
				TLSClientConfig: &tls.Config{
					RootCAs: caCertPool,
				},
			},
		},
		Timeout: 10 * time.Second,
	}, nil
}

type tokenTransport struct {
	tokenPath string
	base      http.RoundTripper
}

func (t *tokenTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	token, err := os.ReadFile(t.tokenPath)
	if err != nil {
		return nil, fmt.Errorf("reading service account token: %w", err)
	}
	req = req.Clone(req.Context())
	req.Header.Set("Authorization", "Bearer "+string(token))
	return t.base.RoundTrip(req)
}
