# Adding declarative tests

Declarative tests are for simple tests to check for OCP resource creations by DSPO when a `DataSciencePipelinesApplication` (`DSPA`) is created. New test cases can be added by simply adding the deployment resources and their expected manifest pairs within the corresponding `case` folder. 

# Adding a new case example

All declarative test cases can be found in `controllers/testdata/declarative`.

Let's say we want a new test case `case_4` that tests that this `DSPA`...

<details> 

<summary> dspa.yaml</summary>

```yaml
# dspa.yaml
apiVersion: datasciencepipelinesapplications.opendatahub.io/v1
kind: DataSciencePipelinesApplication
metadata:
  name: testdsp4 #ma
spec:
  objectStorage:
    minio:
      image: minio:test4
```
</details>

...will deploy this configmap: 

<details> 

<summary> configmap.yaml</summary>

```yaml
apiVersion: v1
data:
  artifact_script: |-
    #!/usr/bin/env sh
    push_artifact() {
        if [ -f "$2" ]; then
            tar -cvzf $1.tgz $2
            aws s3 --endpoint ${ARTIFACT_ENDPOINT} cp $1.tgz s3://$ARTIFACT_BUCKET/artifacts/$PIPELINERUN/$PIPELINETASK/$1.tgz
        else
            echo "$2 file does not exist. Skip artifact tracking for $1"
        fi
    }
    push_log() {
        cat /var/log/containers/$PODNAME*$NAMESPACE*step-main*.log > step-main.log
        push_artifact main-log step-main.log
    }
    strip_eof() {
        if [ -f "$2" ]; then
            awk 'NF' $2 | head -c -1 > $1_temp_save && cp $1_temp_save $2
        fi
    }
kind: ConfigMap
metadata:
  name: ds-pipeline-artifact-script-testdsp4
  namespace: default
  labels:
    app: ds-pipeline-testdsp4
    component: data-science-pipelines

```

</details>


We can do this by first creating a folder `controllers/testdata/declarative/case_4`
Then adding the DSPA cr in: `controllers/testdata/declarative/case_4/deploy/dspa.yaml` (we want to the test case to `deploy` this DSPA)

> Note you can add multiple resources in the ../deploy folder, and the test case will deploy all of them 
> If certain resources are dependent on others, they should be ordered alphabetically in the order they shoudl be
> deployed. For example resource 00_res.yaml will be deployed before 01_res.yaml, so ensure that 00_res.yaml does not 
> depend on 01_res.yaml.

Then adding the configmap resource in: `controllers/testdata/declarative/case_4/expected/created/configmap.yaml`
Each case requires a configmap, we can add one like this: 

```yaml
Images:
  ApiServer: api-server:test4
  Artifact: artifact-manager:test4
  PersistenceAgent: persistenceagent:test4
  ScheduledWorkflow: scheduledworkflow:test4
  Cache: ubi-minimal:test4
  MoveResultsImage: busybox:test4
  MariaDB: mariadb:test4
  Minio: minio:test4
```
In `controllers/testdata/declarative/case_4/config.yaml`

Then run the tests by running: 

`make test`

You can add more expected resources by adding files in the `../expected/created` folder. To test resources that you do 
not expect to be created, add them in the `../expected/not_created` folders. Please see `/controllers/testutil/equalities.go` for
all supported resources.

For more complex tests, it is advised to create tests via logic.
