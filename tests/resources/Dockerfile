FROM docker.io/python:3.9.17

WORKDIR /
RUN mkdir .cache .local minio /tmp/kfp s3 gcs
RUN chgrp -R 0 /.cache && \
chmod -R g=u /.cache && \
chgrp -R 0 /tmp/kfp && \
chmod -R g=u /tmp/kfp && \
chgrp -R 0 /.local && \
chmod -R g=u /.local && \
chgrp -R 0 /minio && \
chmod -R g=u /minio && \
chgrp -R 0 /s3 && \
chmod -R g=u /s3 && \
chgrp -R 0 /gcs && \
chmod -R g=u /gcs
