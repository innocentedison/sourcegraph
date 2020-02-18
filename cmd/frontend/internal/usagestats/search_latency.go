package usagestats

import (
	"context"

	"github.com/sourcegraph/sourcegraph/cmd/frontend/db"
	"github.com/sourcegraph/sourcegraph/cmd/frontend/types"
)

// GetSearchLatencyStatistics return search latency statistics for time spans of X days, Y weeks, and Z months respectively.
func GetSearchLatencyStatistics(ctx context.Context, days int, weeks int, months int) (*types.SearchLatencyStatistics, error) {
	get := func(periodType db.PeriodType) ([]*types.SearchLatencyPeriod, error) {
		var periods int
		switch periodType {
		case db.Daily:
			periods = minIntOrZero(maxStorageDays, days)
		case db.Weekly:
			periods = minIntOrZero(maxStorageDays/7, weeks)
		case db.Monthly:
			periods = minIntOrZero(maxStorageDays/31, months)
		}
		return searchQueryLatency(ctx, periodType, periods)
	}

	daily, err := get(db.Daily)
	if err != nil {
		return nil, err
	}
	weekly, err := get(db.Weekly)
	if err != nil {
		return nil, err
	}
	monthly, err := get(db.Monthly)
	if err != nil {
		return nil, err
	}
	return &types.SearchLatencyStatistics{
		Daily:   daily,
		Weekly:  weekly,
		Monthly: monthly,
	}, nil
}

func searchQueryLatency(ctx context.Context, periodType db.PeriodType, periods int) ([]*types.SearchLatencyPeriod, error) {
	if periods == 0 {
		return []*types.SearchLatencyPeriod{}, nil
	}

	activityPeriods := []*types.SearchLatencyPeriod{}
	for i := 0; i <= periods; i++ {
		activityPeriods = append(activityPeriods, &types.SearchLatencyPeriod{
			Latencies: &types.SearchTypeLatency{
				Literal:    &types.SearchLatency{},
				Regexp:     &types.SearchLatency{},
				Structural: &types.SearchLatency{},
				File:       &types.SearchLatency{},
				Repo:       &types.SearchLatency{},
				Diff:       &types.SearchLatency{},
				Commit:     &types.SearchLatency{},
			},
		})
	}

	latenciesByName := map[string]func(p *types.SearchLatencyPeriod) *types.SearchLatency{
		"search.latencies.literal":    func(p *types.SearchLatencyPeriod) *types.SearchLatency { return p.Latencies.Literal },
		"search.latencies.regexp":     func(p *types.SearchLatencyPeriod) *types.SearchLatency { return p.Latencies.Regexp },
		"search.latencies.structural": func(p *types.SearchLatencyPeriod) *types.SearchLatency { return p.Latencies.Structural },
		"search.latencies.file":       func(p *types.SearchLatencyPeriod) *types.SearchLatency { return p.Latencies.File },
		"search.latencies.repo":       func(p *types.SearchLatencyPeriod) *types.SearchLatency { return p.Latencies.Repo },
		"search.latencies.diff":       func(p *types.SearchLatencyPeriod) *types.SearchLatency { return p.Latencies.Diff },
		"search.latencies.commit":     func(p *types.SearchLatencyPeriod) *types.SearchLatency { return p.Latencies.Commit },
	}

	durationField := "durationMs"
	durationPercentiles := []float64{0.5, 0.9, 0.99}

	for name, getLatencies := range latenciesByName {
		percentiles, err := db.EventLogs.PercentilesPerPeriod(ctx, periodType, timeNow().UTC(), periods, durationField, durationPercentiles, &db.EventFilterOptions{
			ByEventName: name,
		})
		if err != nil {
			return nil, err
		}
		for i, p := range percentiles {
			getLatencies(activityPeriods[i]).P50 = p.Values[0]
			getLatencies(activityPeriods[i]).P90 = p.Values[1]
			getLatencies(activityPeriods[i]).P99 = p.Values[2]
		}
	}

	return activityPeriods, nil
}
