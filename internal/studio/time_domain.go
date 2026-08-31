package studio

import (
	"math"
	"sort"
)

type timeDomainKind string

const (
	timeDomainNeutral    timeDomainKind = "neutral"
	timeDomainContinuous timeDomainKind = "continuous"
	timeDomainDiscrete   timeDomainKind = "discrete"
)

type sampleTimeMode string

const (
	sampleTimeExplicit  sampleTimeMode = "explicit"
	sampleTimeInherited sampleTimeMode = "inherited"
)

type sampleTimeSpec struct {
	mode    sampleTimeMode
	seconds float64
}

func (spec sampleTimeSpec) resolve(baseStep float64) (float64, error) {
	switch spec.mode {
	case sampleTimeExplicit:
		if math.IsNaN(spec.seconds) || math.IsInf(spec.seconds, 0) || spec.seconds <= 0 {
			return 0, invalid("explicit sample time must be a positive finite number")
		}
		return spec.seconds, nil
	case sampleTimeInherited:
		if math.IsNaN(baseStep) || math.IsInf(baseStep, 0) || baseStep <= 0 {
			return 0, invalid("inherited sample time requires a positive run sample time")
		}
		return baseStep, nil
	default:
		return 0, invalid("sample time mode must be explicit or inherited")
	}
}

type blockTimeDomain struct {
	kind       timeDomainKind
	sampleTime sampleTimeSpec
}

func resolveModelSampleTimes(
	blocks []Block,
	connections []Connection,
	baseStep float64,
) ([]Block, map[int64]float64, error) {
	blockByID := make(map[int64]Block, len(blocks))
	domainByID := make(map[int64]blockTimeDomain, len(blocks))
	orderedIDs := make([]int64, 0, len(blocks))
	resolved := make(map[int64]float64)
	for _, block := range blocks {
		blockByID[block.ID] = block
		orderedIDs = append(orderedIDs, block.ID)
		domain := blockDefinitions[block.Kind].domain(block.Parameters)
		domainByID[block.ID] = domain
		if domain.kind != timeDomainDiscrete || domain.sampleTime.mode != sampleTimeExplicit {
			continue
		}
		sampleTime, err := domain.sampleTime.resolve(0)
		if err != nil {
			return nil, nil, invalid("%s: %s", block.Name, err)
		}
		resolved[block.ID] = sampleTime
	}
	sort.Slice(orderedIDs, func(i, j int) bool { return orderedIDs[i] < orderedIDs[j] })

	neighbors := make(map[int64][]int64, len(blocks))
	for _, connection := range connections {
		source, sourceOK := domainByID[connection.SourceID]
		target, targetOK := domainByID[connection.TargetID]
		if !sourceOK || !targetOK ||
			source.kind == timeDomainContinuous ||
			target.kind == timeDomainContinuous {
			continue
		}
		neighbors[connection.SourceID] = append(
			neighbors[connection.SourceID], connection.TargetID,
		)
		neighbors[connection.TargetID] = append(
			neighbors[connection.TargetID], connection.SourceID,
		)
	}
	for blockID := range neighbors {
		sort.Slice(neighbors[blockID], func(i, j int) bool {
			return neighbors[blockID][i] < neighbors[blockID][j]
		})
	}

	type anchor struct {
		blockID    int64
		sampleTime float64
	}
	visited := make(map[int64]bool, len(blocks))
	for _, start := range orderedIDs {
		if visited[start] || domainByID[start].kind == timeDomainContinuous {
			continue
		}
		visited[start] = true
		queue := []int64{start}
		var anchors []anchor
		var inherited []int64
		for len(queue) > 0 {
			blockID := queue[0]
			queue = queue[1:]
			domain := domainByID[blockID]
			if domain.kind == timeDomainDiscrete {
				switch domain.sampleTime.mode {
				case sampleTimeExplicit:
					anchors = append(anchors, anchor{
						blockID: blockID, sampleTime: resolved[blockID],
					})
				case sampleTimeInherited:
					inherited = append(inherited, blockID)
				}
			}
			for _, neighbor := range neighbors[blockID] {
				if visited[neighbor] {
					continue
				}
				visited[neighbor] = true
				queue = append(queue, neighbor)
			}
		}
		if len(anchors) == 0 || len(inherited) == 0 {
			continue
		}
		selected := anchors[0]
		for _, candidate := range anchors[1:] {
			if compareSampleTimes(selected.sampleTime, candidate.sampleTime).relation == sampleTimesEqual {
				continue
			}
			return nil, nil, invalid(
				"%s inherits from conflicting explicit sample times: %s uses %.12g s and %s uses %.12g s; use one discrete sample time",
				blockByID[inherited[0]].Name,
				blockByID[selected.blockID].Name, selected.sampleTime,
				blockByID[candidate.blockID].Name, candidate.sampleTime,
			)
		}
		for _, blockID := range inherited {
			resolved[blockID] = selected.sampleTime
		}
	}

	compiled := make([]Block, len(blocks))
	for i, block := range blocks {
		domain := blockDefinitions[block.Kind].domain(block.Parameters)
		if domain.kind != timeDomainDiscrete {
			compiled[i] = block
			continue
		}
		sampleTime, ok := resolved[block.ID]
		if !ok {
			var err error
			sampleTime, err = domain.sampleTime.resolve(baseStep)
			if err != nil {
				return nil, nil, invalid("%s: %s", block.Name, err)
			}
			resolved[block.ID] = sampleTime
		}
		block.Parameters.SampleTime = sampleTime
		block.Parameters.SampleTimeMode = string(sampleTimeExplicit)
		compiled[i] = block
	}
	return compiled, resolved, nil
}

