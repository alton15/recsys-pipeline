package com.recsys.inventory

import com.recsys.model.InventoryEvent
import com.recsys.model.SinkCommand
import org.apache.flink.api.common.state.MapState
import org.apache.flink.api.common.state.MapStateDescriptor
import org.apache.flink.api.common.state.ValueState
import org.apache.flink.api.common.state.ValueStateDescriptor
import org.apache.flink.api.common.typeinfo.Types
import org.apache.flink.configuration.Configuration
import org.apache.flink.streaming.api.functions.KeyedProcessFunction
import org.apache.flink.util.Collector
import org.slf4j.LoggerFactory

/**
 * Consumes inventory-events and updates the stock availability bitmap
 * in DragonflyDB. Each item maps to a bitmap offset via stock:id_map:<item_id>.
 *
 * Commands produced:
 * - SETBIT stock:bitmap <offset> <0|1>
 * - SET stock:id_map:<item_id> <offset>
 */
class StockBitmapUpdater : KeyedProcessFunction<String, InventoryEvent, SinkCommand>() {

    companion object {
        private val logger = LoggerFactory.getLogger(StockBitmapUpdater::class.java)
        const val BITMAP_KEY = "stock:bitmap"
        private const val ID_MAP_PREFIX = "stock:id_map:"

        fun idMapKey(itemId: String): String = "$ID_MAP_PREFIX$itemId"
    }

    private lateinit var bitmapOffset: ValueState<Long>

    // Counter for auto-incrementing bitmap offsets (keyed by item)
    private lateinit var nextOffset: ValueState<Long>

    override fun open(parameters: Configuration) {
        bitmapOffset = runtimeContext.getState(
            ValueStateDescriptor("bitmapOffset", Types.LONG)
        )
        nextOffset = runtimeContext.getState(
            ValueStateDescriptor("nextOffset", Types.LONG)
        )
    }

    override fun processElement(
        event: InventoryEvent,
        ctx: Context,
        out: Collector<SinkCommand>
    ) {
        // Get or assign bitmap offset for this item
        var offset = bitmapOffset.value()
        val isNewItem = offset == null
        if (isNewItem) {
            offset = ctx.currentKey.hashCode().toLong().and(0x7FFFFFFF) // positive offset
            bitmapOffset.update(offset)
        }

        val commands = buildCommands(event, offset)
        commands.forEach { out.collect(it) }

        logger.debug("Item {} stock={} offset={}", event.itemId, resolveStockStatus(event), offset)
    }

    /**
     * Resolves the effective stock status.
     * If quantity is 0, item is considered out-of-stock regardless of inStock flag.
     */
    fun resolveStockStatus(event: InventoryEvent): Boolean {
        return event.inStock && event.quantity > 0
    }

    /**
     * Builds the list of SinkCommands for a single inventory event.
     * Returns an immutable list.
     */
    fun buildCommands(event: InventoryEvent, bitmapOffset: Long): List<SinkCommand> {
        val effectiveInStock = resolveStockStatus(event)
        val bitValue = if (effectiveInStock) "1" else "0"

        val setbitCommand = SinkCommand(
            key = BITMAP_KEY,
            operation = "SETBIT",
            value = bitValue,
            field = bitmapOffset.toString()
        )

        val idMapCommand = SinkCommand(
            key = idMapKey(event.itemId),
            operation = "SET",
            value = bitmapOffset.toString()
        )

        return listOf(setbitCommand, idMapCommand)
    }
}
