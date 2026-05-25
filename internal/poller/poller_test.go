package poller

import (
	"context"
	"errors"
	"testing"
	"time"

	agentic "github.com/openshift/lightspeed-agentic-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/rioloc/lightspeed-agentic-alerts-adapter/internal/alertmanager"
)

func newScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := agentic.AddToScheme(s); err != nil {
		t.Fatalf("failed to add agentic scheme: %v", err)
	}
	return s
}

func newPoller(t *testing.T, fetcher alertmanager.AlertFetcher, objs []client.Object) *Poller {
	t.Helper()
	s := newScheme(t)
	builder := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&agentic.Proposal{})
	if len(objs) > 0 {
		builder = builder.WithObjects(objs...)
	}
	k8s := builder.Build()
	return NewPoller(fetcher, k8s, Config{
		InitialDelay:     5 * time.Minute,
		CooldownWindow:   1 * time.Hour,
		DefaultNamespace: "openshift-lightspeed",
		DefaultAgent:     "default",
	})
}

func TestNewPoller(t *testing.T) {
	fetcher := &fakeFetcher{}
	s := newScheme(t)
	k8s := fake.NewClientBuilder().WithScheme(s).Build()
	cfg := Config{
		InitialDelay:     5 * time.Minute,
		CooldownWindow:   1 * time.Hour,
		DefaultNamespace: "openshift-lightspeed",
		DefaultAgent:     "default",
	}

	p := NewPoller(fetcher, k8s, cfg)
	if p == nil {
		t.Fatal("NewPoller returned nil")
	}
	if p.alertFetcher == nil {
		t.Error("alertFetcher is nil")
	}
	if p.k8sClient == nil {
		t.Error("k8sClient is nil")
	}
	if p.config.InitialDelay != 5*time.Minute {
		t.Errorf("InitialDelay = %v, want %v", p.config.InitialDelay, 5*time.Minute)
	}
	if p.config.CooldownWindow != 1*time.Hour {
		t.Errorf("CooldownWindow = %v, want %v", p.config.CooldownWindow, 1*time.Hour)
	}
	if p.config.DefaultNamespace != "openshift-lightspeed" {
		t.Errorf("DefaultNamespace = %q, want %q", p.config.DefaultNamespace, "openshift-lightspeed")
	}
	if p.config.DefaultAgent != "default" {
		t.Errorf("DefaultAgent = %q, want %q", p.config.DefaultAgent, "default")
	}

	var _ alertmanager.AlertFetcher = fetcher
}

func TestPoller_InitialDelayFilter(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name      string
		startsAt  time.Time
		wantPass  bool
	}{
		{
			name:     "alert started 10 minutes ago passes 5min delay",
			startsAt: now.Add(-10 * time.Minute),
			wantPass: true,
		},
		{
			name:     "alert started 2 minutes ago fails 5min delay",
			startsAt: now.Add(-2 * time.Minute),
			wantPass: false,
		},
		{
			name:     "alert started exactly 5 minutes ago passes",
			startsAt: now.Add(-5 * time.Minute),
			wantPass: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newPoller(t, &fakeFetcher{}, nil)
			alert := alertmanager.Alert{
				Labels:   map[string]string{"alertname": "TestAlert"},
				StartsAt: tt.startsAt,
			}
			got := p.passesInitialDelay(alert, now)
			if got != tt.wantPass {
				t.Errorf("passesInitialDelay() = %v, want %v", got, tt.wantPass)
			}
		})
	}
}

