#!/usr/bin/env bash
# Post-deploy smoke test: verifies the compose stack actually serves traffic
# after `docker compose up`, with no manual step — the C3.4 "mise en
# production automatisée, sans intervention manuelle" criterion needs an
# artifact that isn't just "the dashboard looks fine", it needs a script
# that fails the pipeline if the stack is actually broken.
set -euo pipefail

API_URL="${API_URL:-http://localhost:8080}"
MAX_WAIT_SECONDS=60

echo "Waiting for API to become healthy at ${API_URL}/health ..."
elapsed=0
until curl -sf "${API_URL}/health" > /dev/null; do
  if (( elapsed >= MAX_WAIT_SECONDS )); then
    echo "::error::API did not become healthy within ${MAX_WAIT_SECONDS}s"
    exit 1
  fi
  sleep 2
  elapsed=$((elapsed + 2))
done
echo "API healthy after ${elapsed}s"

echo "Exercising register+login end-to-end against the deployed stack ..."
# Do this *before* checking /metrics: streampulse_auth_logins_total and
# streampulse_auth_registrations_total are CounterVecs — client_golang
# emits no HELP/TYPE line for a Vec metric until at least one label
# combination has actually been observed, so hitting /metrics cold
# would report them as "missing" even though the deploy is fine.
STAMP="$(date +%s 2>/dev/null || echo static)"
EMAIL="smoke-${STAMP}@example.test"
REGISTER_STATUS=$(curl -s -o /dev/null -w '%{http_code}' -X POST "${API_URL}/api/v1/auth/register" \
  -H 'Content-Type: application/json' \
  -d "{\"email\":\"${EMAIL}\",\"username\":\"smoke-${STAMP}\",\"password\":\"SmokeTest1234!\"}")
if [[ "${REGISTER_STATUS}" != "201" ]]; then
  echo "::error::Register returned ${REGISTER_STATUS}, expected 201"
  exit 1
fi
echo "  ok: register -> ${REGISTER_STATUS}"

LOGIN_STATUS=$(curl -s -o /dev/null -w '%{http_code}' -X POST "${API_URL}/api/v1/auth/login" \
  -H 'Content-Type: application/json' \
  -d "{\"email\":\"${EMAIL}\",\"password\":\"SmokeTest1234!\"}")
if [[ "${LOGIN_STATUS}" != "200" ]]; then
  echo "::error::Login returned ${LOGIN_STATUS}, expected 200"
  exit 1
fi
echo "  ok: login -> ${LOGIN_STATUS}"

echo "Checking /metrics exposes the auth business counters ..."
METRICS=$(curl -sf "${API_URL}/metrics")
for metric in streampulse_auth_logins_total streampulse_auth_registrations_total streampulse_http_requests_total; do
  if ! grep -q "^# HELP ${metric} " <<< "${METRICS}"; then
    echo "::error::Expected metric ${metric} missing from /metrics — deploy is up but unobservable"
    exit 1
  fi
  echo "  ok: ${metric}"
done

echo "Smoke test passed: deployed stack is healthy, observable, and functional."
