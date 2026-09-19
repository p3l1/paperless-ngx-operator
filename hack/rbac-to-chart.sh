#!/bin/sh
# Splices controller-gen's RBAC rules into the chart's ClusterRole, keeping the
# hand-written leases rule leader election needs alongside them.
set -eu

generated=$1
chart_rbac=$2

{
	sed -n '1,/^  # BEGIN GENERATED/p' "$chart_rbac"
	echo '  - apiGroups: [coordination.k8s.io]'
	echo '    resources: [leases]'
	echo '    verbs: [get, list, watch, create, update, patch, delete]'
	if [ -f "$generated" ]; then
		awk '/^rules:/ { f = 1; next } f' "$generated"
	fi
	sed -n '/^  # END GENERATED/,$p' "$chart_rbac"
} > "$chart_rbac.tmp"
mv "$chart_rbac.tmp" "$chart_rbac"
