package com.recsys.sinks

import com.recsys.model.SinkCommand
import org.apache.flink.configuration.Configuration
import org.apache.flink.streaming.api.functions.sink.RichSinkFunction
import org.slf4j.LoggerFactory
import redis.clients.jedis.JedisPool
import redis.clients.jedis.JedisPoolConfig

/**
 * Flink sink that executes SinkCommands against DragonflyDB (Redis-compatible).
 * Supports SET, SETBIT, and ZADD operations with optional TTL.
 *
 * Connection parameters are read from environment variables:
 * - DRAGONFLY_HOST (default: localhost)
 * - DRAGONFLY_PORT (default: 6379)
 */
class DragonflySink : RichSinkFunction<SinkCommand>() {

    companion object {
        private val logger = LoggerFactory.getLogger(DragonflySink::class.java)
        private const val DEFAULT_HOST = "localhost"
        private const val DEFAULT_PORT = 6379
    }

    @Transient
    private var jedisPool: JedisPool? = null

    override fun open(parameters: Configuration) {
        val host = System.getenv("DRAGONFLY_HOST") ?: DEFAULT_HOST
        val port = (System.getenv("DRAGONFLY_PORT") ?: DEFAULT_PORT.toString()).toInt()

        val poolConfig = JedisPoolConfig().apply {
            maxTotal = 16
            maxIdle = 8
            minIdle = 2
            testOnBorrow = true
        }

        jedisPool = JedisPool(poolConfig, host, port)
        logger.info("DragonflySink connected to {}:{}", host, port)
    }

    override fun invoke(command: SinkCommand, context: org.apache.flink.streaming.api.functions.sink.SinkFunction.Context) {
        val pool = jedisPool ?: throw IllegalStateException("JedisPool not initialized")

        pool.resource.use { jedis ->
            when (command.operation) {
                "SET" -> {
                    jedis.set(command.key, command.value)
                    if (command.ttlSeconds > 0) {
                        jedis.expire(command.key, command.ttlSeconds)
                    }
                }
                "SETBIT" -> {
                    val offset = command.field.toLong()
                    val bit = command.value == "1"
                    jedis.setbit(command.key, offset, bit)
                }
                "ZADD" -> {
                    jedis.zadd(command.key, command.score, command.value)
                    if (command.ttlSeconds > 0) {
                        jedis.expire(command.key, command.ttlSeconds)
                    }
                }
                else -> {
                    logger.warn("Unknown operation: {}", command.operation)
                }
            }

            logger.debug("Executed {} on key {}", command.operation, command.key)
        }
    }

    override fun close() {
        jedisPool?.close()
        jedisPool = null
        logger.info("DragonflySink closed")
    }
}
