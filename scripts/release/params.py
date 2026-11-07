import shutil
import subprocess
import sys

import requests


ODH_QUAY_ORG = "opendatahub"

QUAY_REPOS = {
    "IMAGES_DSPO":  "data-science-pipelines-operator",
    "IMAGES_APISERVER":  "ds-pipelines-api-server",
    "IMAGES_PERSISTENCEAGENT":  "ds-pipelines-persistenceagent",
    "IMAGES_SCHEDULEDWORKFLOW":  "ds-pipelines-scheduledworkflow",
    "IMAGES_LAUNCHER":  "ds-pipelines-launcher",
    "IMAGES_DRIVER":  "ds-pipelines-driver",
}

TAGGED_REPOS = {
    "IMAGES_ARGO_WORKFLOWCONTROLLER" : {
        "TAG": "odh-v3.4.17-1",
        "REPO": "ds-pipelines-argo-workflowcontroller"
    },
    "IMAGES_ARGO_EXEC" : {
        "TAG": "odh-v3.4.17-1",
        "REPO": "ds-pipelines-argo-argoexec"
    },
    "IMAGES_MLMDGRPC": {
        "TAG": "main-94ae1e9",
        "REPO": "mlmd-grpc-server"
    },
    "IMAGES_PIPELINES_COMPONENTS": {
        "TAG": "odh-stable",
        "REPO": "odh-pipelines-components",
    },
}

STATIC_REPOS = {
    "IMAGES_MLMDENVOY": "registry.redhat.io/openshift-service-mesh/proxyv2-rhel8@sha256:b30d60cd458133430d4c92bf84911e03cecd02f60e88a58d1c6c003543cf833a",
    "IMAGES_MARIADB": "registry.redhat.io/rhel8/mariadb-103@sha256:f0ee0d27bb784e289f7d88cc8ee0e085ca70e88a5d126562105542f259a1ac01",
}

OTHER_OPTIONS = {
    "ZAP_LOG_LEVEL": "info",
    "MAX_CONCURRENT_RECONCILES": "10",
    "DSPO_HEALTHCHECK_DATABASE_CONNECTIONTIMEOUT": "15s",
    "DSPO_HEALTHCHECK_OBJECTSTORE_CONNECTIONTIMEOUT": "15s",
    "DSPO_REQUEUE_TIME": "20s",
    "DSPO_APISERVER_INCLUDE_OWNERREFERENCE": "true",
    "DSPO_ARGOWORKFLOWSCONTROLLERS": "{\"managementState\":\"Managed\"}",
    "MANAGEDPIPELINES": "\"{}\"",
    "PLATFORMVERSION": "\"v0.0.0\"",
    "FIPSENABLED": "false",
    "WEBHOOK_ANNOTATIONS": "",
}


def fetch_quay_repo_tag_digest(quay_repo, quay_org, tag):
    api_url = f"https://quay.io/api/v1/repository/{quay_org}/{quay_repo}/tag/?specificTag={tag}"

    response = requests.get(api_url).json()
    if 'tags' not in response:
        print(f"Could not fetch tag: {tag} for repo {quay_org}/{quay_repo}. Response: {response}")
        exit(1)

    tags = response['tags']
    if len(tags) == 0 or 'end_ts' in tags[0]:
        print(f"Tag: {tag} for repo {quay_org}/{quay_repo} does not exist or was deleted.", file=sys.stderr)
        exit(1)
    digest = tags[0].get('manifest_digest')
    if not digest:
        print("Could not find image digest when retrieving image tag.", file=sys.stderr)
        exit(1)
    return digest


def fetch_images(repos, overrides, lines, org, tag):
    for image_env_var in repos:
        if image_env_var in overrides:
            lines.append(f"{image_env_var}={overrides[image_env_var]}")
        else:
            image_repo = repos[image_env_var]
            digest = fetch_quay_repo_tag_digest(image_repo, org, tag)
            image_repo_with_digest = f"{image_repo}@{digest}"
            lines.append(f"{image_env_var}=quay.io/{org}/{image_repo_with_digest}")


# The operator projects these flags into kube-rbac-proxy sidecar arguments
# under enforcing adherence; an image that does not recognize a flag exits
# and the sidecar crash-loops.
REQUIRED_KUBE_RBAC_PROXY_TLS_FLAGS = (
    "--tls-min-version",
    "--tls-cipher-suites",
    "--tls-curve-preferences",
)


