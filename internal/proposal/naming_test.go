package proposal

import (
	"strings"
	"testing"
)

func TestProposalName(t *testing.T) {
	tests := []struct {
		name        string
		alertname   string
		namespace   string
		fingerprint string
		want        string
	}{
		{
			name:        "normal case",
			alertname:   "KubePodCrashLooping",
			namespace:   "production",
			fingerprint: "a1b2c3d4e5f6",
			want:        "kubepodcrashlooping-production-a1b2c3d4",
		},
		{
			name:        "no namespace",
			alertname:   "EtcdHighFsyncDurations",
			namespace:   "",
			fingerprint: "f9e8d7c6b5a4",
			want:        "etcdhighfsyncdurations--f9e8d7c6",
		},
		{
			name:        "short fingerprint",
			alertname:   "TestAlert",
			namespace:   "default",
			fingerprint: "abc",
			want:        "testalert-default-abc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ProposalName(tt.alertname, tt.namespace, tt.fingerprint)
			if got != tt.want {
				t.Errorf("ProposalName(%q, %q, %q) = %q, want %q",
					tt.alertname, tt.namespace, tt.fingerprint, got, tt.want)
			}
		})
	}
}

func TestSanitizeDNSSubdomain(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "uppercase to lowercase",
			input: "KubePodCrashLooping",
			want:  "kubepodcrashlooping",
		},
		{
			name:  "non-alphanumeric replaced with dash",
			input: "alert_name!with@special#chars",
			want:  "alert-name-with-special-chars",
		},
		{
			name:  "leading and trailing dashes stripped",
			input: "--leading-and-trailing--",
			want:  "leading-and-trailing",
		},
		{
			name:  "exceeding 253 chars truncated",
			input: strings.Repeat("a", 300),
			want:  strings.Repeat("a", 253),
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "dots preserved",
			input: "my.alert.name",
			want:  "my.alert.name",
		},
		{
			name:  "trailing dash after truncation stripped",
			input: strings.Repeat("a", 252) + "-b",
			want:  strings.Repeat("a", 252),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeDNSSubdomain(tt.input)
			if got != tt.want {
				t.Errorf("SanitizeDNSSubdomain(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
