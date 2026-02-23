{{/*
Общие метки для всех ресурсов SE
*/}}
{{- define "artsore-se.labels" -}}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: artsore
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version }}
{{- end }}

{{/*
Полный URL Docker-образа SE
*/}}
{{- define "artsore-se.seImage" -}}
{{ .Values.registry }}/{{ .Values.seImage }}:{{ .Values.seTag }}
{{- end }}

{{/*
Keycloak HTTPS URL (для SE — JWKS валидация)
Имя KC service формируется через infraReleaseName: <infraReleaseName>-keycloak
*/}}
{{- define "artsore-se.keycloakHttpsUrl" -}}
https://{{ .Values.infraReleaseName }}-keycloak.{{ .Values.namespace }}.svc.cluster.local:8443
{{- end }}

{{/*
JWKS URL для SE (Keycloak HTTPS endpoint)
*/}}
{{- define "artsore-se.jwksUrl" -}}
{{ include "artsore-se.keycloakHttpsUrl" . }}/realms/{{ .Values.keycloak.realm }}/protocol/openid-connect/certs
{{- end }}

{{/*
Метки selector для SE экземпляра (принимает имя через контекст)
Использование: include "artsore-se.se.selectorLabels" (dict "name" $instance.name "Release" $.Release)
*/}}
{{- define "artsore-se.se.selectorLabels" -}}
app.kubernetes.io/name: storage-element
app.kubernetes.io/instance: {{ .name }}
{{- end }}
