# Demo TLS Certificates

The `localhost` profile does not use TLS certificates.

The `computercat` profile expects a certificate/key pair on the host and mounts them into `ghttp` via:

- `DEMO_TLS_CERT_FILE`
- `DEMO_TLS_KEY_FILE`

If you do not set those variables, Compose defaults to the existing computercat host paths used in other repos:

- `/media/share/Drive/exchange/certs/computercat/computercat-cert.pem`
- `/media/share/Drive/exchange/certs/computercat/computercat-key.pem`

This directory can still hold local copies if you want to point the env vars here instead.
