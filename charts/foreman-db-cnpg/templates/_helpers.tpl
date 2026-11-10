{{/*
Cluster resource name.
*/}}
{{- define "foreman-db-cnpg.name" -}}
{{- default .Release.Name .Values.name | trunc 50 | trimSuffix "-" -}}
{{- end -}}
