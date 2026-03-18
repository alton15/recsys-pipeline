package com.recsys.model

import com.fasterxml.jackson.annotation.JsonIgnoreProperties
import com.fasterxml.jackson.annotation.JsonProperty

/**
 * Represents a user interaction event consumed from the user-events topic.
 * Mirrors the Go Event struct from shared/go/event/event.go.
 * Immutable data class — never mutate, always copy.
 */
@JsonIgnoreProperties(ignoreUnknown = true)
data class UserEvent(
    @JsonProperty("event_id") val eventId: String,
    @JsonProperty("user_id") val userId: String,
    @JsonProperty("event_type") val eventType: String,
    @JsonProperty("item_id") val itemId: String,
    @JsonProperty("category_id") val categoryId: String = "",
    val timestamp: Long,
    @JsonProperty("session_id") val sessionId: String,
    val metadata: EventMetadata = EventMetadata()
)

@JsonIgnoreProperties(ignoreUnknown = true)
data class EventMetadata(
    val query: String = "",
    val position: Int = 0,
    val price: Long = 0,
    val source: String = ""
)
