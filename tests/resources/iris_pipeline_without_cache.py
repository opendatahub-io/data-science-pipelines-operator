from kfp import compiler, dsl
from kfp.dsl import Dataset, Input, Model, Output

@dsl.component()
def create_dataset(iris_dataset: Output[Dataset]):
    data = """\
Sepal_Length,Sepal_Width,Petal_Length,Petal_Width,Labels
5.1,3.5,1.4,0.2,Iris-setosa
4.9,3.0,1.4,0.2,Iris-setosa
4.7,3.2,1.3,0.2,Iris-setosa
7.0,3.2,4.7,1.4,Iris-versicolor
6.4,3.2,4.5,1.5,Iris-versicolor
6.9,3.1,4.9,1.5,Iris-versicolor
6.3,3.3,6.0,2.5,Iris-virginica
5.8,2.7,5.1,1.9,Iris-virginica
7.1,3.0,5.9,2.1,Iris-virginica
"""
    with open(iris_dataset.path, "w") as f:
        f.write(data)

@dsl.component()
def train_model(
    iris_dataset: Input[Dataset],  # <-- renamed
    model: Output[Model],
):
    with open(model.path, "w") as f:
        f.write("my model")

@dsl.pipeline(name="sample-training-pipeline")
def my_pipeline():
    create_dataset_task = create_dataset().set_caching_options(False)
    train_model(iris_dataset=create_dataset_task.output).set_caching_options(False)  # <-- matching name

if __name__ == "__main__":
    compiler.Compiler().compile(my_pipeline, package_path=__file__.replace(".py", "_compiled.yaml"))
