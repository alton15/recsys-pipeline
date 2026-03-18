package com.recsys.model

import com.fasterxml.jackson.annotation.JsonIgnoreProperties
import com.fasterxml.jackson.annotation.JsonProperty

/**
 * Represents an inventory status change from the inventory-events topic.
 * Immutable data class.
 */
@JsonIgnoreProperties(ignoreUnknown = true)
data class InventoryEvent(
    @JsonProperty("item_id") val itemId: String,
    @JsonProperty("in_stock") val inStock: Boolean,
    val quantity: Int,
    val timestamp: Long
)
