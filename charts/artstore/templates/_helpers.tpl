{{/*
Общие шаблоны для umbrella chart Artstore.
*/}}

{{/*
Полное имя релиза. Используется для именования общих ресурсов.
*/}}
{{- define "artstore.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Имя chart-а.
*/}}
{{- define "artstore.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Общие labels для всех ресурсов umbrella chart.
*/}}
{{- define "artstore.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: artstore
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- if .Values.global.labels }}
{{ toYaml .Values.global.labels }}
{{- end }}
{{- end }}

{{/*
Namespace для развёртывания.
Использует .Values.global.namespace или .Release.Namespace.
*/}}
{{- define "artstore.namespace" -}}
{{- default .Release.Namespace .Values.global.namespace }}
{{- end }}

{{/*
URL для PostgreSQL — формируется из параметров или используется внешний.
*/}}
{{- define "artstore.postgresql.host" -}}
{{- if .Values.global.postgresql.external }}
{{- .Values.global.postgresql.host }}
{{- else }}
{{- printf "%s-postgresql.%s.svc.cluster.local" .Release.Name (include "artstore.namespace" .) }}
{{- end }}
{{- end }}

{{/*
URL для Keycloak — формируется из параметров или используется внешний.
*/}}
{{- define "artstore.keycloak.url" -}}
{{- if .Values.global.keycloak.external }}
{{- .Values.global.keycloak.url }}
{{- else }}
{{- printf "http://%s-keycloak.%s.svc.cluster.local:8080" .Release.Name (include "artstore.namespace" .) }}
{{- end }}
{{- end }}

{{/*
JWKS URL для JWT-валидации — через Admin Module.
*/}}
{{- define "artstore.jwks.url" -}}
{{- $amName := default "admin-module" .Values.global.adminModuleServiceName }}
{{- printf "https://%s.%s.svc.cluster.local:8000/.well-known/jwks.json" $amName (include "artstore.namespace" .) }}
{{- end }}

{{/*
URL Admin Module API (внутренний, для IM и QM).
*/}}
{{- define "artstore.adminModule.url" -}}
{{- $amName := default "admin-module" .Values.global.adminModuleServiceName }}
{{- printf "https://%s.%s.svc.cluster.local:8000" $amName (include "artstore.namespace" .) }}
{{- end }}

{{/*
Keycloak Token URL для service accounts (OAuth2 Client Credentials).
*/}}
{{- define "artstore.keycloak.tokenUrl" -}}
{{- $kcUrl := include "artstore.keycloak.url" . }}
{{- $realm := .Values.global.keycloak.realm }}
{{- printf "%s/realms/%s/protocol/openid-connect/token" $kcUrl $realm }}
{{- end }}

{{/*
Keycloak Issuer URL для JWT-валидации.
*/}}
{{- define "artstore.keycloak.issuerUrl" -}}
{{- if .Values.global.keycloak.externalUrl }}
{{- printf "%s/realms/%s" .Values.global.keycloak.externalUrl .Values.global.keycloak.realm }}
{{- else }}
{{- $kcUrl := include "artstore.keycloak.url" . }}
{{- printf "%s/realms/%s" $kcUrl .Values.global.keycloak.realm }}
{{- end }}
{{- end }}
