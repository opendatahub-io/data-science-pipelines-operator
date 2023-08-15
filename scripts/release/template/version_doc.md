# DSP Version Compatibility Table

This is an auto generated DSP version compatibility table.
Each row outlines the versions for individual subcomponents and images that are leveraged within DSP.

For some components, the versions match with their respective image tags within their respective Quay, GCR, or RedHat image
registries, this is true for the following:

* [ml-metadata]
* [envoy]
* [oauth-proxy]
  * for Oauth Proxy DSP follows the same version digest as the Oauth Proxy leveraged within the rest of ODH.
* [mariaDB]
  * for MariaDB the entire column represents different tag versions for MariDB Version 10.3, DSP follows the latest digest for the `1` tag
    for each DSP release.
* [ubi-minimal]
  * Used for default base images during Pipeline Runs
* [ubi-micro]
  * Used for default cache image for runs


<<TABLE>>


[ml-metadata]: https://github.com/opendatahub-io/data-science-pipelines/blob/master/third-party/ml-metadata/Dockerfile#L15
[envoy]: https://github.com/opendatahub-io/data-science-pipelines/blob/master/third-party/metadata_envoy/Dockerfile#L15
[oauth-proxy]: https://catalog.redhat.com/software/containers/openshift4/ose-oauth-proxy/5cdb2133bed8bd5717d5ae64?tag=v4.13.0-202307271338.p0.g44af5a3.assembly.stream&push_date=1691493453000
[mariaDB]: https://catalog.redhat.com/software/containers/rhel8/mariadb-103/5ba0acf2d70cc57b0d1d9e78
[ubi-minimal]: https://catalog.redhat.com/software/containers/ubi8/ubi-minimal/5c359a62bed8bd75a2c3fba8?architecture=amd64&tag=8.8
[ubi-micro]: https://catalog.redhat.com/software/containers/ubi8-micro/601a84aadd19c7786c47c8ea?architecture=amd64&tag=8.8
