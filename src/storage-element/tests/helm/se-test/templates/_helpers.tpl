{{/*
Общие метки для всех ресурсов тестовой среды
*/}}
{{- define "se-test.labels" -}}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: artsore
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version }}
{{- end }}

{{/*
Полный URL Docker-образа SE
*/}}
{{- define "se-test.seImage" -}}
{{ .Values.registry }}/{{ .Values.seImage }}:{{ .Values.seTag }}
{{- end }}

{{/*
Полный URL Docker-образа JWKS Mock
*/}}
{{- define "se-test.mockImage" -}}
{{ .Values.registry }}/{{ .Values.mockImage }}:{{ .Values.mockTag }}
{{- end }}

{{/*
JWKS URL для всех SE (внутри кластера)
*/}}
{{- define "se-test.jwksUrl" -}}
https://jwks-mock.{{ .Values.namespace }}.svc.cluster.local:{{ .Values.jwksMock.port }}/jwks
{{- end }}

{{/*
Метки selector для JWKS Mock
*/}}
{{- define "se-test.jwksMock.selectorLabels" -}}
app.kubernetes.io/name: jwks-mock
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Метки selector для SE экземпляра (принимает имя через контекст)
Использование: include "se-test.se.selectorLabels" (dict "name" $instance.name "Release" $.Release)
*/}}
{{- define "se-test.se.selectorLabels" -}}
app.kubernetes.io/name: storage-element
app.kubernetes.io/instance: {{ .name }}
{{- end }}
