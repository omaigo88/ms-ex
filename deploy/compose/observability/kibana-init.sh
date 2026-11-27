#!/bin/sh
# Creates (or overwrites) the Kibana data view for the otel-logs index, so
# service logs are browsable in Discover right after `task up-all`.
set -eu

KIBANA=http://kibana:5601

echo "waiting for Kibana..."
until curl -fsS "$KIBANA/api/status" 2>/dev/null | grep -q '"level":"available"'; do
	sleep 3
done

curl -fsS -X POST "$KIBANA/api/data_views/data_view" \
	-H 'kbn-xsrf: true' -H 'Content-Type: application/json' \
	-d '{"override":true,"data_view":{"id":"otel-logs","title":"otel-logs*","name":"otel-logs","timeFieldName":"@timestamp"}}' \
	>/dev/null

echo "data view otel-logs created"
