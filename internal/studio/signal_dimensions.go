package studio

func inheritsSignalWidth(block Block) bool {
	if block.Kind != BlockGain && block.Kind != BlockUnitDelay {
		return false
	}
	return normalizedSignalWidthMode(block.Parameters) == signalWidthInherited
}

func resolveModelSignalWidths(
	blocks []Block,
	connections []Connection,
) ([]Block, error) {
	resolved := make([]Block, len(blocks))
	indexByID := make(map[int64]int, len(blocks))
	widthByID := make(map[int64]int)
	initialWidthByID := make(map[int64]int)
	for i, block := range blocks {
		block.Parameters = cloneParameters(block.Parameters)
		block.resolvedSignalWidth = 0
		resolved[i] = block
		indexByID[block.ID] = i
		if block.Kind == BlockUnitDelay && inheritsSignalWidth(block) &&
			block.Parameters.InitialState != nil &&
			block.Parameters.InitialState.Len() > 1 {
			widthByID[block.ID] = block.Parameters.InitialState.Len()
			initialWidthByID[block.ID] = block.Parameters.InitialState.Len()
		}
	}

	for {
		changed := false
		for _, connection := range connections {
			targetIndex, targetOK := indexByID[connection.TargetID]
			sourceIndex, sourceOK := indexByID[connection.SourceID]
			if !targetOK || !sourceOK {
				continue
			}
			target := resolved[targetIndex]
			if !inheritsSignalWidth(target) {
				continue
			}
			source := resolved[sourceIndex]
			sourceWidth := 0
			if inheritsSignalWidth(source) {
				sourceWidth = widthByID[source.ID]
			} else if port, ok := source.OutputPort(connection.SourcePort); ok {
				sourceWidth = port.Width
			}
			if sourceWidth == 0 {
				continue
			}
			if targetWidth := widthByID[target.ID]; targetWidth != 0 {
				if initialWidth := initialWidthByID[target.ID]; initialWidth != 0 &&
					initialWidth != sourceWidth {
					return nil, invalid(
						"%s inherits %d channels from %s but its initial condition requires %d",
						target.Name, sourceWidth, source.Name, initialWidth,
					)
				}
				continue
			}
			widthByID[target.ID] = sourceWidth
			changed = true
		}
		if !changed {
			break
		}
	}

	for i, block := range resolved {
		if !inheritsSignalWidth(block) {
			continue
		}
		width := widthByID[block.ID]
		if width == 0 {
			width = 1
		}
		if width < 1 || width > maxDirectSignalWidth {
			return nil, invalid(
				"%s inherits %d channels; supported widths are 1 to %d",
				block.Name, width, maxDirectSignalWidth,
			)
		}
		block.resolvedSignalWidth = width
		resolved[i] = block
	}

	byID := make(map[int64]Block, len(resolved))
	for _, block := range resolved {
		byID[block.ID] = block
	}
	for _, connection := range connections {
		source, sourceOK := byID[connection.SourceID]
		target, targetOK := byID[connection.TargetID]
		// Endpoint and port validity stay with callers so snapshots can open legacy
		// partial graphs while Connect, documents, and compile reject them.
		if !sourceOK || !targetOK {
			continue
		}
		if !source.hasOutputPort(connection.SourcePort) ||
			!target.hasInputPort(connection.TargetPort) {
			continue
		}
		if err := validateConnectionWidth(
			source, connection.SourcePort, target, connection.TargetPort,
		); err != nil {
			return nil, err
		}
	}
	return resolved, nil
}
