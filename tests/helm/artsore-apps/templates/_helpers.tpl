{{/*
Общие метки для всех ресурсов приложений
*/}}
{{- define "artsore-apps.labels" -}}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: artsore
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version }}
{{- end }}

{{/*
Полный URL Docker-образа Admin Module
*/}}
{{- define "artsore-apps.amImage" -}}
{{ .Values.registry }}/{{ .Values.amImage }}:{{ .Values.amTag }}
{{- end }}

{{/*
Keycloak HTTP URL (для AM — внутрикластерная связь)
Имя KC service формируется через infraReleaseName: <infraReleaseName>-keycloak
*/}}
{{- define "artsore-apps.keycloakHttpUrl" -}}
http://{{ .Values.infraReleaseName }}-keycloak.{{ .Values.namespace }}.svc.cluster.local:8080
{{- end }}

{{/*
Admin Module URL (для init job и тестов)
*/}}
{{- define "artsore-apps.adminModuleUrl" -}}
https://admin-module.{{ .Values.namespace }}.svc.cluster.local:{{ .Values.adminModule.port }}
{{- end }}

{{/*
Метки selector для Admin Module
*/}}
{{- define "artsore-apps.am.selectorLabels" -}}
app.kubernetes.io/name: admin-module
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}
