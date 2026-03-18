package com.recsys.features

import com.recsys.model.SinkCommand
import com.recsys.model.UserEvent
import org.apache.flink.streaming.api.functions.windowing.ProcessWindowFunction
import org.apache.flink.streaming.api.windowing.windows.TimeWindow
import org.apache.flink.util.Collector
import org.slf4j.LoggerFactory

/**
 * Sliding window function that aggregates click events per user
 * over a 1-hour window with 1-minute slide.
 *
 * Produces SinkCommand with the click count to be written to DragonflyDB
 * at key: feat:user:<user_id>:clicks_1h
 */
class ClickWindowFunction : ProcessWindowFunction<UserEvent, SinkCommand, String, TimeWindow>() {

    companion object {
        private val logger = LoggerFactory.getLogger(ClickWindowFunction::class.java)
        private const val KEY_PREFIX = "feat:user:"
        private const val KEY_SUFFIX = ":clicks_1h"
        private const val TTL_SECONDS = 3660L // 1 hour + 1 minute buffer
    }

    override fun process(
        key: String,
        context: Context,
        elements: Iterable<UserEvent>,
        out: Collector<SinkCommand>
    ) {
        val clickCount = elements.count { isClickEvent(it.eventType) }
        val command = buildSinkCommand(key, clickCount)

        logger.debug("User {} click count in window: {}", key, clickCount)
        out.collect(command)
    }

    /**
     * Returns true if the event type represents a click.
     */
    fun isClickEvent(eventType: String): Boolean = eventType == "click"

    /**
     * Builds an immutable SinkCommand for writing the click count to DragonflyDB.
     */
    fun buildSinkCommand(userId: String, clickCount: Int): SinkCommand {
        return SinkCommand(
            key = "$KEY_PREFIX$userId$KEY_SUFFIX",
            operation = "SET",
            value = clickCount.toString(),
            ttlSeconds = TTL_SECONDS
        )
    }
}
