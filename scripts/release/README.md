## DSP Release tools

The scripts found in this folder contain tools utilized for performing a DSP release. 

### Params Generation
This tool will generate a new `params.env` file based on the upcoming DSP tags. 

If images in Red Hat registry have also been updated (e.g. security fixes) without changes to tag version, then the newer 
digests will be used. The following command will generate the `params.env`: 

**Pre-condition**: All DSP/DSPO images should have been build with tag <DSP_TAG>
```
python release.py params --tag v1.2.0 --out_file params.env \
    --override="kube-rbac-proxy=registry.redhat.io/openshift4/ose-kube-rbac-proxy-rhel9@<digest>"
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
