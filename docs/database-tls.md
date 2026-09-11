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

## Migration from verification bypass modes

The `skip-verify` and `preferred` modes are rejected because they bypass
certificate verification. Before upgrading, replace either mode with the JSON
string value `"true"`:

```json
{"tls":"true"}
```

Configure `spec.apiServer.cABundle` when the database CA is not already trusted
by the system.

Existing pipeline server workloads are not rewritten automatically because
the operator cannot infer the correct CA certificate. A DSPA that still uses
one of the rejected modes will fail reconciliation until its configuration is
updated.

The `false` mode remains available for an intentionally unencrypted database
connection. It should not be used when transport encryption is required.
