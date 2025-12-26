package consuming

import (
	"context"
	"fmt"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/twmb/franz-go/pkg/kgo"
)

func (pc *partitionConsumer) isMarketDataTopic(topic string) bool {
	// All market.* topics need special handling
	return strings.HasPrefix(topic, "market.")
}

func (pc *partitionConsumer) processMarketDataRecord(ctx context.Context, record *kgo.Record) error {
	// Use gjson to parse JSON without unmarshaling - much faster
	result := gjson.ParseBytes(record.Value)

	// Get the data object as raw JSON
	dataField := result.Get("data")
	if !dataField.Exists() {
		return fmt.Errorf("'data' field not found in message")
	}

	// Determine channel based on topic type
	var channel string
	switch record.Topic {
	case "market.quote", "market.bidoffer", "market.quote.oddlot":
		// Extract symbol field 's' without unmarshaling
		symbol := dataField.Get("s").String()
		if symbol == "" {
			return fmt.Errorf("symbol field 's' is empty in market data")
		}

		// Channel: {topic}.{symbol}
		channel = record.Topic + "." + symbol

	case "market.dealNotice", "market.advertised":
		// Extract market field 'm' without unmarshaling
		market := dataField.Get("m").String()
		if market == "" {
			return fmt.Errorf("market field 'm' is empty in market data")
		}

		// Channel: {topic}.{market}
		channel = record.Topic + "." + market

	default:
		// Other market.* topics use topic name as channel
		channel = record.Topic
	}

	// Build JSON payload manually without marshaling - much faster
	// Using string builder for efficient string concatenation
	var sb strings.Builder
	sb.Grow(len(channel) + len(dataField.Raw) + 32) // Pre-allocate capacity
	sb.WriteString(`{"channel":"`)
	sb.WriteString(channel)
	sb.WriteString(`","data":`)
	sb.WriteString(dataField.Raw)
	sb.WriteByte('}')

	return pc.dispatcher.DispatchCommand(ctx, "publish", []byte(sb.String()))
}
