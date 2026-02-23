{{/*
Общие метки для всех ресурсов тестовой среды
*/}}
{{- define "artsore-test.labels" -}}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: artsore
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version }}
{{- end }}

{{/*
Полный URL Docker-образа Admin Module
*/}}
{{- define "artsore-test.amImage" -}}
{{ .Values.registry }}/{{ .Values.amImage }}:{{ .Values.amTag }}
{{- end }}

{{/*
Полный URL Docker-образа SE
*/}}
{{- define "artsore-test.seImage" -}}
{{ .Values.registry }}/{{ .Values.seImage }}:{{ .Values.seTag }}
{{- end }}

{{/*
Keycloak HTTP URL (для AM — внутрикластерная связь)
*/}}
{{- define "artsore-test.keycloakHttpUrl" -}}
http://{{ .Release.Name }}-keycloak.{{ .Values.namespace }}.svc.cluster.local:8080
{{- end }}

{{/*
Keycloak HTTPS URL (для SE — JWKS валидация)
*/}}
{{- define "artsore-test.keycloakHttpsUrl" -}}
https://{{ .Release.Name }}-keycloak.{{ .Values.namespace }}.svc.cluster.local:8443
{{- end }}

{{/*
JWKS URL для SE (Keycloak HTTPS endpoint)
*/}}
{{- define "artsore-test.jwksUrl" -}}
{{ include "artsore-test.keycloakHttpsUrl" . }}/realms/{{ .Values.adminModule.keycloakRealm }}/protocol/openid-connect/certs
{{- end }}

{{/*
Token endpoint (для init job — Client Credentials flow)
*/}}
{{- define "artsore-test.tokenEndpoint" -}}
{{ include "artsore-test.keycloakHttpUrl" . }}/realms/{{ .Values.adminModule.keycloakRealm }}/protocol/openid-connect/token
{{- end }}

{{/*
Admin Module URL (для init job и тестов)
*/}}
{{- define "artsore-test.adminModuleUrl" -}}
https://admin-module.{{ .Values.namespace }}.svc.cluster.local:{{ .Values.adminModule.port }}
{{- end }}

{{/*
Метки selector для Admin Module
*/}}
{{- define "artsore-test.am.selectorLabels" -}}
app.kubernetes.io/name: admin-module
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Метки selector для PostgreSQL
*/}}
{{- define "artsore-test.pg.selectorLabels" -}}
app.kubernetes.io/name: postgresql
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Метки selector для SE экземпляра (принимает имя через контекст)
Использование: include "artsore-test.se.selectorLabels" (dict "name" $instance.name "Release" $.Release)
*/}}
{{- define "artsore-test.se.selectorLabels" -}}
app.kubernetes.io/name: storage-element
app.kubernetes.io/instance: {{ .name }}
{{- end }}
