from kfp import dsl, compiler

# Edited the compiled version manually, to remove the --trusted-host flag
# this is so we can test for tls certs validations when launcher installs packages
@dsl.component(base_image="quay.io/opendatahub/ds-pipelines-ci-executor-image:v1.0",
packages_to_install=['boto3'],
pip_index_urls=['https://nginx-service.test-pypiserver.svc.cluster.local:443/simple/'])
def say_hello() -> str:
    hello_text = 'Hello!'
    print(hello_text)
    return hello_text


@dsl.pipeline
def hello_pipeline() -> str:
    hello_task = say_hello()
    return hello_task.output


if __name__ == '__main__':
    compiler.Compiler().compile(hello_pipeline, __file__ + '.yaml')
