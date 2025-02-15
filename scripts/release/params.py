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
    "IMAGES_PIPELINESRUNTIMEGENERIC":  "ds-pipelines-runtime-generic",
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
}

STATIC_REPOS = {
    "IMAGES_MLMDENVOY": "registry.redhat.io/openshift-service-mesh/proxyv2-rhel8@sha256:b30d60cd458133430d4c92bf84911e03cecd02f60e88a58d1c6c003543cf833a",
    "IMAGES_MARIADB": "registry.redhat.io/rhel8/mariadb-103@sha256:f0ee0d27bb784e289f7d88cc8ee0e085ca70e88a5d126562105542f259a1ac01",
    "IMAGES_OAUTHPROXY": "registry.redhat.io/openshift4/ose-oauth-proxy@sha256:8ce44de8c683f198bf24ba36cd17e89708153d11f5b42c0a27e77f8fdb233551",
    "IMAGES_TOOLBOX": "registry.redhat.io/ubi9/toolbox@sha256:da31dee8904a535d12689346e65e5b00d11a6179abf1fa69b548dbd755fa2770",
    "IMAGES_RHELAI": "registry.redhat.io/rhelai1/instructlab-nvidia-rhel9@sha256:05cfba1fb13ed54b1de4d021da2a31dd78ba7d8cc48e10c7fe372815899a18ae",
}

OTHER_OPTIONS = {
    "ZAP_LOG_LEVEL": "info",
    "MAX_CONCURRENT_RECONCILES": "10",
    "DSPO_HEALTHCHECK_DATABASE_CONNECTIONTIMEOUT": "15s",
    "DSPO_HEALTHCHECK_OBJECTSTORE_CONNECTIONTIMEOUT": "15s",
    "DSPO_REQUEUE_TIME": "20s",
    "DSPO_APISERVER_INCLUDE_OWNERREFERENCE": "true",
    "MANAGEDPIPELINES": "\"{}\"",
    "PLATFORMVERSION": "\"v0.0.0\""
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
                      "e.g: IMAGES_OAUTHPROXY=registry.redhat.io/openshift4/ose-oauth-proxy"
                      "@sha256:ab112105ac37352a2a4916a39d6736f5db6ab4c29bad4467de8d613e80e9bb33", file=sys.stderr)
                exit(1)
            overrides[entry[0]] = entry[1]

    env_var_lines = []

    fetch_images(QUAY_REPOS, overrides, env_var_lines, quay_org, tag)
    for image in TAGGED_REPOS:
        target_repo = {image: TAGGED_REPOS[image]["REPO"]}
        target_tag = TAGGED_REPOS[image]["TAG"]
        fetch_images(target_repo, overrides, env_var_lines, quay_org, target_tag)

    static_vars(STATIC_REPOS, overrides, env_var_lines)
    static_vars(OTHER_OPTIONS, overrides, env_var_lines)

    with open(file_out, 'w') as f:
        for env_var_lines in env_var_lines:
            f.write(f"{env_var_lines}\n")
