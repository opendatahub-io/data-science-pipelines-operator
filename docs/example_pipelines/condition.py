from kfp import dsl
from kfp import components


def random_num(low:int, high:int) -> int:
    """Generate a random number between low and high."""
    import random
    result = random.randint(low, high)
    print(result)
    return result


def flip_coin() -> str:
    """Flip a coin and output heads or tails randomly."""
    import random
    result = 'heads' if random.randint(0, 1) == 0 else 'tails'
    print(result)
    return result


def print_msg(msg: str):
    """Print a message."""
    print(msg)


flip_coin_op = components.create_component_from_func(
    flip_coin, base_image='registry.access.redhat.com/ubi8/python-39')
print_op = components.create_component_from_func(
    print_msg, base_image='registry.access.redhat.com/ubi8/python-39')
random_num_op = components.create_component_from_func(
    random_num, base_image='registry.access.redhat.com/ubi8/python-39')


@dsl.pipeline(
    name='conditional-execution-pipeline',
    description='Shows how to use dsl.Condition().'
)
def flipcoin_pipeline():
    flip = flip_coin_op()
    with dsl.Condition(flip.output == 'heads'):
        random_num_head = random_num_op(0, 9)
        with dsl.Condition(random_num_head.output > 5):
            print_op('heads and %s > 5!' % random_num_head.output)
        with dsl.Condition(random_num_head.output <= 5):
            print_op('heads and %s <= 5!' % random_num_head.output)

    with dsl.Condition(flip.output == 'tails'):
        random_num_tail = random_num_op(10, 19)
        with dsl.Condition(random_num_tail.output > 15):
            print_op('tails and %s > 15!' % random_num_tail.output)
        with dsl.Condition(random_num_tail.output <= 15):
            print_op('tails and %s <= 15!' % random_num_tail.output)


if __name__ == '__main__':
    from kfp_tekton.compiler import TektonCompiler
    from kfp_tekton.compiler.pipeline_utils import TektonPipelineConf
    config = TektonPipelineConf()
    config.set_condition_image_name("registry.access.redhat.com/ubi8/python-39")
    compiler = TektonCompiler()
    compiler._set_pipeline_conf(config)
    compiler.compile(flipcoin_pipeline, __file__.replace('.py', '.yaml'))
