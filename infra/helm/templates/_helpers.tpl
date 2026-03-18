{{/*
Expand the name of the chart.
*/}}
{{- define "recsys.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "recsys.fullname" -}}
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
Create chart name and version as used by the chart label.
*/}}
{{- define "recsys.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "recsys.labels" -}}
helm.sh/chart: {{ include "recsys.chart" . }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: recsys-pipeline
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}

{{/*
Service-specific labels
*/}}
{{- define "recsys.serviceLabels" -}}
{{ include "recsys.labels" . }}
app.kubernetes.io/name: {{ .serviceName }}
app.kubernetes.io/instance: {{ .Release.Name }}-{{ .serviceName }}
{{- end }}

{{/*
Service-specific selector labels
*/}}
{{- define "recsys.selectorLabels" -}}
app.kubernetes.io/name: {{ .serviceName }}
app.kubernetes.io/instance: {{ .Release.Name }}-{{ .serviceName }}
{{- end }}

{{/*
Create the image reference for a service.
Usage: {{ include "recsys.image" (dict "image" .Values.eventCollector.image "global" .Values.global) }}
*/}}
{{- define "recsys.image" -}}
{{- $registry := .image.registry | default .global.imageRegistry | default "ghcr.io" -}}
{{- $tag := .image.tag | default .global.imageTag | default "latest" -}}
{{- printf "%s/%s:%s" $registry .image.repository $tag -}}
{{- end }}
