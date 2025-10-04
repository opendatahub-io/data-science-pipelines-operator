import argparse

from params import generate_params
from version_doc import version_doc


def main():
    parser = argparse.ArgumentParser(
        description="DSP Release Tools."
    )

    subparsers = parser.add_subparsers(help='sub-command help', required=True)

    # Params.env generator inputs
    parser_params = subparsers.add_parser('params', help='Params.env generator inputs')
    parser_params.set_defaults(func=generate_params)
    parser_params.add_argument('--tag', type=str, required=True, help='Tag for which to fetch image digests for.')
    parser_params.add_argument('--quay_org', default="opendatahub", type=str,
                               help='Tag for which to fetch image digests for.')
    parser_params.add_argument('--out_file', default='params.env', type=str, help='File path output for params.env')
    parser_params.add_argument("--ubi-minimal", dest="ubi_minimal_tag", default="8.8",
                        help="ubi-minimal version tag in rh registry")
    parser_params.add_argument("--ubi-micro", dest="ubi_micro_tag", default="8.8",
                        help="ubi-micro version tag in rh registry")
    parser_params.add_argument("--mariadb", dest="mariadb_tag", default="1",
                        help="mariadb version tag in rh registry")
    parser_params.add_argument("--kube-rbac-proxy", dest="kube_rbac_proxy_tag", default="v4.17",
                        help="kube-rbac-proxy version tag in rh registry")

    parser_params.add_argument("--override", dest="overrides",
                               help="Override an env var with a manually submitted digest "
                                    "entry of the form --overide=\"ENV_VAR=DIGEST\". Can be "
                                    "used for multiple entries by using --override multiple times.",
                               action='append')

    # Version Compatibility Matrix doc generator
    parser_vd = subparsers.add_parser('version_doc', help='Version Compatibility Matrix doc generator')
    parser_vd.set_defaults(func=version_doc)
    parser_vd.add_argument('--out_file', default='compatibility.md', type=str, help='File output for markdown doc.')
    parser_vd.add_argument('--input_file', default='compatibility.yaml', type=str,
                           help='Yaml input for compatibility doc generation.')

    args = parser.parse_args()
    args.func(args)


if __name__ == "__main__":
    main()
