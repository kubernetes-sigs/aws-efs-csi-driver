{{/* vim: set filetype=mustache: */}}
{{/*
Expand the name of the chart.
*/}}
{{- define "aws-efs-csi-driver.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "aws-efs-csi-driver.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "aws-efs-csi-driver.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Common labels
*/}}
{{- define "aws-efs-csi-driver.labels" -}}
app.kubernetes.io/name: {{ include "aws-efs-csi-driver.name" . }}
helm.sh/chart: {{ include "aws-efs-csi-driver.chart" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{/*
Common selector labels
*/}}
{{- define "aws-efs-csi-driver.selectorLabels" -}}
app.kubernetes.io/name: {{ include "aws-efs-csi-driver.name" . }}
{{- if ne .Release.Name "kustomize" }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}
{{- end -}}

{{/*
Create a string out of the map for controller tags flag
*/}}
{{- define "aws-efs-csi-driver.tags" -}}
{{- $tags := list -}}
{{ range $key, $val := . }}
{{- $tags = print $key ":" $val | append $tags -}}
{{- end -}}
{{- join " " $tags -}}
{{- end -}}

{{/*
Create a string out of the map for the controller --extra-tags flag (k1=v1,k2=v2)
*/}}
{{- define "aws-efs-csi-driver.extraTags" -}}
{{- $tags := list -}}
{{- range $key, $val := . -}}
{{- $tags = print $key "=" $val | append $tags -}}
{{- end -}}
{{- join "," $tags -}}
{{- end -}}

{{/*
The controller --cluster-tag-isolation state. The value must be the string
"disable", "backfill" or "enforce".
*/}}
{{- define "aws-efs-csi-driver.clusterTagIsolation" -}}
{{- $state := .Values.controller.clusterTagIsolation -}}
{{- if kindIs "invalid" $state -}}
{{- $state = "disable" -}}
{{- end -}}
{{- if not (kindIs "string" $state) -}}
{{- fail (printf "controller.clusterTagIsolation must be one of \"disable\", \"backfill\" or \"enforce\", got the %s %v" (kindOf $state) $state) -}}
{{- end -}}
{{- if not (has $state (list "" "disable" "backfill" "enforce")) -}}
{{- fail (printf "controller.clusterTagIsolation must be \"disable\", \"backfill\" or \"enforce\", got %q" $state) -}}
{{- end -}}
{{- if eq $state "" -}}
{{- $state = "disable" -}}
{{- end -}}
{{- if and (ne $state "disable") (or (not .Values.controller.clusterName) (not .Values.controller.clusterTagKey)) -}}
{{- fail (printf "controller.clusterTagIsolation=%s requires controller.clusterName and controller.clusterTagKey" $state) -}}
{{- end -}}
{{- $state -}}
{{- end -}}
