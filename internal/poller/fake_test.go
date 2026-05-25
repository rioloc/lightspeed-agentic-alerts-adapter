package poller

import (
	"context"

	"github.com/rioloc/lightspeed-agentic-alerts-adapter/internal/alertmanager"
)

type fakeFetcher struct {
	alerts []alertmanager.Alert
	err    error
}

func (f *fakeFetcher) FetchFiringAlerts(_ context.Context) ([]alertmanager.Alert, error) {
	return f.alerts, f.err
}
