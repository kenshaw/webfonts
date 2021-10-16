@font-face {
  font-family: '{{ .family }}';
  font-style: {{ .style }};
  font-weight: {{ .weight }};
{{- if .display }}
  font-display: {{ .display }};
{{- end }}
{{- if .stretch }}
  font-stretch: {{ .stretch }};
{{- end }}
  src: {{ src "  " .paths }};
}
