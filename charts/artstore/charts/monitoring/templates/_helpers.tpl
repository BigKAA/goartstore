{{/*
Monitoring subchart — вспомогательные шаблоны
*/}}

{{/*
Полное имя ресурса с учётом релиза
*/}}
{{- define "monitoring.fullname" -}}
{{- printf "%s-monitoring" .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Общие labels
*/}}
{{- define "monitoring.labels" -}}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: artstore
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version }}
{{- end -}}

{{/*
Labels для Prometheus
*/}}
{{- define "monitoring.prometheus.labels" -}}
{{ include "monitoring.labels" . }}
app.kubernetes.io/name: prometheus
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: monitoring
{{- end -}}

{{/*
Selector labels для Prometheus
*/}}
{{- define "monitoring.prometheus.selectorLabels" -}}
app.kubernetes.io/name: prometheus
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{/*
Labels для Grafana
*/}}
{{- define "monitoring.grafana.labels" -}}
{{ include "monitoring.labels" . }}
app.kubernetes.io/name: grafana
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: monitoring
{{- end -}}

{{/*
Selector labels для Grafana
*/}}
{{- define "monitoring.grafana.selectorLabels" -}}
app.kubernetes.io/name: grafana
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}
