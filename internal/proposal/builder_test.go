package proposal

import (
	"strings"
	"testing"
	"time"

	"github.com/rioloc/lightspeed-agentic-alerts-adapter/internal/alertmanager"
)

func TestRenderRequest(t *testing.T) {
	tests := []struct {
		name    string
		alert   alertmanager.Alert
		wantErr bool
		check   func(t *testing.T, result string)
	}{
		{
			name: "full alert data",
			alert: alertmanager.Alert{
				Labels: map[string]string{
					"alertname": "KubePodCrashLooping",
					"severity":  "critical",
					"namespace": "production",
					"pod":       "nginx-abc123",
				},
				Annotations: map[string]string{
					"summary":     "Pod is crash looping",
					"description": "Pod production/nginx-abc123 has restarted 5 times in the last 10 minutes",
				},
			},
			check: func(t *testing.T, result string) {
				for _, want := range []string{
					"KubePodCrashLooping",
					"critical",
					"production",
					"Pod is crash looping",
					"Pod production/nginx-abc123 has restarted 5 times",
					"pod: nginx-abc123",
				} {
					if !strings.Contains(result, want) {
						t.Errorf("rendered request missing %q:\n%s", want, result)
					}
				}
			},
		},
		{
			name: "missing optional fields",
			alert: alertmanager.Alert{
				Labels: map[string]string{
					"alertname": "EtcdHighFsyncDurations",
				},
				Annotations: map[string]string{},
			},
			check: func(t *testing.T, result string) {
				if !strings.Contains(result, "EtcdHighFsyncDurations") {
					t.Errorf("rendered request missing alert name:\n%s", result)
				}
				if strings.Contains(result, "<no value>") {
					t.Errorf("rendered request contains <no value>:\n%s", result)
				}
			},
		},
		{
			name: "special characters in annotations",
			alert: alertmanager.Alert{
				Labels: map[string]string{
					"alertname": "TestAlert",
				},
				Annotations: map[string]string{
					"summary":     `Pod has "unusual" <state> & errors`,
					"description": "Line1\nLine2\ttab",
				},
			},
			check: func(t *testing.T, result string) {
				if !strings.Contains(result, `"unusual"`) {
					t.Errorf("rendered request should preserve quotes:\n%s", result)
				}
				if !strings.Contains(result, "<state>") {
					t.Errorf("rendered request should preserve angle brackets (text/template):\n%s", result)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RenderRequest(tt.alert)
			if (err != nil) != tt.wantErr {
				t.Fatalf("RenderRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}

func TestBuildProposal(t *testing.T) {
	tests := []struct {
		name             string
		alert            alertmanager.Alert
		defaultNamespace string
		agentName        string
		wantNamespace    string
		wantTargetNS     []string
		wantErr          bool
	}{
		{
			name: "namespaced alert",
			alert: alertmanager.Alert{
				Labels: map[string]string{
					"alertname": "KubePodCrashLooping",
					"namespace": "production",
				},
				Annotations: map[string]string{
					"summary": "Pod is crash looping",
				},
				Fingerprint: "a1b2c3d4e5f6",
			},
			defaultNamespace: "openshift-lightspeed",
			agentName:        "default",
			wantNamespace:    "production",
			wantTargetNS:     []string{"production"},
		},
		{
			name: "cluster-scoped alert no namespace",
			alert: alertmanager.Alert{
				Labels: map[string]string{
					"alertname": "EtcdHighFsyncDurations",
				},
				Annotations: map[string]string{
					"summary": "etcd fsync durations are high",
				},
				Fingerprint: "f9e8d7c6b5a4",
			},
			defaultNamespace: "openshift-lightspeed",
			agentName:        "default",
			wantNamespace:    "openshift-lightspeed",
			wantTargetNS:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := BuildProposal(tt.alert, tt.defaultNamespace, tt.agentName)
			if (err != nil) != tt.wantErr {
				t.Fatalf("BuildProposal() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			wantName := ProposalName(tt.alert.AlertName(), tt.alert.Namespace(), tt.alert.Fingerprint)
			if got.Name != wantName {
				t.Errorf("Name = %q, want %q", got.Name, wantName)
			}

			if got.Namespace != tt.wantNamespace {
				t.Errorf("Namespace = %q, want %q", got.Namespace, tt.wantNamespace)
			}

			if len(tt.wantTargetNS) == 0 && len(got.Spec.TargetNamespaces) != 0 {
				t.Errorf("TargetNamespaces = %v, want empty", got.Spec.TargetNamespaces)
			}
			if len(tt.wantTargetNS) > 0 {
				if len(got.Spec.TargetNamespaces) != len(tt.wantTargetNS) {
					t.Errorf("TargetNamespaces = %v, want %v", got.Spec.TargetNamespaces, tt.wantTargetNS)
				}
				for i, ns := range tt.wantTargetNS {
					if got.Spec.TargetNamespaces[i] != ns {
						t.Errorf("TargetNamespaces[%d] = %q, want %q", i, got.Spec.TargetNamespaces[i], ns)
					}
				}
			}

			if got.Spec.Analysis.Agent != tt.agentName {
				t.Errorf("Analysis.Agent = %q, want %q", got.Spec.Analysis.Agent, tt.agentName)
			}
			if got.Spec.Execution.Agent != tt.agentName {
				t.Errorf("Execution.Agent = %q, want %q", got.Spec.Execution.Agent, tt.agentName)
			}
			if got.Spec.Verification.Agent != tt.agentName {
				t.Errorf("Verification.Agent = %q, want %q", got.Spec.Verification.Agent, tt.agentName)
			}

			if got.Spec.AnalysisOutput.Mode != "Default" {
				t.Errorf("AnalysisOutput.Mode = %q, want %q", got.Spec.AnalysisOutput.Mode, "Default")
			}

			if got.Spec.Request == "" {
				t.Error("Request should not be empty")
			}
		})
	}
}

func TestBuildProposal_LabelsAndAnnotations(t *testing.T) {
	startsAt := time.Date(2026, 5, 21, 10, 30, 0, 0, time.UTC)
	alert := alertmanager.Alert{
		Labels: map[string]string{
			"alertname": "KubePodCrashLooping",
			"severity":  "critical",
			"namespace": "production",
		},
		Annotations: map[string]string{
			"summary": "Pod is crash looping",
		},
		Fingerprint: "a1b2c3d4e5f6",
		StartsAt:    startsAt,
	}

	got, err := BuildProposal(alert, "openshift-lightspeed", "default")
	if err != nil {
		t.Fatalf("BuildProposal() error = %v", err)
	}

	wantLabels := map[string]string{
		"agentic.openshift.io/source":            "alertmanager",
		"agentic.openshift.io/alert-fingerprint": "a1b2c3d4",
		"agentic.openshift.io/alert-name":        "kubepodcrashlooping",
		"agentic.openshift.io/alert-severity":     "critical",
	}
	for k, want := range wantLabels {
		if got.Labels[k] != want {
			t.Errorf("Label %q = %q, want %q", k, got.Labels[k], want)
		}
	}

	wantStartsAt := startsAt.Format(time.RFC3339)
	if got.Annotations["agentic.openshift.io/alert-starts-at"] != wantStartsAt {
		t.Errorf("Annotation alert-starts-at = %q, want %q",
			got.Annotations["agentic.openshift.io/alert-starts-at"], wantStartsAt)
	}

	if got.Annotations["agentic.openshift.io/alert-summary"] != "Pod is crash looping" {
		t.Errorf("Annotation alert-summary = %q, want %q",
			got.Annotations["agentic.openshift.io/alert-summary"], "Pod is crash looping")
	}
}

func TestBuildProposal_LongSummaryTruncated(t *testing.T) {
	alert := alertmanager.Alert{
		Labels: map[string]string{
			"alertname": "TestAlert",
		},
		Annotations: map[string]string{
			"summary": strings.Repeat("x", 300),
		},
		Fingerprint: "abcdef12",
	}

	got, err := BuildProposal(alert, "openshift-lightspeed", "default")
	if err != nil {
		t.Fatalf("BuildProposal() error = %v", err)
	}

	summary := got.Annotations["agentic.openshift.io/alert-summary"]
	if len(summary) > maxAnnotationValueLength {
		t.Errorf("summary annotation length = %d, want <= %d", len(summary), maxAnnotationValueLength)
	}
}