func TestPoller_ActiveProposalDedup(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name       string
		alert      alertmanager.Alert
		proposals  []client.Object
		wantActive bool
	}{
		{
			name: "no existing proposal",
			alert: alertmanager.Alert{
				Labels:      map[string]string{"alertname": "TestAlert", "namespace": "test-ns"},
				Fingerprint: "abcdef123456",
			},
			proposals:  nil,
			wantActive: false,
		},
		{
			name: "active proposal exists (Pending phase)",
			alert: alertmanager.Alert{
				Labels:      map[string]string{"alertname": "TestAlert", "namespace": "test-ns"},
				Fingerprint: "abcdef123456",
			},
			proposals: []client.Object{
				&agentic.Proposal{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "testalert-test-ns-abcdef12",
						Namespace: "test-ns",
						Labels: map[string]string{
							"agentic.openshift.io/alert-fingerprint": "abcdef12",
						},
					},
				},
			},
			wantActive: true,
		},
		{
			name: "terminal proposal exists (Completed) - not active",
			alert: alertmanager.Alert{
				Labels:      map[string]string{"alertname": "TestAlert", "namespace": "test-ns"},
				Fingerprint: "abcdef123456",
			},
			proposals: []client.Object{
				&agentic.Proposal{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "testalert-test-ns-abcdef12",
						Namespace: "test-ns",
						Labels: map[string]string{
							"agentic.openshift.io/alert-fingerprint": "abcdef12",
						},
					},
					Status: agentic.ProposalStatus{
						Conditions: []metav1.Condition{
							{
								Type:               agentic.ProposalConditionVerified,
								Status:             metav1.ConditionTrue,
								LastTransitionTime: metav1.NewTime(time.Now().Add(-2 * time.Hour)),
							},
						},
					},
				},
			},
			wantActive: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newPoller(t, &fakeFetcher{}, tt.proposals)
			got, err := p.hasActiveProposal(ctx, tt.alert)
			if err != nil {
				t.Fatalf("hasActiveProposal() error = %v", err)
			}
			if got != tt.wantActive {
				t.Errorf("hasActiveProposal() = %v, want %v", got, tt.wantActive)
			}
		})
	}
}

func TestPoller_CooldownWindow(t *testing.T) {
	ctx := context.Background()
	now := time.Now()

	tests := []struct {
		name           string
		alert          alertmanager.Alert
		proposals      []client.Object
		wantWithin     bool
	}{
		{
			name: "terminal proposal 30min ago, cooldown 1h - within cooldown",
			alert: alertmanager.Alert{
				Labels:      map[string]string{"alertname": "TestAlert", "namespace": "test-ns"},
				Fingerprint: "abcdef123456",
			},
			proposals: []client.Object{
				&agentic.Proposal{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "testalert-test-ns-abcdef12",
						Namespace: "test-ns",
						Labels: map[string]string{
							"agentic.openshift.io/alert-fingerprint": "abcdef12",
						},
					},
					Status: agentic.ProposalStatus{
						Conditions: []metav1.Condition{
							{
								Type:               agentic.ProposalConditionVerified,
								Status:             metav1.ConditionTrue,
								LastTransitionTime: metav1.NewTime(now.Add(-30 * time.Minute)),
							},
						},
					},
				},
			},
			wantWithin: true,
		},
		{
			name: "terminal proposal 2h ago, cooldown 1h - expired",
			alert: alertmanager.Alert{
				Labels:      map[string]string{"alertname": "TestAlert", "namespace": "test-ns"},
				Fingerprint: "abcdef123456",
			},
			proposals: []client.Object{
				&agentic.Proposal{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "testalert-test-ns-abcdef12",
						Namespace: "test-ns",
						Labels: map[string]string{
							"agentic.openshift.io/alert-fingerprint": "abcdef12",
						},
					},
					Status: agentic.ProposalStatus{
						Conditions: []metav1.Condition{
							{
								Type:               agentic.ProposalConditionVerified,
								Status:             metav1.ConditionTrue,
								LastTransitionTime: metav1.NewTime(now.Add(-2 * time.Hour)),
							},
						},
					},
				},
			},
			wantWithin: false,
		},
		{
			name: "no terminal proposal - not within cooldown",
			alert: alertmanager.Alert{
				Labels:      map[string]string{"alertname": "TestAlert", "namespace": "test-ns"},
				Fingerprint: "aaaa11112222",
			},
			proposals:  nil,
			wantWithin: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newPoller(t, &fakeFetcher{}, tt.proposals)
			got, err := p.withinCooldown(ctx, tt.alert, now)
			if err != nil {
				t.Fatalf("withinCooldown() error = %v", err)
			}
			if got != tt.wantWithin {
				t.Errorf("withinCooldown() = %v, want %v", got, tt.wantWithin)
			}
		})
	}
}

