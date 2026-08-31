package studio

import (
	"context"
	"database/sql"
	"fmt"
)

type modelQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func loadModelGraph(
	ctx context.Context,
	queryer modelQueryer,
	flowID int64,
) ([]Block, []Connection, error) {
	rows, err := queryer.QueryContext(ctx, `
		SELECT id, flow_id, kind, name, x, y, parameters_json
		FROM blocks WHERE flow_id = ? ORDER BY id`, flowID)
	if err != nil {
		return nil, nil, fmt.Errorf("load blocks: %w", err)
	}
	var blocks []Block
	for rows.Next() {
		var block Block
		var encoded string
		if err := rows.Scan(
			&block.ID, &block.FlowID, &block.Kind, &block.Name,
			&block.Position.X, &block.Position.Y, &encoded,
		); err != nil {
			rows.Close()
			return nil, nil, fmt.Errorf("scan block: %w", err)
		}
		block.Parameters, err = decodeParameters(block.Kind, encoded)
		if err != nil {
			rows.Close()
			return nil, nil, fmt.Errorf("decode parameters for %s: %w", block.Name, err)
		}
		blocks = append(blocks, block)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, nil, fmt.Errorf("iterate blocks: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, nil, fmt.Errorf("close blocks: %w", err)
	}

	rows, err = queryer.QueryContext(ctx, `
		SELECT id, flow_id, source_id, source_port, target_id, target_port
		FROM connections WHERE flow_id = ? ORDER BY id`, flowID)
	if err != nil {
		return nil, nil, fmt.Errorf("load connections: %w", err)
	}
	var connections []Connection
	for rows.Next() {
		var connection Connection
		if err := rows.Scan(
			&connection.ID, &connection.FlowID,
			&connection.SourceID, &connection.SourcePort,
			&connection.TargetID, &connection.TargetPort,
		); err != nil {
			rows.Close()
			return nil, nil, fmt.Errorf("scan connection: %w", err)
		}
		connections = append(connections, connection)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, nil, fmt.Errorf("iterate connections: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, nil, fmt.Errorf("close connections: %w", err)
	}
	return blocks, connections, nil
}
