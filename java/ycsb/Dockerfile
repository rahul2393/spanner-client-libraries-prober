# Build from spanner-client-libraries-prober repo root.
# Release example:
#   docker build -f java/ycsb/Dockerfile --target runtime-release \
#     --build-arg SPANNER_VERSION=6.117.0 -t irahul-ycsb:6.117.0 .
# Source branch example:
#   docker build -f java/ycsb/Dockerfile --target runtime-source \
#     --build-arg GCJ_REPO_URL=https://github.com/rahul2393/google-cloud-java.git \
#     --build-arg GCJ_REF=my-fix-branch -t irahul-ycsb:my-fix-branch .

FROM maven:3.9-eclipse-temurin-17 AS release-build

ARG SPANNER_VERSION=6.117.0
WORKDIR /workspace/prober/java/ycsb

COPY --chown=1000:1000 java/ycsb /workspace/prober/java/ycsb

RUN mvn -B -ntp package \
    -pl cloudspanner -am \
    -Dcloudspanner.version="${SPANNER_VERSION}" \
    -DskipTests -Dmaven.test.skip=true -Dcheckstyle.skip

RUN /workspace/prober/java/ycsb/scripts/assemble-runtime.sh /workspace/prober/java/ycsb /tmp/out

FROM maven:3.9-eclipse-temurin-17 AS source-build

ARG GCJ_REPO_URL=https://github.com/rahul2393/google-cloud-java.git
ARG GCJ_REF=main
WORKDIR /workspace

USER root
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates git file \
 && rm -rf /var/lib/apt/lists/* \
 && chown -R 1000:1000 /workspace
USER 1000

COPY --chown=1000:1000 java/ycsb /workspace/prober/java/ycsb

RUN git clone --depth 1 --branch "${GCJ_REF}" "${GCJ_REPO_URL}" /workspace/google-cloud-java \
 || (git clone "${GCJ_REPO_URL}" /workspace/google-cloud-java \
     && cd /workspace/google-cloud-java \
     && git checkout "${GCJ_REF}")

RUN mvn -f /workspace/google-cloud-java/sdk-platform-java/pom.xml \
    -pl gapic-generator-java-pom-parent,java-shared-dependencies/third-party-dependencies \
    -am clean install \
    -DskipTests -Dmaven.test.skip=true -Dclirr.skip -Dmaven.javadoc.skip=true -Dcheckstyle.skip
RUN mvn -f /workspace/google-cloud-java/java-common-protos/pom.xml clean install \
    -DskipTests -Dmaven.test.skip=true -Dclirr.skip -Dmaven.javadoc.skip=true -Dcheckstyle.skip
RUN mvn -f /workspace/google-cloud-java/sdk-platform-java/pom.xml \
    -pl api-common-java \
    -am install \
    -DskipTests -Dmaven.test.skip=true -Dclirr.skip -Dmaven.javadoc.skip=true -Dcheckstyle.skip
RUN mvn -f /workspace/google-cloud-java/google-auth-library-java/pom.xml \
    -pl credentials,oauth2_http,bom \
    -am clean install \
    -DskipTests -Dmaven.test.skip=true -Dclirr.skip -Dmaven.javadoc.skip=true -Dcheckstyle.skip
RUN mvn -f /workspace/google-cloud-java/sdk-platform-java/pom.xml \
    -pl gax-java/gax-bom \
    -am install \
    -DskipTests -Dmaven.test.skip=true -Dclirr.skip -Dmaven.javadoc.skip=true -Dcheckstyle.skip
RUN mvn -f /workspace/google-cloud-java/java-iam/pom.xml \
    -pl proto-google-iam-v1 \
    -am clean install \
    -DskipTests -Dmaven.test.skip=true -Dclirr.skip -Dmaven.javadoc.skip=true -Dcheckstyle.skip
RUN mvn -f /workspace/google-cloud-java/sdk-platform-java/pom.xml \
    -pl java-shared-dependencies,api-common-java,gax-java/gax-bom,gax-java/gax,gax-java/gax-grpc,gax-java/gax-httpjson,java-core/google-cloud-core-grpc \
    -am install \
    -DskipTests -Dmaven.test.skip=true -Dclirr.skip -Dmaven.javadoc.skip=true -Dcheckstyle.skip
RUN mvn -f /workspace/google-cloud-java/sdk-platform-java/pom.xml \
    -pl gapic-generator-java-bom,java-core/google-cloud-core-bom,java-shared-dependencies/first-party-dependencies,java-shared-dependencies \
    -am install \
    -DskipTests -Dmaven.test.skip=true -Dclirr.skip -Dmaven.javadoc.skip=true -Dcheckstyle.skip
RUN mvn -f /workspace/google-cloud-java/java-monitoring/pom.xml \
    -pl google-cloud-monitoring,proto-google-cloud-monitoring-v3,grpc-google-cloud-monitoring-v3 \
    -am clean install \
    -DskipTests -Dmaven.test.skip=true -Dclirr.skip -Dmaven.javadoc.skip=true -Dcheckstyle.skip
RUN mvn -f /workspace/google-cloud-java/java-trace/pom.xml \
    -pl google-cloud-trace,proto-google-cloud-trace-v1 \
    -am clean install \
    -DskipTests -Dmaven.test.skip=true -Dclirr.skip -Dmaven.javadoc.skip=true -Dcheckstyle.skip

RUN mvn -f /workspace/google-cloud-java/java-spanner/pom.xml install \
    -DskipTests -Dmaven.test.skip=true -Dclirr.skip -Dmaven.javadoc.skip=true -Dcheckstyle.skip \
    -pl google-cloud-spanner -am

RUN SPANNER_VERSION="$(mvn -q -f /workspace/google-cloud-java/java-spanner/pom.xml help:evaluate -Dexpression=project.version -DforceStdout)" \
 && echo "Building YCSB with google-cloud-spanner ${SPANNER_VERSION}" \
 && mvn -B -ntp -f /workspace/prober/java/ycsb/pom.xml package \
      -pl cloudspanner -am \
      -Dcloudspanner.version="${SPANNER_VERSION}" \
      -DskipTests -Dmaven.test.skip=true -Dcheckstyle.skip \
 && /workspace/prober/java/ycsb/scripts/assemble-runtime.sh /workspace/prober/java/ycsb /tmp/out \
 && file /tmp/out/cloudspanner-binding/lib/cloudspanner-binding.jar

FROM hengyunabc/arthas:latest AS arthas

FROM eclipse-temurin:17-jdk AS runtime-release

WORKDIR /ycsb
RUN apt-get update \
 && apt-get install -y --no-install-recommends python3 python-is-python3 procps \
 && rm -rf /var/lib/apt/lists/*
COPY --from=release-build /tmp/out/ /ycsb/
COPY --from=arthas /opt/arthas /opt/arthas
ENTRYPOINT ["/ycsb/bin/ycsb"]

FROM eclipse-temurin:17-jdk AS runtime-source

WORKDIR /ycsb
RUN apt-get update \
 && apt-get install -y --no-install-recommends python3 python-is-python3 procps \
 && rm -rf /var/lib/apt/lists/*
COPY --from=source-build /tmp/out/ /ycsb/
COPY --from=arthas /opt/arthas /opt/arthas
ENTRYPOINT ["/ycsb/bin/ycsb"]