func TestPoller_CreateProposal(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name      string
		existing  []client.Object
		proposal  *agentic.Proposal
		wantErr   bool
	}{
		{
			name: "successful creation",
			proposal: &agentic.Proposal{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-proposal",
					Namespace: "openshift-lightspeed",
				},
			},
			wantErr: false,
		},
		{
			name: "already exists (409) treated as success",
			existing: []client.Object{
				&agentic.Proposal{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-proposal",
						Namespace: "openshift-lightspeed",
					},
				},
			},
			proposal: &agentic.Proposal{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-proposal",
					Namespace: "openshift-lightspeed",
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newPoller(t, &fakeFetcher{}, tt.existing)
			err := p.createProposal(ctx, tt.proposal)
			if (err != nil) != tt.wantErr {
				t.Errorf("createProposal() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPoller_PollCycle(t *testing.T) {
	now := time.Now()

	t.Run("mixed alerts: one eligible, one too recent, one with active proposal", func(t *testing.T) {
		activeProposal := &agentic.Proposal{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "activealert-prod-bbbbbbbb",
				Namespace: "prod",
				Labels: map[string]string{
					"agentic.openshift.io/alert-fingerprint": "bbbbbbbb",
				},
			},
		}

		alerts := []alertmanager.Alert{
			{
				Labels:      map[string]string{"alertname": "TooRecent", "namespace": "test-ns"},
				Annotations: map[string]string{"summary": "too recent"},
				StartsAt:    now.Add(-1 * time.Minute),
				Fingerprint: "aaaaaaaaaa",
			},
			{
				Labels:      map[string]string{"alertname": "ActiveAlert", "namespace": "prod"},
				Annotations: map[string]string{"summary": "already active"},
				StartsAt:    now.Add(-10 * time.Minute),
				Fingerprint: "bbbbbbbbbb",
			},
			{
				Labels:      map[string]string{"alertname": "EligibleAlert", "namespace": "staging"},
				Annotations: map[string]string{"summary": "should create proposal"},
				StartsAt:    now.Add(-10 * time.Minute),
				Fingerprint: "cccccccccc",
			},
		}

		fetcher := &fakeFetcher{alerts: alerts}
		p := newPoller(t, fetcher, []client.Object{activeProposal})

		err := p.PollOnce(context.Background())
		if err != nil {
			t.Fatalf("PollOnce() error = %v", err)
		}

		var proposals agentic.ProposalList
		if err := p.k8sClient.List(context.Background(), &proposals); err != nil {
			t.Fatalf("listing proposals: %v", err)
		}

		created := 0
		for _, prop := range proposals.Items {
			if prop.Name != activeProposal.Name {
				created++
			}
		}
		if created != 1 {
			t.Errorf("expected 1 new proposal created, got %d", created)
		}
	})

	t.Run("alertmanager error skips cycle", func(t *testing.T) {
		fetcher := &fakeFetcher{err: errors.New("connection refused")}
		p := newPoller(t, fetcher, nil)

		err := p.PollOnce(context.Background())
		if err == nil {
			t.Fatal("expected error from PollOnce when fetcher fails")
		}

		var proposals agentic.ProposalList
		if err := p.k8sClient.List(context.Background(), &proposals); err != nil {
			t.Fatalf("listing proposals: %v", err)
		}
		if len(proposals.Items) != 0 {
			t.Errorf("expected 0 proposals, got %d", len(proposals.Items))
		}
	})

	t.Run("alert with missing alertname is skipped", func(t *testing.T) {
		alerts := []alertmanager.Alert{
			{
				Labels:      map[string]string{},
				Annotations: map[string]string{"summary": "no alertname"},
				StartsAt:    now.Add(-10 * time.Minute),
				Fingerprint: "dddddddddd",
			},
			{
				Labels:      map[string]string{"alertname": "ValidAlert", "namespace": "ns1"},
				Annotations: map[string]string{"summary": "valid"},
				StartsAt:    now.Add(-10 * time.Minute),
				Fingerprint: "eeeeeeeeee",
			},
		}

		fetcher := &fakeFetcher{alerts: alerts}
		p := newPoller(t, fetcher, nil)

		err := p.PollOnce(context.Background())
		if err != nil {
			t.Fatalf("PollOnce() error = %v", err)
		}

		var proposals agentic.ProposalList
		if err := p.k8sClient.List(context.Background(), &proposals); err != nil {
			t.Fatalf("listing proposals: %v", err)
		}
		if len(proposals.Items) != 1 {
			t.Errorf("expected 1 proposal (skipping no-name alert), got %d", len(proposals.Items))
		}
	})
}
