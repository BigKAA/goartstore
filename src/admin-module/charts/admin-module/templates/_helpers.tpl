{{/*
Полное имя ресурса: admin-module
*/}}
{{- define "am.fullname" -}}
admin-module
{{- end }}

{{/*
Имя chart для label helm.sh/chart
*/}}
{{- define "am.chart" -}}
{{ .Chart.Name }}-{{ .Chart.Version | replace "+" "_" }}
{{- end }}

{{/*
Полный путь к образу контейнера
*/}}
{{- define "am.image" -}}
{{ .Values.registry }}/{{ .Values.image }}:{{ .Values.tag }}
{{- end }}

{{/*
Стандартные Kubernetes labels
*/}}
{{- define "am.labels" -}}
helm.sh/chart: {{ include "am.chart" . }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: artsore
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{ include "am.selectorLabels" . }}
{{- end }}

{{/*
Selector labels для Service → Pod matching
*/}}
{{- define "am.selectorLabels" -}}
app.kubernetes.io/name: admin-module
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Env-переменные из ConfigMap (не-секретные)
*/}}
{{- define "am.configEnvFrom" -}}
- configMapRef:
    name: {{ include "am.fullname" . }}-config
{{- end }}

{{/*
Env-переменные из Secret (секретные)
*/}}
{{- define "am.secretEnvFrom" -}}
- secretRef:
    name: {{ include "am.fullname" . }}-secret
{{- end }}

{{/*
Volume mounts для TLS CA-сертификата (если caSecret задан)
*/}}
{{- define "am.volumeMounts" -}}
{{- if .Values.tls.caSecret }}
- name: ca-certs
  mountPath: /certs
  readOnly: true
{{- end }}
{{- end }}

{{/*
Volumes для TLS CA-сертификата (если caSecret задан)
*/}}
{{- define "am.volumes" -}}
{{- if .Values.tls.caSecret }}
- name: ca-certs
  secret:
    secretName: {{ .Values.tls.caSecret }}
{{- end }}
{{- end }}

{{/*
Liveness и readiness probes (HTTP — AM не использует собственный TLS)
*/}}
{{- define "am.probes" -}}
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
