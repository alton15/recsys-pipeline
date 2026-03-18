### Builder stage
FROM gradle:8.10-jdk21 AS builder

WORKDIR /build

# Copy build files first for layer caching
COPY services/stream-processor/build.gradle.kts services/stream-processor/
COPY services/stream-processor/settings.gradle.kts services/stream-processor/
COPY services/stream-processor/gradle/ services/stream-processor/gradle/

WORKDIR /build/services/stream-processor
RUN gradle dependencies --no-daemon || true

# Copy source code
WORKDIR /build
COPY services/stream-processor/ services/stream-processor/

WORKDIR /build/services/stream-processor
RUN gradle shadowJar --no-daemon 2>/dev/null || gradle jar --no-daemon

### Runtime stage
FROM eclipse-temurin:21-jre-alpine

RUN apk add --no-cache tini

COPY --from=builder /build/services/stream-processor/build/libs/*.jar /opt/flink/app.jar

ENV KAFKA_BROKERS=redpanda:29092
ENV DRAGONFLY_HOST=dragonfly
ENV DRAGONFLY_PORT=6379
ENV FLINK_PARALLELISM=2
ENV CONSUMER_GROUP=recsys-stream-processor

ENTRYPOINT ["tini", "--"]
CMD ["java", "-cp", "/opt/flink/app.jar", "com.recsys.StreamProcessorJobKt"]
