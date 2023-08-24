import sys

import requests

QUAY_REPOS = {
    "IMAGES_APISERVER":  "ds-pipelines-api-server",
    "IMAGES_ARTIFACT":  "ds-pipelines-artifact-manager",
    "IMAGES_PERSISTENTAGENT":  "ds-pipelines-persistenceagent",
    "IMAGES_SCHEDULEDWORKFLOW":  "ds-pipelines-scheduledworkflow",
    "IMAGES_MLMDENVOY":  "ds-pipelines-metadata-envoy",
    "IMAGES_MLMDGRPC":  "ds-pipelines-metadata-grpc",
    "IMAGES_MLMDWRITER":  "ds-pipelines-metadata-writer",
    "IMAGES_DSPO":  "data-science-pipelines-operator",
}

ARCH = "amd64"

# RH Registry Env vars
IMAGES_CACHE = "IMAGES_CACHE"
IMAGES_MOVERESULTSIMAGE = "IMAGES_MOVERESULTSIMAGE"
IMAGES_MARIADB = "IMAGES_MARIADB"
IMAGES_OAUTHPROXY = "IMAGES_OAUTHPROXY"

# RH Registry repos
REPO_UBI_MINIMAL = "ubi8/ubi-minimal"
REPO_UBI_MICRO = "ubi8/ubi-micro"
REPO_MARIADB = "rhel8/mariadb-103"
REPO_OAUTH_PROXY = "openshift4/ose-oauth-proxy"

# RH Registry servers
RH_REGISTRY_ACCESS = "registry.access.redhat.com"
RH_REGISTRY_IO = "registry.redhat.io"


def fetch_quay_repo_tag_digest(quay_repo, quay_org, tag):
    api_url = f"https://quay.io/api/v1/repository/{quay_org}/{quay_repo}/tag/?specificTag={tag}"

    response = requests.get(api_url).json()
    tags = response['tags']

    if len(tags) == 0 or 'end_ts' in tags[0]:
        print("Tag does not exist or was deleted.", file=sys.stderr)
        exit(1)
    digest = tags[0].get('manifest_digest')
    if not digest:
        print("Could not find image digest when retrieving image tag.", file=sys.stderr)
        exit(1)
    return digest


def fetch_rh_repo_tag_digest(repo, tag):
    api_url = f"https://catalog.redhat.com/api/containers/v1/repositories/registry/{RH_REGISTRY_ACCESS}/repository/{repo}/tag/{tag}"

    response = requests.get(api_url).json()

    amd_img = {}
    for img in response['data']:
        arch = img.get('architecture')
        if not arch:
            print(f"No 'architecture' field found when fetching image from RH registry.", file=sys.stderr)
            exit(1)
        if img['architecture'] == 'amd64':
            amd_img = img

    if not amd_img:
        print(f"AMD64 arch image not found for repo {repo} and tag {tag}", file=sys.stderr)
        exit(1)

    sha_digest = amd_img['image_id']

    return sha_digest


def generate_params(args):
    tag = args.tag
    quay_org = args.quay_org
    file_out = args.out_file
    ubi_minimal_tag = args.ubi_minimal_tag
    ubi_micro_tag = args.ubi_micro_tag
    mariadb_tag = args.mariadb_tag
    oauth_proxy_tag = args.oauth_proxy_tag

    # Structure: { "ENV_VAR": "IMG_DIGEST",...}
    overrides = {}
    for override in args.overrides:
        entry = override.split('=')
        if len(entry) != 2:
            print("--override values must be of the form var=digest,\n"
                  "e.g: IMAGES_OAUTHPROXY=registry.redhat.io/openshift4/ose-oauth-proxy"
                  "@sha256:ab112105ac37352a2a4916a39d6736f5db6ab4c29bad4467de8d613e80e9bb33", file=sys.stderr)
            exit(1)
        overrides[entry[0]] = entry[1]

    images = []
    # Fetch QUAY Images
    for image_env_var in QUAY_REPOS:
        if image_env_var in overrides:
            images.append(f"{image_env_var}={overrides[image_env_var]}")
        else:
            image_repo = QUAY_REPOS[image_env_var]
            digest = fetch_quay_repo_tag_digest(image_repo, quay_org, tag)
            image_repo_with_digest = f"{image_repo}@{digest}"
            images.append(f"{image_env_var}=quay.io/{quay_org}/{image_repo_with_digest}")

    # Fetch RH Registry images
    rh_registry_images = {
        RH_REGISTRY_ACCESS: [
            {
                "repo": REPO_UBI_MINIMAL,
                "tag": ubi_minimal_tag,
                "env": IMAGES_CACHE
            },
            {
                "repo": REPO_UBI_MICRO,
                "tag": ubi_micro_tag,
                "env": IMAGES_MOVERESULTSIMAGE
            },
        ],
        RH_REGISTRY_IO: [
            {
                "repo": REPO_MARIADB,
                "tag": mariadb_tag,
                "env": IMAGES_MARIADB
            },
            {
                "repo": REPO_OAUTH_PROXY,
                "tag": oauth_proxy_tag,
                "env": IMAGES_OAUTHPROXY
            },
        ]
    }
    for registry in rh_registry_images:
        for img in rh_registry_images[registry]:
            image_env_var, tag, repo = img['env'], img['tag'], img['repo']
            if image_env_var in overrides:
                images.append(f"{image_env_var}={overrides[image_env_var]}")
            else:
                digest = fetch_rh_repo_tag_digest(repo, tag)
                images.append(f"{image_env_var}={registry}/{repo}@{digest}")

    with open(file_out, 'w') as f:
        for images in images:
            f.write(f"{images}\n")
