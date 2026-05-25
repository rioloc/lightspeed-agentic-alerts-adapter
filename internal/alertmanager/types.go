package alertmanager

import "time"

type AlertStatus struct {
	State string `json:"state"`
}

type Alert struct {
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	StartsAt    time.Time         `json:"startsAt"`
	EndsAt      time.Time         `json:"endsAt"`
	Fingerprint string            `json:"fingerprint"`
	Status      AlertStatus       `json:"status"`
}

func (a Alert) AlertName() string {
	return a.Labels["alertname"]
}

func (a Alert) Severity() string {
	return a.Labels["severity"]
}

func (a Alert) Namespace() string {
	return a.Labels["namespace"]
}

func (a Alert) Summary() string {
	return a.Annotations["summary"]
}

func (a Alert) Description() string {
	return a.Annotations["description"]
}
