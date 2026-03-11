{{- define "bering.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "bering.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name (include "bering.name" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}

{{- define "bering.labels" -}}
app.kubernetes.io/name: {{ include "bering.name" . }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/version: {{ default .Chart.AppVersion .Values.image.tag | quote }}
{{- end -}}

{{- define "bering.selectorLabels" -}}
app.kubernetes.io/name: {{ include "bering.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "bering.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "bering.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{- define "bering.image" -}}
{{- $repo := .Values.image.repository -}}
{{- if .Values.image.digest -}}
{{- printf "%s@%s" $repo .Values.image.digest -}}
{{- else -}}
{{- printf "%s:%s" $repo (default .Chart.AppVersion .Values.image.tag) -}}
{{- end -}}
{{- end -}}

{{- define "bering.configMapName" -}}
{{- if .Values.config.existingConfigMap -}}
{{- .Values.config.existingConfigMap -}}
{{- else -}}
{{- printf "%s-config" (include "bering.fullname" .) -}}
{{- end -}}
{{- end -}}

{{- define "bering.httpListenAddress" -}}
{{- default (printf ":%v" .Values.service.ports.http.targetPort) .Values.config.server.listenAddress -}}
{{- end -}}

{{- define "bering.grpcListenAddress" -}}
{{- if .Values.service.ports.grpc.enabled -}}
{{- default (printf ":%v" .Values.service.ports.grpc.targetPort) .Values.config.server.grpcListenAddress -}}
{{- else -}}
{{- default "" .Values.config.server.grpcListenAddress -}}
{{- end -}}
{{- end -}}
