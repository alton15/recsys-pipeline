package com.recsys

import com.fasterxml.jackson.databind.ObjectMapper
import com.fasterxml.jackson.module.kotlin.jacksonObjectMapper
import com.recsys.features.ClickWindowFunction
import com.recsys.features.SessionProcessor
import com.recsys.inventory.StockBitmapUpdater
import com.recsys.model.InventoryEvent
import com.recsys.model.SinkCommand
import com.recsys.model.UserEvent
import com.recsys.sinks.DragonflySink
import org.apache.flink.api.common.eventtime.WatermarkStrategy
import org.apache.flink.api.common.serialization.AbstractDeserializationSchema
import org.apache.flink.connector.kafka.source.KafkaSource
import org.apache.flink.connector.kafka.source.enumerator.initializer.OffsetsInitializer
import org.apache.flink.streaming.api.environment.StreamExecutionEnvironment
import org.apache.flink.streaming.api.windowing.assigners.SlidingEventTimeWindows
import org.apache.flink.streaming.api.windowing.time.Time
import org.slf4j.LoggerFactory
import java.time.Duration

private val logger = LoggerFactory.getLogger("StreamProcessorJob")

/**
 * Flink streaming job that:
 * 1. Consumes user-events from Redpanda, performs sliding window click aggregation
 * 2. Buffers session events into sorted sets
 * 3. Consumes inventory-events and updates stock bitmap
 *
 * All results are written to DragonflyDB via DragonflySink.
 */
fun main() {
    val brokers = System.getenv("KAFKA_BROKERS") ?: "localhost:9092"
    val groupId = System.getenv("CONSUMER_GROUP") ?: "recsys-stream-processor"

    val env = StreamExecutionEnvironment.getExecutionEnvironment()
    env.parallelism = (System.getenv("FLINK_PARALLELISM") ?: "2").toInt()

    // ── Source: user-events ──────────────────────────────────────
    val userEventSource = buildKafkaSource<UserEvent>(
        brokers = brokers,
        topic = "user-events",
        groupId = groupId,
        deserializer = JsonDeserializationSchema(UserEvent::class.java)
    )

    val userEvents = env.fromSource(
        userEventSource,
        WatermarkStrategy
            .forBoundedOutOfOrderness<UserEvent>(Duration.ofSeconds(5))
            .withTimestampAssigner { event, _ -> event.timestamp },
        "user-events-source"
    )

    // ── Branch 1: Click window aggregation ──────────────────────
    userEvents
        .keyBy { it.userId }
        .window(SlidingEventTimeWindows.of(Time.hours(1), Time.minutes(1)))
        .process(ClickWindowFunction())
        .name("click-window-aggregation")
        .addSink(DragonflySink())
        .name("click-count-sink")

    // ── Branch 2: Session event buffer ──────────────────────────
    userEvents
        .keyBy { it.sessionId }
        .process(SessionProcessor())
        .name("session-processor")
        .addSink(DragonflySink())
        .name("session-events-sink")

    // ── Source: inventory-events → Stock bitmap ─────────────────
    val inventoryEventSource = buildKafkaSource<InventoryEvent>(
        brokers = brokers,
        topic = "inventory-events",
        groupId = groupId,
        deserializer = JsonDeserializationSchema(InventoryEvent::class.java)
    )

    val inventoryEvents = env.fromSource(
        inventoryEventSource,
        WatermarkStrategy.forMonotonousTimestamps<InventoryEvent>()
            .withTimestampAssigner { event, _ -> event.timestamp },
        "inventory-events-source"
    )

    inventoryEvents
        .keyBy { it.itemId }
        .process(StockBitmapUpdater())
        .name("stock-bitmap-updater")
        .addSink(DragonflySink())
        .name("stock-bitmap-sink")

    logger.info("Starting recsys-stream-processor with brokers={}", brokers)
    env.execute("recsys-stream-processor")
}

/**
 * Builds a KafkaSource configured for Redpanda compatibility.
 */
private fun <T> buildKafkaSource(
    brokers: String,
    topic: String,
    groupId: String,
    deserializer: AbstractDeserializationSchema<T>
): KafkaSource<T> {
    return KafkaSource.builder<T>()
        .setBootstrapServers(brokers)
        .setTopics(topic)
        .setGroupId(groupId)
        .setStartingOffsets(OffsetsInitializer.latest())
        .setValueOnlyDeserializer(deserializer)
        .build()
}

/**
 * Generic JSON deserialization schema using Jackson.
 */
class JsonDeserializationSchema<T>(
    private val targetClass: Class<T>
) : AbstractDeserializationSchema<T>(targetClass) {

    @Transient
    private var mapper: ObjectMapper? = null

    private fun getMapper(): ObjectMapper {
        if (mapper == null) {
            mapper = jacksonObjectMapper()
        }
        return mapper!!
    }

    override fun deserialize(message: ByteArray): T {
        return getMapper().readValue(message, targetClass)
    }
}
