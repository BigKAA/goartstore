{{/*
Полное имя ресурса: ingester-module
*/}}
{{- define "im.fullname" -}}
ingester-module
{{- end }}

{{/*
Имя chart для label helm.sh/chart
*/}}
{{- define "im.chart" -}}
{{ .Chart.Name }}-{{ .Chart.Version | replace "+" "_" }}
{{- end }}

{{/*
Полный путь к образу контейнера
*/}}
{{- define "im.image" -}}
{{ .Values.registry }}/{{ .Values.image }}:{{ .Values.tag }}
{{- end }}

{{/*
Стандартные Kubernetes labels
*/}}
{{- define "im.labels" -}}
helm.sh/chart: {{ include "im.chart" . }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: artstore
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{ include "im.selectorLabels" . }}
{{- end }}

{{/*
Selector labels для Service → Pod matching
*/}}
{{- define "im.selectorLabels" -}}
app.kubernetes.io/name: ingester-module
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Env-переменные из ConfigMap (не-секретные)
*/}}
{{- define "im.configEnvFrom" -}}
- configMapRef:
    name: {{ include "im.fullname" . }}-config
{{- end }}

{{/*
Env-переменные из Secret (секретные)
*/}}
{{- define "im.secretEnvFrom" -}}
- secretRef:
    name: {{ include "im.fullname" . }}-secret
{{- end }}

{{/*
Volume mounts для TLS CA-сертификата и temp-upload
*/}}
{{- define "im.volumeMounts" -}}
- name: temp-upload
  mountPath: {{ .Values.tempUpload.mountPath }}
{{- if .Values.tls.caSecret }}
- name: ca-certs
  mountPath: /certs
  readOnly: true
{{- end }}
{{- end }}

{{/*
Volumes для TLS CA-сертификата и temp-upload
*/}}
{{- define "im.volumes" -}}
- name: temp-upload
  emptyDir:
    sizeLimit: {{ .Values.tempUpload.sizeLimit }}
{{- if .Values.tls.caSecret }}
- name: ca-certs
  secret:
    secretName: {{ .Values.tls.caSecret }}
{{- end }}
{{- end }}

{{/*
Liveness и readiness probes (HTTP — IM не использует собственный TLS)
*/}}
{{- define "im.probes" -}}
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
