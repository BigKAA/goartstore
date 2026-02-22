{{/*
Полное имя ресурса: storage-element-{elementId}
*/}}
{{- define "se.fullname" -}}
storage-element-{{ .Values.elementId }}
{{- end }}

{{/*
Имя chart для label helm.sh/chart
*/}}
{{- define "se.chart" -}}
{{ .Chart.Name }}-{{ .Chart.Version | replace "+" "_" }}
{{- end }}

{{/*
Полный путь к образу контейнера
*/}}
{{- define "se.image" -}}
{{ .Values.registry }}/{{ .Values.image }}:{{ .Values.tag }}
{{- end }}

{{/*
Стандартные Kubernetes labels
*/}}
{{- define "se.labels" -}}
helm.sh/chart: {{ include "se.chart" . }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: artsore
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{ include "se.selectorLabels" . }}
{{- end }}

{{/*
Selector labels для Service → Pod matching
*/}}
{{- define "se.selectorLabels" -}}
app.kubernetes.io/name: storage-element
app.kubernetes.io/instance: {{ .Values.elementId }}
{{- end }}

{{/*
Имя TLS Secret (cert-manager или existingSecret)
*/}}
{{- define "se.tlsSecretName" -}}
{{- if .Values.tls.existingSecret -}}
{{ .Values.tls.existingSecret }}
{{- else -}}
{{ include "se.fullname" . }}-tls
{{- end -}}
{{- end }}

{{/*
Имя PVC для data (standalone)
*/}}
{{- define "se.dataPvcName" -}}
{{ include "se.fullname" . }}-data
{{- end }}

{{/*
Имя PVC для WAL (standalone)
*/}}
{{- define "se.walPvcName" -}}
{{ include "se.fullname" . }}-wal
{{- end }}

{{/*
Имя shared PVC для data (replicated, RWX)
*/}}
{{- define "se.sharedDataPvcName" -}}
{{ include "se.fullname" . }}-data-shared
{{- end }}

{{/*
Общие env-переменные SE, используемые в deployment.yaml и statefulset.yaml
*/}}
{{- define "se.envVars" -}}
- name: SE_PORT
  value: {{ .Values.port | quote }}
- name: SE_STORAGE_ID
  value: {{ .Values.elementId | quote }}
- name: SE_DATA_DIR
  value: "/data"
- name: SE_WAL_DIR
  value: "/wal"
- name: SE_MODE
  value: {{ .Values.mode | quote }}
- name: SE_MAX_FILE_SIZE
  value: {{ .Values.maxFileSize | quote }}
- name: SE_GC_INTERVAL
  value: {{ .Values.gcInterval | quote }}
- name: SE_RECONCILE_INTERVAL
  value: {{ .Values.reconcileInterval | quote }}
- name: SE_JWKS_URL
  value: {{ .Values.jwksUrl | quote }}
- name: SE_TLS_CERT
  value: "/certs/tls.crt"
- name: SE_TLS_KEY
  value: "/certs/tls.key"
- name: SE_LOG_LEVEL
  value: {{ .Values.logLevel | quote }}
- name: SE_LOG_FORMAT
  value: {{ .Values.logFormat | quote }}
- name: SE_DEPHEALTH_CHECK_INTERVAL
  value: {{ .Values.dephealthCheckInterval | quote }}
- name: SE_DEPHEALTH_GROUP
  value: {{ .Values.dephealthGroup | quote }}
- name: SE_DEPHEALTH_DEP_NAME
  value: {{ .Values.dephealthDepName | quote }}
{{- end }}

{{/*
Общие volume mounts для data, WAL и TLS сертификатов
*/}}
{{- define "se.volumeMounts" -}}
- name: data
  mountPath: /data
- name: wal
  mountPath: /wal
- name: tls-certs
  mountPath: /certs
  readOnly: true
{{- end }}

{{/*
Liveness и readiness probes (HTTPS)
*/}}
{{- define "se.probes" -}}
livenessProbe:
  httpGet:
    path: /health/live
    port: https
    scheme: HTTPS
  initialDelaySeconds: 15
  periodSeconds: 30
  timeoutSeconds: 5
  failureThreshold: 3
readinessProbe:
  httpGet:
    path: /health/ready
    port: https
    scheme: HTTPS
  initialDelaySeconds: 10
  periodSeconds: 15
  timeoutSeconds: 5
  failureThreshold: 3
{{- end }}
