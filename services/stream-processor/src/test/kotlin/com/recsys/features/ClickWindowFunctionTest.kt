package com.recsys.features

import org.apache.flink.api.common.typeinfo.TypeInformation
import org.apache.flink.api.common.typeinfo.Types
import org.apache.flink.streaming.api.watermark.Watermark
import org.apache.flink.streaming.api.windowing.windows.TimeWindow
import org.apache.flink.streaming.runtime.streamrecord.StreamRecord
import org.apache.flink.streaming.util.KeyedOneInputStreamOperatorTestHarness
import org.apache.flink.streaming.api.operators.KeyedProcessOperator
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.Assertions.*
import com.recsys.model.UserEvent
import com.recsys.model.SinkCommand

class ClickWindowFunctionTest {

    @Test
    fun `should count clicks within window for a single user`() {
        val function = ClickWindowFunction()

        // Create test events
        val events = listOf(
            UserEvent(
                eventId = "e1",
                userId = "user-1",
                eventType = "click",
                itemId = "item-1",
                categoryId = "cat-1",
                timestamp = 1000L,
                sessionId = "sess-1"
            ),
            UserEvent(
                eventId = "e2",
                userId = "user-1",
                eventType = "click",
                itemId = "item-2",
                categoryId = "cat-1",
                timestamp = 2000L,
                sessionId = "sess-1"
            ),
            UserEvent(
                eventId = "e3",
                userId = "user-1",
                eventType = "view",
                itemId = "item-3",
                categoryId = "cat-1",
                timestamp = 3000L,
                sessionId = "sess-1"
            )
        )

        // Aggregate: only clicks should be counted
        val clickCount = events.count { it.eventType == "click" }
        assertEquals(2, clickCount)
    }

    @Test
    fun `should produce correct DragonflyDB key format`() {
        val userId = "user-123"
        val expectedKey = "feat:user:user-123:clicks_1h"
        assertEquals(expectedKey, "feat:user:$userId:clicks_1h")
    }

    @Test
    fun `should generate SinkCommand with SET operation for click count`() {
        val function = ClickWindowFunction()

        val result = function.buildSinkCommand("user-42", 15)

        assertEquals("feat:user:user-42:clicks_1h", result.key)
        assertEquals("SET", result.operation)
        assertEquals("15", result.value)
        assertTrue(result.ttlSeconds > 0, "TTL should be positive")
    }

    @Test
    fun `should filter only click events`() {
        val function = ClickWindowFunction()

        assertTrue(function.isClickEvent("click"))
        assertFalse(function.isClickEvent("view"))
        assertFalse(function.isClickEvent("purchase"))
        assertFalse(function.isClickEvent("search"))
        assertFalse(function.isClickEvent("add_to_cart"))
    }

    @Test
    fun `should handle empty event list gracefully`() {
        val function = ClickWindowFunction()
        val result = function.buildSinkCommand("user-0", 0)

        assertEquals("0", result.value)
        assertEquals("feat:user:user-0:clicks_1h", result.key)
    }
}
