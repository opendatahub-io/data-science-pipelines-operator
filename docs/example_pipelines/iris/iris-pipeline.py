from typing import List
import kfp
from kfp import compiler
from kfp import dsl
from kfp import Client
from kfp.dsl import Dataset
from kfp.dsl import Input
from kfp.dsl import Model
from kfp.dsl import Output
from kfp.dsl import ClassificationMetrics


@dsl.component(
    base_image="registry.access.redhat.com/ubi9/python-311:latest",
    packages_to_install=['pandas==2.2.0']
)
def create_dataset(iris_dataset: Output[Dataset]):
    import pandas as pd

    csv_url = 'https://archive.ics.uci.edu/ml/machine-learning-databases/iris/iris.data'
    col_names = [
        'Sepal_Length', 'Sepal_Width', 'Petal_Length', 'Petal_Width', 'Labels'
    ]
    df = pd.read_csv(csv_url, names=col_names)

    with open(iris_dataset.path, 'w') as f:
        df.to_csv(f)

@dsl.component(
    base_image="registry.access.redhat.com/ubi9/python-311:latest",
    packages_to_install=['pandas==2.2.0', 'scikit-learn==1.4.0']
)
def normalize_dataset(
    input_iris_dataset: Input[Dataset],
    normalized_iris_dataset: Output[Dataset],
    standard_scaler: bool,
):

    import pandas as pd
    from sklearn.preprocessing import MinMaxScaler
    from sklearn.preprocessing import StandardScaler

    with open(input_iris_dataset.path) as f:
        df = pd.read_csv(f)
    labels = df.pop('Labels')

    scaler = StandardScaler() if standard_scaler else MinMaxScaler()

    df = pd.DataFrame(scaler.fit_transform(df))
    df['Labels'] = labels
    normalized_iris_dataset.metadata['state'] = "Normalized"
    with open(normalized_iris_dataset.path, 'w') as f:
        df.to_csv(f)


@dsl.component(
    base_image="registry.access.redhat.com/ubi9/python-311:latest",
    packages_to_install=['pandas==2.2.0', 'scikit-learn==1.4.0']
)
def train_model(
    normalized_iris_dataset: Input[Dataset],
    model: Output[Model],
    metrics: Output[ClassificationMetrics],
    n_neighbors: int,
):
    import pickle

    import pandas as pd
    from sklearn.model_selection import train_test_split
    from sklearn.neighbors import KNeighborsClassifier

    from sklearn.metrics import roc_curve
    from sklearn.model_selection import train_test_split, cross_val_predict
    from sklearn.metrics import confusion_matrix


    with open(normalized_iris_dataset.path) as f:
        df = pd.read_csv(f)

    y = df.pop('Labels')
    X = df

    X_train, X_test, y_train, y_test = train_test_split(X, y, random_state=0)

    clf = KNeighborsClassifier(n_neighbors=n_neighbors)
    clf.fit(X_train, y_train)

    predictions = cross_val_predict(
        clf, X_train, y_train, cv=3)
    metrics.log_confusion_matrix(
        ['Iris-Setosa', 'Iris-Versicolour', 'Iris-Virginica'],
        confusion_matrix(
            y_train,
            predictions).tolist()  # .tolist() to convert np array to list.
    )

    model.metadata['framework'] = 'scikit-learn'
    with open(model.path, 'wb') as f:
        pickle.dump(clf, f)


@dsl.pipeline(name='iris-training-pipeline')
def my_pipeline(
    standard_scaler: bool = True,
    neighbors: int = 3,
):
    create_dataset_task = create_dataset()

    normalize_dataset_task = normalize_dataset(
        input_iris_dataset=create_dataset_task.outputs['iris_dataset'],
        standard_scaler=True)

    train_model(
        normalized_iris_dataset=normalize_dataset_task
        .outputs['normalized_iris_dataset'],
        n_neighbors=neighbors)

endpoint = 'http://ml-pipeline-ui-kubeflow.apps.rmartine.dev.datahub.redhat.com/'

compiler.Compiler().compile(
    pipeline_func=my_pipeline,
    package_path= __file__.replace('.py', '-v2.yaml'))
