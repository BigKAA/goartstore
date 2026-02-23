{{/*
Общие метки для всех ресурсов инфраструктурного слоя
*/}}
{{- define "artsore-infra.labels" -}}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: artsore
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version }}
{{- end }}

{{/*
Keycloak HTTP URL (для AM — внутрикластерная связь)
Bitnami KC создаёт service <release>-keycloak
*/}}
{{- define "artsore-infra.keycloakHttpUrl" -}}
http://{{ .Release.Name }}-keycloak.{{ .Values.namespace }}.svc.cluster.local:8080
{{- end }}

{{/*
Keycloak HTTPS URL (для SE — JWKS валидация)
*/}}
{{- define "artsore-infra.keycloakHttpsUrl" -}}
https://{{ .Release.Name }}-keycloak.{{ .Values.namespace }}.svc.cluster.local:8443
{{- end }}

{{/*
Метки selector для PostgreSQL
*/}}
{{- define "artsore-infra.pg.selectorLabels" -}}
app.kubernetes.io/name: postgresql
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}
