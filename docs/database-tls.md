# Database TLS configuration

Configure the pipeline server database connection with
`spec.database.customExtraParams`.

## Verified TLS

Use `tls=true` to require TLS with certificate verification:

```yaml
spec:
  database:
    customExtraParams: |
      {"tls":"true"}
```

For a database certificate signed by a self-signed or internal CA, create a
ConfigMap containing the CA certificate and reference it from the DSPA:

```yaml
spec:
  apiServer:
    cABundle:
      configMapName: database-ca
      configMapKey: ca.crt
```

## Verification bypass modes

The `skip-verify` and `preferred` modes remain supported for compatibility, but
they do not provide reliable security. `skip-verify` disables certificate
verification and `preferred` can fall back to an unencrypted connection. The
operator logs a warning when either mode is used.

Use the JSON string value `"true"` for verified TLS:

```json
{"tls":"true"}
```

Configure `spec.apiServer.cABundle` when the database CA is not already trusted
by the system.

The `false` mode remains available for an intentionally unencrypted database
connection. It should not be used when transport encryption is required.

The TLS scanner finding for these compatibility modes is handled through the
approved security exception process. See the [TLS workflow guidance](https://github.com/opendatahub-io/security-config/tree/main#tls-workflows).
