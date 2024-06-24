# How to create a DSP release

This doc outlines the steps required for manually preparing and performing a DSP release.

Versioning for DSP follows [semver]:

```txt
Given a version number MAJOR.MINOR.PATCH, increment the:

    MAJOR version when you make incompatible API changes
    MINOR version when you add functionality in a backward compatible manner
    PATCH version when you make backward compatible bug fixes
```

DSPO and DSP versioning is tied together, and DSP `MAJOR` versions are tied to [kfp-tekton] upstream.

> Note: In main branch all images should point to `latest` and not any specific versions, as `main` is rapidly moving,
> it is likely to quickly become incompatible with any specific tags/shas that are hardcoded.

## Release pre-requisites

Need GitHub repo admin permissions for DSPO and DSP repos.

### Release workflow

Steps required for performing releases for `MAJOR`, `MINOR`, or `PATCH` vary depending on type.

### MAJOR Releases

Given that `MAJOR` releases often contain large scale, api breaking, changes. It is likely the release process will vary
between each `MAJOR` release. As such, each `MAJOR` release should have a specifically catered strategy.

### MINOR Releases

Let `x.y.z` be the `latest` release that is highest DSPO/DSP version.

Steps on how to release `x.y+1`

1. Ensure `compatibility.yaml` is upto date, and generate a new `compatibility.md`
   - Use [release-tools] to accomplish this
2. Cut branch `vx.y+1` from `main/master`
   - Do this for DSPO and DSP repos
3. Build images. Use the [build-tags] workflow, specifying the branches from above
4. Retrieve the sha images from the resulting workflow (check quay.io for the digests)
   - Using [release-tools] generate a `params.env` and submit a new pr to `vx.y+1` branch
   - For images pulled from registry, ensure latest images are upto date
5. Perform any tests on the branch, confirm stability
   - If issues are found, they should be corrected in `main/master` and be cherry-picked into this branch.
6. Create a tag release (using the branches from above) for `x.y+1.0` in DSPO and DSP (e.g. `v1.3.0`)

## PATCH Releases

DSP supports bug/security fixes for versions that are at most 1 `MINOR` versions behind the latest `MINOR` release.
For example, if `v1.2` is the `latest` DSP release, DSP will backport bugs/security fixes to `v1.1` as `PATCH` (z) releases.

Let `x.y.z` be the `latest` release that is the highest version.\
Let `x.y-1.a` be the highest version release that is one `MINOR` version behind `x.y.z`

**Example**:
If the latest release that is the highest version is `v1.2.0`
Then:

```txt
x.y.z = v1.2.0
x.y-1.a = v1.1.0
vx.y.z+1 = v1.2.1
vx.y-1.a+1 = v1.1.1
```

> Note `a` value in `x.y-1.a` is arbitrarily picked here. It is not always the case `z == a`, though it will likely
> be the case most of the time.

Following along our example, suppose a security bug was found in `main`, `x.y.z`, and `x.y-1.a`.
And suppose that commit `08eb98d` in `main` has resolved this issue.

Then the commit `08eb98d` needs to trickle to `vx.y.z` and `vx.y-1.a` as `PATCH` (z) releases: `vx.y.z+1` and `vx.y-1.a+1`

1. Cherry-pick commit `08eb98d` onto relevant minor branches `vx.y` and `vx.y-1`
2. Build images for `vx.y.z+1` and `vx.y-1.a+1` (e.g. `v1.2.1` and `v1.1.1`) DSPO and DSP
   - Images should be built off the `vx.y` and `vx.y-1` branches respectively
   - Use the [build-tags] workflow
3. Retrieve the sha image digests from the resulting workflow
   - Using [release-tools] generate a params.env and submit a new pr to `vx.y` and `vx.y-1` branches
4. Cut `vx.y.z+1` and `vx.y-1.a+1` releases in DSP and DSPO

### Downstream Specifics

Downstream maintainers of DSP should:

- ensure `odh-stable` branches in DSP/DSPO are upto date with bug/security fixes for the appropriate DSPO/DSP versions,
  and forward any changes from `odh-stable` to their downstream DSPO/DSP repos

[semver]: https://semver.org/
[build-tags]: https://github.com/opendatahub-io/data-science-pipelines-operator/actions/workflows/build-tags.yml
[release-tools]: ../../scripts/release/README.md