def verify_kube_rbac_proxy_tls_flags(image_ref, skip=False):
    """Verify the selected kube-rbac-proxy image supports the TLS flags the
    operator projects into its sidecars, by running the image's --help.
    Verification is required before a release is generated; it can only be
    bypassed through the explicit --skip-proxy-verification flag."""
    if skip:
        print("WARNING: kube-rbac-proxy TLS flag verification skipped by "
              f"--skip-proxy-verification for {image_ref}; this explicitly "
              "accepts the risk of an image that cannot serve the projected "
              "TLS arguments.", file=sys.stderr)
        return
    runtime = next((r for r in ("podman", "docker") if shutil.which(r)), None)
    if runtime is None:
        print("ERROR: no container runtime (podman or docker) found; the "
              "kube-rbac-proxy image cannot be verified. The verification "
              "must run before a release is generated, because an image "
              "without the projected TLS flags crash-loops proxy sidecars. "
              "Install a container runtime, or explicitly accept the risk "
              "with --skip-proxy-verification.", file=sys.stderr)
        exit(1)
    # S603: list-form invocation without a shell; the only variable input is
    # the image reference selected by the release engineer.
    result = subprocess.run(  # noqa: S603
        [runtime, "run", "--rm", "--network=none", "--platform", "linux/amd64",
         image_ref, "--help"],
        capture_output=True,
        text=True,
        check=False,
    )
    if result.returncode != 0:
        print(f"ERROR: could not run {image_ref} with --help "
              f"(exit {result.returncode}): {result.stderr.strip()}", file=sys.stderr)
        exit(1)
    help_text = result.stdout + result.stderr
    missing = [f for f in REQUIRED_KUBE_RBAC_PROXY_TLS_FLAGS if f not in help_text]
    if missing:
        print(f"ERROR: kube-rbac-proxy image {image_ref} does not support required TLS "
              f"flags: {', '.join(missing)}. The operator projects these flags into "
              "proxy sidecars and an unknown flag crash-loops the sidecar. Use an "
              "image that supports them (odh-kube-rbac-proxy v3.6.0 or later).",
              file=sys.stderr)
        exit(1)
    print(f"Verified kube-rbac-proxy image {image_ref} supports the projected TLS flags.")


def static_vars(values, overrides, lines):
    for var in values:
        if var in overrides:
            lines.append(f"{var}={overrides[var]}")
        else:
            value = values[var]
            lines.append(f"{var}={value}")


def generate_params(args):
    tag = args.tag
    quay_org = args.quay_org
    file_out = args.out_file

    # Structure: { "ENV_VAR": "IMG_DIGEST",...}
    overrides = {}
    if args.overrides:
        for override in args.overrides:
            entry = override.split('=')
            if len(entry) != 2:
                print("--override values must be of the form var=digest,\n"
                      "e.g: kube-rbac-proxy=quay.io/opendatahub/odh-kube-rbac-proxy"
                      "@sha256:fd4d1d59dbba435a4c6315fcb18493c6a887bdb205491b5355e5ce6342ae4ad2", file=sys.stderr)
                exit(1)
            overrides[entry[0]] = entry[1]

    env_var_lines = []

    fetch_images(QUAY_REPOS, overrides, env_var_lines, quay_org, tag)
    for image in TAGGED_REPOS:
        target_repo = {image: TAGGED_REPOS[image]["REPO"]}
        target_tag = TAGGED_REPOS[image]["TAG"]
        fetch_images(target_repo, overrides, env_var_lines, quay_org, target_tag)

    static_vars(STATIC_REPOS, overrides, env_var_lines)
    fetch_images(
        {"kube-rbac-proxy": "odh-kube-rbac-proxy"},
        overrides,
        env_var_lines,
        ODH_QUAY_ORG,
        args.kube_rbac_proxy_tag,
    )
    proxy_ref = next(
        line.split("=", 1)[1] for line in env_var_lines
        if line.startswith("kube-rbac-proxy=")
    )
    verify_kube_rbac_proxy_tls_flags(proxy_ref, skip=args.skip_proxy_verification)
    static_vars(OTHER_OPTIONS, overrides, env_var_lines)

    with open(file_out, 'w') as f:
        for env_var_lines in env_var_lines:
            f.write(f"{env_var_lines}\n")
