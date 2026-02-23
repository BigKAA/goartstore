{{/*
Общие метки для всех ресурсов инфраструктурного слоя
*/}}
{{- define "artstore-infra.labels" -}}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: artstore
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version }}
{{- end }}

{{/*
Keycloak HTTP URL (для AM — внутрикластерная связь)
*/}}
{{- define "artstore-infra.keycloakHttpUrl" -}}
http://{{ .Release.Name }}-keycloak.{{ .Values.namespace }}.svc.cluster.local:8080
{{- end }}

{{/*
Keycloak HTTPS URL (для SE — JWKS валидация)
*/}}
{{- define "artstore-infra.keycloakHttpsUrl" -}}
https://{{ .Release.Name }}-keycloak.{{ .Values.namespace }}.svc.cluster.local:8443
{{- end }}

{{/*
Метки selector для PostgreSQL
*/}}
{{- define "artstore-infra.pg.selectorLabels" -}}
app.kubernetes.io/name: postgresql
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Метки selector для Keycloak
*/}}
{{- define "artstore-infra.kc.selectorLabels" -}}
app.kubernetes.io/name: keycloak
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}
