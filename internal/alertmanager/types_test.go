package alertmanager

import (
	"encoding/json"
	"testing"
	"time"
)

func TestAlert_Accessors(t *testing.T) {
	tests := []struct {
		name   string
		alert  Alert
		method string
		want   string
	}{
		{
			name: "AlertName returns alertname label",
			alert: Alert{
				Labels: map[string]string{"alertname": "KubePodCrashLooping"},
			},
			method: "AlertName",
			want:   "KubePodCrashLooping",
		},
		{
			name: "Severity returns severity label",
			alert: Alert{
				Labels: map[string]string{"severity": "critical"},
			},
			method: "Severity",
			want:   "critical",
		},
		{
			name:   "Severity returns empty when missing",
			alert:  Alert{Labels: map[string]string{}},
			method: "Severity",
			want:   "",
		},
		{
			name: "Namespace returns namespace label",
			alert: Alert{
				Labels: map[string]string{"namespace": "production"},
			},
			method: "Namespace",
			want:   "production",
		},
		{
			name:   "Namespace returns empty when missing",
			alert:  Alert{Labels: map[string]string{}},
			method: "Namespace",
			want:   "",
		},
		{
			name: "Summary returns summary annotation",
			alert: Alert{
				Annotations: map[string]string{"summary": "Pod is crash looping"},
			},
			method: "Summary",
			want:   "Pod is crash looping",
		},
		{
			name: "Description returns description annotation",
			alert: Alert{
				Annotations: map[string]string{"description": "Detailed info"},
			},
			method: "Description",
			want:   "Detailed info",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got string
			switch tt.method {
			case "AlertName":
				got = tt.alert.AlertName()
			case "Severity":
				got = tt.alert.Severity()
			case "Namespace":
				got = tt.alert.Namespace()
			case "Summary":
				got = tt.alert.Summary()
			case "Description":
				got = tt.alert.Description()
			}
			if got != tt.want {
				t.Errorf("%s() = %q, want %q", tt.method, got, tt.want)
			}
		})
	}
}

func TestAlert_JSONUnmarshal(t *testing.T) {
	raw := `{
		"labels": {
			"alertname": "KubePodCrashLooping",
			"namespace": "production",
			"severity": "critical"
		},
		"annotations": {
			"summary": "Pod is crash looping",
			"description": "Pod production/my-pod is restarting frequently"
		},
		"startsAt": "2026-05-21T10:00:00Z",
		"endsAt": "0001-01-01T00:00:00Z",
		"fingerprint": "a1b2c3d4e5f6",
		"status": {
			"state": "firing"
		}
	}`

	var alert Alert
	if err := json.Unmarshal([]byte(raw), &alert); err != nil {
		t.Fatalf("unexpected unmarshal error: %v", err)
	}

	if got := alert.Labels["alertname"]; got != "KubePodCrashLooping" {
		t.Errorf("Labels[alertname] = %q, want %q", got, "KubePodCrashLooping")
	}
	if got := alert.Labels["namespace"]; got != "production" {
		t.Errorf("Labels[namespace] = %q, want %q", got, "production")
	}
	if got := alert.Annotations["summary"]; got != "Pod is crash looping" {
		t.Errorf("Annotations[summary] = %q, want %q", got, "Pod is crash looping")
	}
	if got := alert.Fingerprint; got != "a1b2c3d4e5f6" {
		t.Errorf("Fingerprint = %q, want %q", got, "a1b2c3d4e5f6")
	}
	if got := alert.Status.State; got != "firing" {
		t.Errorf("Status.State = %q, want %q", got, "firing")
	}

	wantStartsAt := time.Date(2026, 5, 21, 10, 0, 0, 0, time.UTC)
	if !alert.StartsAt.Equal(wantStartsAt) {
		t.Errorf("StartsAt = %v, want %v", alert.StartsAt, wantStartsAt)
	}

	wantEndsAt := time.Time{}
	if !alert.EndsAt.Equal(wantEndsAt) {
		t.Errorf("EndsAt = %v, want %v", alert.EndsAt, wantEndsAt)
	}
}
