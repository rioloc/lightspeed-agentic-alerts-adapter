package proposal

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
	"time"

	agentic "github.com/openshift/lightspeed-agentic-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/rioloc/lightspeed-agentic-alerts-adapter/internal/alertmanager"
)

const maxAnnotationValueLength = 256

const requestTemplateSrc = `A Kubernetes alert is firing in the cluster.
Investigate the root cause and propose a remediation.

Alert: {{ .AlertName }}
Severity: {{ .Severity }}
Namespace: {{ .Namespace }}
Summary: {{ .Summary }}
Description: {{ .Description }}

Labels:
{{ range $k, $v := .Labels }}  {{ $k }}: {{ $v }}
{{ end }}`

var requestTmpl = template.Must(template.New("request").Parse(requestTemplateSrc))

type requestData struct {
	AlertName   string
	Severity    string
	Namespace   string
	Summary     string
	Description string
	Labels      map[string]string
}

func RenderRequest(alert alertmanager.Alert) (string, error) {
	data := requestData{
		AlertName:   alert.AlertName(),
		Severity:    alert.Severity(),
		Namespace:   alert.Namespace(),
		Summary:     alert.Summary(),
		Description: alert.Description(),
		Labels:      alert.Labels,
	}

	var buf bytes.Buffer
	if err := requestTmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("rendering request template: %w", err)
	}
	return buf.String(), nil
}

func BuildProposal(alert alertmanager.Alert, defaultNamespace, agentName string) (*agentic.Proposal, error) {
	request, err := RenderRequest(alert)
	if err != nil {
		return nil, err
	}

	ns := alert.Namespace()
	if ns == "" {
		ns = defaultNamespace
	}

	name := ProposalName(alert.AlertName(), alert.Namespace(), alert.Fingerprint)

	fp := alert.Fingerprint
	if len(fp) > 8 {
		fp = fp[:8]
	}

	proposal := &agentic.Proposal{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "agentic.openshift.io/v1alpha1",
			Kind:       "Proposal",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			Labels: map[string]string{
				"agentic.openshift.io/source":            "alertmanager",
				"agentic.openshift.io/alert-fingerprint": fp,
				"agentic.openshift.io/alert-name":        strings.ToLower(alert.AlertName()),
				"agentic.openshift.io/alert-severity":    alert.Severity(),
			},
			Annotations: map[string]string{
				"agentic.openshift.io/alert-starts-at": alert.StartsAt.Format(time.RFC3339),
				"agentic.openshift.io/alert-summary":   truncate(alert.Summary(), maxAnnotationValueLength),
			},
		},
		Spec: agentic.ProposalSpec{
			Request: request,
			AnalysisOutput: agentic.AnalysisOutput{
				Mode: agentic.AnalysisOutputModeDefault,
			},
			Analysis:     agentic.ProposalStep{Agent: agentName},
			Execution:    agentic.ProposalStep{Agent: agentName},
			Verification: agentic.ProposalStep{Agent: agentName},
		},
	}

	if alertNS := alert.Namespace(); alertNS != "" {
		proposal.Spec.TargetNamespaces = []string{alertNS}
	}

	return proposal, nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}
