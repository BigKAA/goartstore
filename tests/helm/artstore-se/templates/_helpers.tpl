{{/*
Общие метки для всех ресурсов SE
*/}}
{{- define "artstore-se.labels" -}}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: artstore
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version }}
{{- end }}

{{/*
Полный URL Docker-образа SE
*/}}
{{- define "artstore-se.seImage" -}}
{{ .Values.registry }}/{{ .Values.seImage }}:{{ .Values.seTag }}
{{- end }}

{{/*
Keycloak HTTPS URL (для SE — JWKS валидация)
Имя KC service формируется через infraReleaseName: <infraReleaseName>-keycloak
*/}}
{{- define "artstore-se.keycloakHttpsUrl" -}}
https://{{ .Values.infraReleaseName }}-keycloak.{{ .Values.namespace }}.svc.cluster.local:8443
{{- end }}

{{/*
JWKS URL для SE — через внешний API Gateway (эмуляция продакшена)
SE получают JWKS ключи через: SE → artstore.kryukov.lan:443 → Envoy → Keycloak:8080
*/}}
{{- define "artstore-se.jwksUrl" -}}
https://{{ .Values.gateway.hostname }}/realms/{{ .Values.keycloak.realm }}/protocol/openid-connect/certs
{{- end }}

{{/*
Метки selector для SE экземпляра (принимает имя через контекст)
Использование: include "artstore-se.se.selectorLabels" (dict "name" $instance.name "Release" $.Release)
*/}}
{{- define "artstore-se.se.selectorLabels" -}}
app.kubernetes.io/name: storage-element
app.kubernetes.io/instance: {{ .name }}
{{- end }}
