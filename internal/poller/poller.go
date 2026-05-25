package poller

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	agentic "github.com/openshift/lightspeed-agentic-operator/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/rioloc/lightspeed-agentic-alerts-adapter/internal/alertmanager"
	"github.com/rioloc/lightspeed-agentic-alerts-adapter/internal/proposal"
)

var terminalPhases = map[agentic.ProposalPhase]bool{
	agentic.ProposalPhaseCompleted: true,
	agentic.ProposalPhaseFailed:    true,
	agentic.ProposalPhaseDenied:    true,
	agentic.ProposalPhaseEscalated: true,
}

type Config struct {
	InitialDelay     time.Duration
	CooldownWindow   time.Duration
	DefaultNamespace string
	DefaultAgent     string
}

type Poller struct {
	alertFetcher alertmanager.AlertFetcher
	k8sClient    client.Client
	config       Config
}

func NewPoller(alertFetcher alertmanager.AlertFetcher, k8sClient client.Client, config Config) *Poller {
	return &Poller{
		alertFetcher: alertFetcher,
		k8sClient:    k8sClient,
		config:       config,
	}
}

func (p *Poller) passesInitialDelay(alert alertmanager.Alert, now time.Time) bool {
	return now.Sub(alert.StartsAt) >= p.config.InitialDelay
}

func (p *Poller) hasActiveProposal(ctx context.Context, alert alertmanager.Alert) (bool, error) {
	fp := alert.Fingerprint
	if len(fp) > 8 {
		fp = fp[:8]
	}

	var proposals agentic.ProposalList
	if err := p.k8sClient.List(ctx, &proposals, client.MatchingLabels{
		"agentic.openshift.io/alert-fingerprint": fp,
	}); err != nil {
		return false, fmt.Errorf("listing proposals: %w", err)
	}

	for i := range proposals.Items {
		phase := agentic.DerivePhase(proposals.Items[i].Status.Conditions)
		if !terminalPhases[phase] {
			return true, nil
		}
	}
	return false, nil
}

func (p *Poller) withinCooldown(ctx context.Context, alert alertmanager.Alert, now time.Time) (bool, error) {
	fp := alert.Fingerprint
	if len(fp) > 8 {
		fp = fp[:8]
	}

	var proposals agentic.ProposalList
	if err := p.k8sClient.List(ctx, &proposals, client.MatchingLabels{
		"agentic.openshift.io/alert-fingerprint": fp,
	}); err != nil {
		return false, fmt.Errorf("listing proposals: %w", err)
	}

	for i := range proposals.Items {
		phase := agentic.DerivePhase(proposals.Items[i].Status.Conditions)
		if !terminalPhases[phase] {
			continue
		}
		for _, cond := range proposals.Items[i].Status.Conditions {
			if now.Sub(cond.LastTransitionTime.Time) < p.config.CooldownWindow {
				return true, nil
			}
		}
	}
	return false, nil
}

func (p *Poller) createProposal(ctx context.Context, prop *agentic.Proposal) error {
	err := p.k8sClient.Create(ctx, prop)
	if apierrors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

func (p *Poller) PollOnce(ctx context.Context) error {
	now := time.Now()

	alerts, err := p.alertFetcher.FetchFiringAlerts(ctx)
	if err != nil {
		return fmt.Errorf("fetching alerts: %w", err)
	}

	for _, alert := range alerts {
		name := alert.AlertName()
		if name == "" {
			slog.Warn("skipping alert with missing alertname", "fingerprint", alert.Fingerprint)
			continue
		}

		if !p.passesInitialDelay(alert, now) {
			slog.Debug("alert too recent, skipping", "alert", name, "startsAt", alert.StartsAt)
			continue
		}

		active, err := p.hasActiveProposal(ctx, alert)
		if err != nil {
			slog.Error("checking active proposals", "alert", name, "error", err)
			continue
		}
		if active {
			slog.Debug("active proposal exists, skipping", "alert", name)
			continue
		}

		cooldown, err := p.withinCooldown(ctx, alert, now)
		if err != nil {
			slog.Error("checking cooldown window", "alert", name, "error", err)
			continue
		}
		if cooldown {
			slog.Debug("within cooldown window, skipping", "alert", name)
			continue
		}

		prop, err := proposal.BuildProposal(alert, p.config.DefaultNamespace, p.config.DefaultAgent)
		if err != nil {
			slog.Error("building proposal", "alert", name, "error", err)
			continue
		}

		if err := p.createProposal(ctx, prop); err != nil {
			slog.Error("creating proposal", "alert", name, "error", err)
			continue
		}

		slog.Info("created proposal", "alert", name, "proposal", prop.Name, "namespace", prop.Namespace)
	}

	return nil
}

