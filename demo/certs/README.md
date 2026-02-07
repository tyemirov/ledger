# Demo TLS Certificates

The demo Compose stack expects a TLS certificate/key pair for the HTTPS UI entrypoint.

Place the following files in this directory (they are gitignored):

- `computercat-cert.pem`
- `computercat-key.pem`

These are mounted into the `ghttp` container and used with `--tls-cert` / `--tls-key`.

Note: the `ghttp` container does not run as root, so the private key must be readable by the container user (for example `chmod 0644 computercat-key.pem`).
