package com.recsys.features

import com.recsys.model.SinkCommand
import com.recsys.model.UserEvent
import org.apache.flink.api.common.state.ValueState
import org.apache.flink.api.common.state.ValueStateDescriptor
import org.apache.flink.api.common.typeinfo.Types
import org.apache.flink.configuration.Configuration
import org.apache.flink.streaming.api.functions.KeyedProcessFunction
import org.apache.flink.util.Collector
import org.slf4j.LoggerFactory

/**
 * Processes user events keyed by sessionId and emits ZADD commands
 * to maintain a sorted set of session events in DragonflyDB.
 *
 * Key: session:<session_id>:events
 * Score: event timestamp
 * Member: serialized event summary
 *
 * Sessions have a 30-minute TTL from the last event.
 */
class SessionProcessor : KeyedProcessFunction<String, UserEvent, SinkCommand>() {

    companion object {
        private val logger = LoggerFactory.getLogger(SessionProcessor::class.java)
        private const val KEY_PREFIX = "session:"
        private const val KEY_SUFFIX = ":events"
        private const val SESSION_TTL_SECONDS = 1800L // 30 minutes
    }

    private lateinit var lastEventTime: ValueState<Long>

    override fun open(parameters: Configuration) {
        val descriptor = ValueStateDescriptor("lastEventTime", Types.LONG)
        lastEventTime = runtimeContext.getState(descriptor)
    }

    override fun processElement(
        event: UserEvent,
        ctx: Context,
        out: Collector<SinkCommand>
    ) {
        val sessionId = ctx.currentKey
        lastEventTime.update(event.timestamp)

        val member = buildMember(event)
        val command = buildSinkCommand(sessionId, event.timestamp.toDouble(), member)

        logger.debug("Session {} event: {} at {}", sessionId, event.eventType, event.timestamp)
        out.collect(command)
    }

    /**
     * Builds a compact member string for the sorted set entry.
     * Format: eventType:itemId:eventId
     */
    fun buildMember(event: UserEvent): String {
        return "${event.eventType}:${event.itemId}:${event.eventId}"
    }

    /**
     * Builds an immutable SinkCommand for ZADD to the session sorted set.
     */
    fun buildSinkCommand(sessionId: String, score: Double, member: String): SinkCommand {
        return SinkCommand(
            key = "$KEY_PREFIX$sessionId$KEY_SUFFIX",
            operation = "ZADD",
            value = member,
            score = score,
            ttlSeconds = SESSION_TTL_SECONDS
        )
    }
}
