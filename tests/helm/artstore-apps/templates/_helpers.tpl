{{/*
Общие метки для всех ресурсов приложений
*/}}
{{- define "artstore-apps.labels" -}}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: artstore
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version }}
{{- end }}

{{/*
Полный URL Docker-образа Admin Module
*/}}
{{- define "artstore-apps.amImage" -}}
{{ .Values.registry }}/{{ .Values.amImage }}:{{ .Values.amTag }}
{{- end }}

{{/*
Keycloak HTTP URL (для AM — внутрикластерная связь)
Имя KC service формируется через infraReleaseName: <infraReleaseName>-keycloak
*/}}
{{- define "artstore-apps.keycloakHttpUrl" -}}
http://{{ .Values.infraReleaseName }}-keycloak.{{ .Values.namespace }}.svc.cluster.local:8080
{{- end }}

{{/*
Admin Module URL (для init job и тестов)
*/}}
{{- define "artstore-apps.adminModuleUrl" -}}
https://admin-module.{{ .Values.namespace }}.svc.cluster.local:{{ .Values.adminModule.port }}
{{- end }}

{{/*
Метки selector для Admin Module
*/}}
{{- define "artstore-apps.am.selectorLabels" -}}
app.kubernetes.io/name: admin-module
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Полный URL Docker-образа Query Module
*/}}
{{- define "artstore-apps.qmImage" -}}
{{ .Values.registry }}/{{ .Values.qmImage }}:{{ .Values.qmTag }}
{{- end }}

{{/*
Метки selector для Query Module
*/}}
{{- define "artstore-apps.qm.selectorLabels" -}}
app.kubernetes.io/name: query-module
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Полный URL Docker-образа Ingester Module
*/}}
{{- define "artstore-apps.imImage" -}}
{{ .Values.registry }}/{{ .Values.imImage }}:{{ .Values.imTag }}
{{- end }}

{{/*
Метки selector для Ingester Module
*/}}
{{- define "artstore-apps.im.selectorLabels" -}}
app.kubernetes.io/name: ingester-module
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}
