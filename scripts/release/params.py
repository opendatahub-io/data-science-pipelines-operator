import sys

import requests

V1_TAG = "v1.6.4"

ODH_QUAY_ORG = "opendatahub"

QUAY_REPOS_V1 = {
    "IMAGES_APISERVER":  "ds-pipelines-api-server",
    "IMAGES_ARTIFACT":  "ds-pipelines-artifact-manager",
    "IMAGES_PERSISTENTAGENT":  "ds-pipelines-persistenceagent",
    "IMAGES_SCHEDULEDWORKFLOW":  "ds-pipelines-scheduledworkflow",
    "IMAGES_MLMDENVOY":  "ds-pipelines-metadata-envoy",
    "IMAGES_MLMDGRPC":  "ds-pipelines-metadata-grpc",
    "IMAGES_MLMDWRITER":  "ds-pipelines-metadata-writer",
}

QUAY_REPOS_V2 = {
    "IMAGES_DSPO":  "data-science-pipelines-operator",
    "V2_LAUNCHER_IMAGE":  "ds-pipelines-launcher",
    "V2_DRIVER_IMAGE":  "ds-pipelines-driver",
    "IMAGESV2_ARGO_APISERVER":  "ds-pipelines-api-server",
    "IMAGESV2_ARGO_PERSISTENCEAGENT":  "ds-pipelines-persistenceagent",
    "IMAGESV2_ARGO_SCHEDULEDWORKFLOW":  "ds-pipelines-scheduledworkflow",
}

TAGGED_REPOS = {
    "IMAGESV2_ARGO_WORKFLOWCONTROLLER" : {
        "TAG": "3.3.10-upstream",
        "REPO": "ds-pipelines-argo-workflowcontroller"
    },
    "IMAGESV2_ARGO_ARGOEXEC" : {
        "TAG": "3.3.10-upstream",
        "REPO": "ds-pipelines-argo-argoexec"
    },
    "IMAGESV2_ARGO_MLMDGRPC": {
        "TAG": "main-94ae1e9",
        "REPO": "mlmd-grpc-server"
    },
}

STATIC_REPOS = {
    "IMAGESV2_ARGO_MLMDENVOY": "registry.redhat.io/openshift-service-mesh/proxyv2-rhel8@sha256:a744c1b386fd5e4f94e43543e829df1bfdd1b564137917372a11da06872f4bcb",
    "IMAGES_MARIADB": "registry.redhat.io/rhel8/mariadb-103@sha256:3d30992e60774f887c4e7959c81b0c41b0d82d042250b3b56f05ab67fd4cdee1",
    "IMAGES_OAUTHPROXY": "registry.redhat.io/openshift4/ose-oauth-proxy@sha256:ab112105ac37352a2a4916a39d6736f5db6ab4c29bad4467de8d613e80e9bb33",
    "IMAGES_CACHE": "registry.redhat.io/ubi8/ubi-minimal@sha256:5d2d4d4dbec470f8ffb679915e2a8ae25ad754cd9193fa966deee1ecb7b3ee00",
    "IMAGES_MOVERESULTSIMAGE": "registry.redhat.io/ubi8/ubi-micro@sha256:396baed3d689157d96aa7d8988fdfea7eb36684c8335eb391cf1952573e689c1",
}

OTHER_OPTIONS = {
    "ZAP_LOG_LEVEL": "info",
    "MAX_CONCURRENT_RECONCILES": "10",
    "DSPO_HEALTHCHECK_DATABASE_CONNECTIONTIMEOUT": "15s",
    "DSPO_HEALTHCHECK_OBJECTSTORE_CONNECTIONTIMEOUT": "15s",
    "DSPO_REQUEUE_TIME": "20s",
    "DSPO_APISERVER_INCLUDE_OWNERREFERENCE": "true"
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

    fetch_images(QUAY_REPOS_V1, overrides, env_var_lines, quay_org, V1_TAG)
    fetch_images(QUAY_REPOS_V2, overrides, env_var_lines, quay_org, tag)
    for image in TAGGED_REPOS:
        target_repo = {image: TAGGED_REPOS[image]["REPO"]}
        target_tag = TAGGED_REPOS[image]["TAG"]
        fetch_images(target_repo, overrides, env_var_lines, quay_org, target_tag)

    static_vars(STATIC_REPOS, overrides, env_var_lines)
    static_vars(OTHER_OPTIONS, overrides, env_var_lines)

    with open(file_out, 'w') as f:
        for env_var_lines in env_var_lines:
            f.write(f"{env_var_lines}\n")
