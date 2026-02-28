{{/*
Полное имя ресурса: query-module
*/}}
{{- define "qm.fullname" -}}
query-module
{{- end }}

{{/*
Имя chart для label helm.sh/chart
*/}}
{{- define "qm.chart" -}}
{{ .Chart.Name }}-{{ .Chart.Version | replace "+" "_" }}
{{- end }}

{{/*
Полный путь к образу контейнера
*/}}
{{- define "qm.image" -}}
{{ .Values.registry }}/{{ .Values.image }}:{{ .Values.tag }}
{{- end }}

{{/*
Стандартные Kubernetes labels
*/}}
{{- define "qm.labels" -}}
helm.sh/chart: {{ include "qm.chart" . }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: artstore
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{ include "qm.selectorLabels" . }}
{{- end }}

{{/*
Selector labels для Service → Pod matching
*/}}
{{- define "qm.selectorLabels" -}}
app.kubernetes.io/name: query-module
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Env-переменные из ConfigMap (не-секретные)
*/}}
{{- define "qm.configEnvFrom" -}}
- configMapRef:
    name: {{ include "qm.fullname" . }}-config
{{- end }}

{{/*
Env-переменные из Secret (секретные)
*/}}
{{- define "qm.secretEnvFrom" -}}
- secretRef:
    name: {{ include "qm.fullname" . }}-secret
{{- end }}

{{/*
Volume mounts для TLS CA-сертификата (если caSecret задан)
*/}}
{{- define "qm.volumeMounts" -}}
{{- if .Values.tls.caSecret }}
- name: ca-certs
  mountPath: /certs
  readOnly: true
{{- end }}
{{- end }}

{{/*
Volumes для TLS CA-сертификата (если caSecret задан)
*/}}
{{- define "qm.volumes" -}}
{{- if .Values.tls.caSecret }}
- name: ca-certs
  secret:
    secretName: {{ .Values.tls.caSecret }}
{{- end }}
{{- end }}

{{/*
Liveness и readiness probes (HTTP — QM не использует собственный TLS)
*/}}
{{- define "qm.probes" -}}
livenessProbe:
  httpGet:
    path: /health/live
    port: http
    scheme: HTTP
  initialDelaySeconds: 15
  periodSeconds: 30
  timeoutSeconds: 5
  failureThreshold: 3
readinessProbe:
  httpGet:
    path: /health/ready
    port: http
    scheme: HTTP
  initialDelaySeconds: 10
  periodSeconds: 15
  timeoutSeconds: 5
  failureThreshold: 3
{{- end }}
