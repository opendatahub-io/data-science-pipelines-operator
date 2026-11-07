## DSP Release tools

The scripts found in this folder contain tools utilized for performing a DSP release. 

### Params Generation
This tool will generate a new `params.env` file based on the upcoming DSP tags. 

If a tag-resolved image is rebuilt without changing its tag, the generated `params.env` records the current digest; images pinned by digest in the script (MLMD Envoy, MariaDB) keep their recorded digest. The following
command generates the `params.env` with the compatible ODH kube-rbac-proxy version:

Before writing the file, the selected kube-rbac-proxy image is verified: the script runs
the image's `--help` through podman or docker (`--network=none`) and fails when it does not
support the TLS flags the operator projects into proxy sidecars (`--tls-min-version`,
`--tls-cipher-suites`, `--tls-curve-preferences`). Verification is required: generation
fails when no container runtime is available. To explicitly accept the risk of an
unverified image, pass `--skip-proxy-verification`.
v3.5.0 and older lack `--tls-curve-preferences`; use v3.6.0 or later.

**Pre-condition**: All DSP/DSPO images should have been build with tag <DSP_TAG>
```
python release.py params --tag v1.2.0 --out_file params.env \
    --kube-rbac-proxy v3.6.0
```

See `--help` for more options like specifying tags for images not tied to DSP (ubi, mariadb, kube-rbac-proxy, etc.)

### Compatibility Doc generation
Before each release, ensure that the [compatibility doc] is upto date. This doc is auto generated, the version compatibility 
is pulled from the [compatibility yaml]. The yaml should be kept upto date by developers (manual).

To generate the version doc run the following: 

**Pre-condition**: ensure that [compatibility yaml] has an entry for the latest DSP version to be released, with version 
compatibility up to date.

```
cd scripts/release
python release.py version_doc --input_file ../../docs/release/compatibility.yaml --out_file ../../docs/release/compatibility.md
```


[compatibility doc]: ../../docs/release/compatibility.md
[compatibility yaml]: ../../docs/release/compatibility.yaml
