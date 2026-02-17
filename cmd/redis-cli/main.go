package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/redis/go-redis/v9"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:6379", "Redis address")
	password := flag.String("password", "", "Redis password")
	db := flag.Int("db", 0, "Redis database")
	flag.Parse()

	// Create Redis client
	client := redis.NewClient(&redis.Options{
		Addr:     *addr,
		Password: *password,
		DB:       *db,
	})
	defer client.Close()

	ctx := context.Background()

	// Test connection
	pong, err := client.Ping(ctx).Result()
	if err != nil {
		fmt.Printf("❌ Failed to connect: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✅ Connected to Redis: %s\n\n", pong)

	// Interactive mode
	fmt.Println("Type commands below (or 'help' for available commands):")
	fmt.Println("---")

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		parts := strings.Fields(input)
		cmd := strings.ToUpper(parts[0])

		switch cmd {
		case "HELP":
			showHelp()

		case "KEYS":
			if len(parts) < 2 {
				fmt.Println("Usage: KEYS <pattern>")
				continue
			}
			pattern := parts[1]
			keys, err := client.Keys(ctx, pattern).Result()
			if err != nil {
				fmt.Printf("❌ Error: %v\n", err)
				continue
			}
			fmt.Printf("Found %d keys:\n", len(keys))
			for _, k := range keys {
				fmt.Printf("  - %s\n", k)
			}

		case "GET":
			if len(parts) < 2 {
				fmt.Println("Usage: GET <key>")
				continue
			}
			key := parts[1]
			val, err := client.Get(ctx, key).Result()
			if err != nil {
				fmt.Printf("❌ Error: %v\n", err)
				continue
			}
			fmt.Printf("✅ %s = %s\n", key, val)

		case "HGETALL":
			if len(parts) < 2 {
				fmt.Println("Usage: HGETALL <key>")
				continue
			}
			key := parts[1]
			data, err := client.HGetAll(ctx, key).Result()
			if err != nil {
				fmt.Printf("❌ Error: %v\n", err)
				continue
			}
			fmt.Printf("✅ Hash %s (%d fields):\n", key, len(data))
			for k, v := range data {
				fmt.Printf("  %s: %s\n", k, truncate(v, 100))
			}

		case "LLEN":
			if len(parts) < 2 {
				fmt.Println("Usage: LLEN <key>")
				continue
			}
			key := parts[1]
			len, err := client.LLen(ctx, key).Result()
			if err != nil {
				fmt.Printf("❌ Error: %v\n", err)
				continue
			}
			fmt.Printf("✅ List length: %d\n", len)

		case "LRANGE":
			if len(parts) < 4 {
				fmt.Println("Usage: LRANGE <key> <start> <stop>")
				continue
			}
			key := parts[1]
			start := parseInt(parts[2])
			stop := parseInt(parts[3])
			vals, err := client.LRange(ctx, key, int64(start), int64(stop)).Result()
			if err != nil {
				fmt.Printf("❌ Error: %v\n", err)
				continue
			}
			fmt.Printf("✅ List range [%d:%d] (%d items):\n", start, stop, len(vals))
			for i, v := range vals {
				fmt.Printf("  [%d] %s\n", i, truncate(v, 100))
			}

		case "SCARD":
			if len(parts) < 2 {
				fmt.Println("Usage: SCARD <key>")
				continue
			}
			key := parts[1]
			card, err := client.SCard(ctx, key).Result()
			if err != nil {
				fmt.Printf("❌ Error: %v\n", err)
				continue
			}
			fmt.Printf("✅ Set cardinality: %d\n", card)

		case "SMEMBERS":
			if len(parts) < 2 {
				fmt.Println("Usage: SMEMBERS <key>")
				continue
			}
			key := parts[1]
			members, err := client.SMembers(ctx, key).Result()
			if err != nil {
				fmt.Printf("❌ Error: %v\n", err)
				continue
			}
			fmt.Printf("✅ Set members (%d items):\n", len(members))
			for _, m := range members {
				fmt.Printf("  - %s\n", m)
			}

		case "TYPE":
			if len(parts) < 2 {
				fmt.Println("Usage: TYPE <key>")
				continue
			}
			key := parts[1]
			keyType, err := client.Type(ctx, key).Result()
			if err != nil {
				fmt.Printf("❌ Error: %v\n", err)
				continue
			}
			fmt.Printf("✅ Type: %s\n", keyType)

		case "DBSIZE":
			size, err := client.DBSize(ctx).Result()
			if err != nil {
				fmt.Printf("❌ Error: %v\n", err)
				continue
			}
			fmt.Printf("✅ DB size: %d keys\n", size)

		case "FLUSHDB":
			err := client.FlushDB(ctx).Err()
			if err != nil {
				fmt.Printf("❌ Error: %v\n", err)
				continue
			}
			fmt.Println("✅ Database flushed")

		case "PING":
			pong, err := client.Ping(ctx).Result()
			if err != nil {
				fmt.Printf("❌ Error: %v\n", err)
				continue
			}
			fmt.Printf("✅ %s\n", pong)

		case "QUEUE-STATS":
			if len(parts) < 2 {
				fmt.Println("Usage: QUEUE-STATS <queue-name>")
				continue
			}
			queueName := parts[1]
			prefix := "bull"

			waiting, _ := client.LLen(ctx, fmt.Sprintf("%s:%s:wait", prefix, queueName)).Result()
			active, _ := client.LLen(ctx, fmt.Sprintf("%s:%s:active", prefix, queueName)).Result()
			failed, _ := client.SCard(ctx, fmt.Sprintf("%s:%s:failed", prefix, queueName)).Result()
			completed, _ := client.SCard(ctx, fmt.Sprintf("%s:%s:completed", prefix, queueName)).Result()
			delayed, _ := client.ZCard(ctx, fmt.Sprintf("%s:%s:delayed", prefix, queueName)).Result()

			fmt.Printf("✅ Queue Stats for '%s':\n", queueName)
			fmt.Printf("  🕐 Waiting:   %d\n", waiting)
			fmt.Printf("  🚀 Active:    %d\n", active)
			fmt.Printf("  ✅ Completed: %d\n", completed)
			fmt.Printf("  ❌ Failed:    %d\n", failed)
			fmt.Printf("  ⏰ Delayed:   %d\n", delayed)

		case "QUIT", "EXIT":
			fmt.Println("Goodbye!")
			return

		default:
			fmt.Printf("Unknown command: %s\n", cmd)
			fmt.Println("Type 'help' for available commands")
		}
	}
}

func showHelp() {
	fmt.Println(`
Available Commands:
  KEYS <pattern>        - List keys matching pattern (e.g., "bull:*:id")
  GET <key>             - Get string value
  HGETALL <key>         - Get all hash fields
  LLEN <key>            - List length
  LRANGE <key> <s> <e>  - Get list items from start to end
  SCARD <key>           - Set cardinality
  SMEMBERS <key>        - Get all set members
  TYPE <key>            - Get key type
  DBSIZE                - Get total number of keys
  FLUSHDB               - Clear all keys (⚠️ careful!)
  QUEUE-STATS <name>    - Get BullMQ queue statistics
  PING                  - Test connection
  HELP                  - Show this help
  QUIT/EXIT             - Exit

Examples:
  > KEYS bull:*
  > QUEUE-STATS orders
  > HGETALL bull:orders:1
  > LRANGE bull:orders:wait 0 10
`)
}

func truncate(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}

func parseInt(s string) int {
	var i int
	fmt.Sscanf(s, "%d", &i)
	return i
}
