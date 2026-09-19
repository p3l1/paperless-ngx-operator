#!/bin/sh
# Wraps a generated CRD so Helm can gate it and will not delete it on uninstall.
set -eu

src=$1
dst=$2

{
	echo '{{- if .Values.crds.enabled }}'
	# controller-gen always emits metadata.annotations (its own version marker) right
	# after "metadata:", so this splices into that map rather than adding a second
	# one, which would make metadata an invalid mapping with a duplicate key.
	awk '
		/^  annotations:/ && !done { print; print "    helm.sh/resource-policy: keep"; done = 1; next }
		{ print }
	' "$src"
	echo '{{- end }}'
} > "$dst"
