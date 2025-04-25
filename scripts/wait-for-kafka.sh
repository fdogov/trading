#!/bin/sh
# wait-for-kafka.sh (для Redpanda)

set -e

host="$1"
shift
cmd="$@"

until nc -z "$host" 9092; do
  >&2 echo "Redpanda is unavailable - sleeping"
  sleep 1
done

>&2 echo "Redpanda is up - executing command"
exec $cmd
