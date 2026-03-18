package com.recsys.model

/**
 * Represents a command to be executed against DragonflyDB.
 * Immutable — create new instances instead of mutating.
 *
 * @param key The DragonflyDB key
 * @param operation The Redis operation (SET, SETBIT, ZADD, etc.)
 * @param value The value to write
 * @param field Optional field for hash or bitmap offset
 * @param score Optional score for sorted set operations
 * @param ttlSeconds TTL in seconds (0 = no expiry)
 */
data class SinkCommand(
    val key: String,
    val operation: String,
    val value: String,
    val field: String = "",
    val score: Double = 0.0,
    val ttlSeconds: Long = 0
)
