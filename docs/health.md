# Health endpoint

Unauthenticated `GET /healthz` checks the required datastore with a read-only query and a one-second deadline.
It returns `200` with `{"status":"ok"}` or `503` with `{"status":"unavailable"}`.
Both responses include `Cache-Control: no-store`.

Successful probes produce no request events. Failed probes retain internal errors in request diagnostics.
Probes do not create accounts, audit records, or provider requests.

Local Docker probes use a one-second startup interval, a 30-second startup period, and a 30-second steady interval.
The web and API origins use `/healthz`. The gRPC health contract remains protocol-native.
