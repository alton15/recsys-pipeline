package com.recsys.inventory

import org.junit.jupiter.api.Test
import org.junit.jupiter.api.Assertions.*
import com.recsys.model.InventoryEvent
import com.recsys.model.SinkCommand

class StockBitmapUpdaterTest {

    @Test
    fun `should produce SETBIT command for in-stock item`() {
        val updater = StockBitmapUpdater()
        val event = InventoryEvent(
            itemId = "item-100",
            inStock = true,
            quantity = 50,
            timestamp = System.currentTimeMillis()
        )

        val commands = updater.buildCommands(event, bitmapOffset = 42L)

        // Should have SETBIT command
        val setbitCmd = commands.find { it.operation == "SETBIT" }
        assertNotNull(setbitCmd, "Should produce SETBIT command")
        assertEquals("stock:bitmap", setbitCmd!!.key)
        assertEquals("42", setbitCmd.field)
        assertEquals("1", setbitCmd.value) // in-stock = bit 1
    }

    @Test
    fun `should produce SETBIT 0 for out-of-stock item`() {
        val updater = StockBitmapUpdater()
        val event = InventoryEvent(
            itemId = "item-200",
            inStock = false,
            quantity = 0,
            timestamp = System.currentTimeMillis()
        )

        val commands = updater.buildCommands(event, bitmapOffset = 99L)

        val setbitCmd = commands.find { it.operation == "SETBIT" }
        assertNotNull(setbitCmd)
        assertEquals("0", setbitCmd!!.value) // out-of-stock = bit 0
    }

    @Test
    fun `should produce SET command for id map entry`() {
        val updater = StockBitmapUpdater()
        val event = InventoryEvent(
            itemId = "item-300",
            inStock = true,
            quantity = 10,
            timestamp = System.currentTimeMillis()
        )

        val commands = updater.buildCommands(event, bitmapOffset = 7L)

        val idMapCmd = commands.find { it.key.startsWith("stock:id_map:") }
        assertNotNull(idMapCmd, "Should produce id_map command")
        assertEquals("stock:id_map:item-300", idMapCmd!!.key)
        assertEquals("SET", idMapCmd.operation)
        assertEquals("7", idMapCmd.value)
    }

    @Test
    fun `should compute correct bitmap key`() {
        assertEquals("stock:bitmap", StockBitmapUpdater.BITMAP_KEY)
    }

    @Test
    fun `should compute correct id map key`() {
        val key = StockBitmapUpdater.idMapKey("item-abc")
        assertEquals("stock:id_map:item-abc", key)
    }

    @Test
    fun `should handle zero quantity as out-of-stock`() {
        val updater = StockBitmapUpdater()
        val event = InventoryEvent(
            itemId = "item-400",
            inStock = true,  // marked in-stock but quantity=0
            quantity = 0,
            timestamp = System.currentTimeMillis()
        )

        // When quantity is 0, treat as out-of-stock regardless of inStock flag
        val effectiveInStock = updater.resolveStockStatus(event)
        assertFalse(effectiveInStock)
    }
}
