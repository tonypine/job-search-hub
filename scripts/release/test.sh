#!/bin/sh
# Runs every release script test.

set -u
status=0
for test in "$(dirname "$0")"/*_test.sh; do
	echo "# $(basename "$test")"
	sh "$test" || status=1
done
exit $status
