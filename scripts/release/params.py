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


def fetch_quay_repo_tag_digest(quay_repo, quay_org, tag):
    api_url = "https://quay.io/api/v1/repository/{0}/{1}/tag/?specificTag={2}".format(
        quay_org,
        quay_repo,
        tag
    )
    response = requests.get(api_url).json()
    tags = response['tags']

    if len(tags) == 0 or 'end_ts' in tags[0]:
        print("Tag does not exist or was deleted.", file=sys.stderr)
        exit(1)
    return tags[0]['manifest_digest']


def fetch_rh_repo_tag_digest(repo, tag):
    registry = "registry.access.redhat.com"
    api_url = "https://catalog.redhat.com/api/containers/v1/repositories/registry/{0}/repository/{1}/tag/{2}".format(
        registry,
        repo,
        tag
    )
    response = requests.get(api_url).json()

    amd_img = {}
    for img in response['data']:
        if img['architecture'] == 'amd64':
            amd_img = img

    if not amd_img:
        print("AMD64 arch image not found for repo {0} and tag {1}".format(repo, tag), file=sys.stderr)
        exit(1)

    sha_digest = amd_img['image_id']

    return sha_digest


def params(args):
    tag = args.tag
    quay_org = args.quay_org
    file_out = args.out_file
    ubi_minimal_tag = args.ubi_minimal_tag
    ubi_micro_tag = args.ubi_micro_tag
    mariadb_tag = args.mariadb_tag
    oauth_proxy_tag = args.oauth_proxy_tag

    images = []

    # Fetch QUAY Images
    for image_env_var in QUAY_REPOS:
        image_repo = QUAY_REPOS[image_env_var]
        digest = fetch_quay_repo_tag_digest(image_repo, quay_org, tag)
        image_repo_with_digest = "{0}:{1}".format(image_repo, digest)
        images.append("{0}=quay.io/opendatahub/{1}".format(image_env_var, image_repo_with_digest))

    # Fetch RH Registry images
    repo = "ubi8/ubi-minimal"
    digest = fetch_rh_repo_tag_digest(repo, ubi_minimal_tag)
    images.append("{0}=registry.access.redhat.com/{1}/{2}".format("IMAGES_CACHE", repo, digest))

    repo = "ubi8/ubi-micro"
    digest = fetch_rh_repo_tag_digest(repo, ubi_micro_tag)
    images.append("{0}=registry.access.redhat.com/{1}/{2}".format("IMAGES_MOVERESULTSIMAGE", repo, digest))

    repo = "rhel8/mariadb-103"
    digest = fetch_rh_repo_tag_digest(repo, mariadb_tag)
    images.append("{0}=registry.redhat.io/{1}/{2}".format("IMAGES_MARIADB", repo, digest))

    repo = "openshift4/ose-oauth-proxy"
    digest = fetch_rh_repo_tag_digest(repo, oauth_proxy_tag)
    images.append("{0}=registry.redhat.io/{1}/{2}".format("IMAGES_OAUTHPROXY", repo, digest))

    with open(file_out, 'w') as f:
        for images in images:
            f.write("{0}\n".format(images))