func neutralTimeDomain() blockTimeDomain {
	return blockTimeDomain{kind: timeDomainNeutral}
}

func continuousTimeDomain() blockTimeDomain {
	return blockTimeDomain{kind: timeDomainContinuous}
}

func discreteTimeDomain(parameters Parameters) blockTimeDomain {
	return blockTimeDomain{
		kind: timeDomainDiscrete,
		sampleTime: sampleTimeSpec{
			mode:    normalizedSampleTimeMode(parameters),
			seconds: parameters.SampleTime,
		},
	}
}

func normalizedSampleTimeMode(parameters Parameters) sampleTimeMode {
	if parameters.SampleTimeMode == "" {
		return sampleTimeExplicit
	}
	return sampleTimeMode(parameters.SampleTimeMode)
}

type sampleTimeRelation uint8

const (
	sampleTimesEqual sampleTimeRelation = iota
	sampleTimesIntegerMultiple
	sampleTimesIncompatible
)

type sampleTimeCompatibility struct {
	relation sampleTimeRelation
	fast     float64
	slow     float64
	ratio    int
}

type sampleSchedule struct {
	updateEvery int
}

func (schedule sampleSchedule) updatesAt(sample int) bool {
	return sample%schedule.updateEvery == 0
}

func scheduleSampleTime(sampleTime, baseStep float64) (sampleSchedule, error) {
	if math.IsNaN(sampleTime) || math.IsInf(sampleTime, 0) || sampleTime <= 0 {
		return sampleSchedule{}, invalid("sample time must be a positive finite number")
	}
	if math.IsNaN(baseStep) || math.IsInf(baseStep, 0) || baseStep <= 0 {
		return sampleSchedule{}, invalid("run sample time must be a positive finite number")
	}
	samples := sampleTime / baseStep
	nearest := math.Round(samples)
	if nearest < 1 || math.Abs(samples-nearest) > 1e-9 {
		lowerSamples := math.Max(1, math.Floor(samples))
		upperSamples := math.Max(1, math.Ceil(samples))
		return sampleSchedule{}, invalid(
			"sample time %.12g s is not an integer multiple of run sample time %.12g s; use %.12g s or %.12g s",
			sampleTime, baseStep, lowerSamples*baseStep, upperSamples*baseStep,
		)
	}
	return sampleSchedule{updateEvery: int(nearest)}, nil
}

func compareSampleTimes(left, right float64) sampleTimeCompatibility {
	fast, slow := left, right
	if fast > slow {
		fast, slow = slow, fast
	}
	if math.Abs(slow-fast) <= 1e-9*math.Max(1, slow) {
		return sampleTimeCompatibility{
			relation: sampleTimesEqual, fast: fast, slow: slow, ratio: 1,
		}
	}
	ratio := slow / fast
	nearest := math.Round(ratio)
	if nearest >= 1 && math.Abs(ratio-nearest) <= 1e-9 {
		return sampleTimeCompatibility{
			relation: sampleTimesIntegerMultiple,
			fast:     fast,
			slow:     slow,
			ratio:    int(nearest),
		}
	}
	return sampleTimeCompatibility{
		relation: sampleTimesIncompatible, fast: fast, slow: slow,
	}
}
