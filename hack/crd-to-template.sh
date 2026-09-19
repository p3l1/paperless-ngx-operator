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

# A silent no-op here would ship a CRD helm uninstall deletes along with users'
# custom resources, so a source CRD missing metadata.annotations fails loudly
# instead of producing a chart file quietly lacking the annotation.
if ! grep -q 'helm.sh/resource-policy: keep' "$dst"; then
	echo "$0: failed to inject helm.sh/resource-policy: keep into $dst" >&2
	echo "(does $src have a metadata.annotations block from controller-gen crd?)" >&2
	rm -f "$dst"
	exit 1
fi
