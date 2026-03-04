{{/*
Demo Client — Helm helpers
*/}}

{{/* Полное имя ресурса */}}
{{- define "dc.fullname" -}}
demo-client
{{- end -}}

{{/* Chart label */}}
{{- define "dc.chart" -}}
{{ .Chart.Name }}-{{ .Chart.Version }}
{{- end -}}

{{/* Полный путь к образу */}}
{{- define "dc.image" -}}
{{ .Values.registry }}/{{ .Values.image }}:{{ .Values.tag }}
{{- end -}}

{{/* Стандартные labels */}}
{{- define "dc.labels" -}}
helm.sh/chart: {{ include "dc.chart" . }}
{{ include "dc.selectorLabels" . }}
app.kubernetes.io/version: {{ .Values.tag | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{/* Selector labels */}}
{{- define "dc.selectorLabels" -}}
app.kubernetes.io/name: {{ include "dc.fullname" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{/* ConfigMap env ref */}}
{{- define "dc.configEnvFrom" -}}
- configMapRef:
    name: {{ include "dc.fullname" . }}
{{- end -}}

{{/* Secret env ref */}}
{{- define "dc.secretEnvFrom" -}}
- secretRef:
    name: {{ include "dc.fullname" . }}
{{- end -}}

{{/* Volume mounts для TLS CA cert */}}
{{- define "dc.volumeMounts" -}}
{{- if .Values.tls.caSecret }}
- name: ca-cert
  mountPath: /certs
  readOnly: true
{{- end }}
{{- end -}}

{{/* Volumes для TLS CA cert */}}
{{- define "dc.volumes" -}}
{{- if .Values.tls.caSecret }}
- name: ca-cert
  secret:
    secretName: {{ .Values.tls.caSecret }}
{{- end }}
{{- end -}}

{{/* HTTP probes */}}
{{- define "dc.probes" -}}
livenessProbe:
  httpGet:
    path: /health/live
    port: http
  initialDelaySeconds: 5
  periodSeconds: 10
  timeoutSeconds: 3
  failureThreshold: 3
readinessProbe:
  httpGet:
    path: /health/ready
    port: http
  initialDelaySeconds: 10
  periodSeconds: 10
  timeoutSeconds: 5
  failureThreshold: 3
{{- end -}}
