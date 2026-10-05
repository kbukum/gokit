package bench

import (
	"context"
	"errors"
	"slices"
	"strings"
)

func distributions(ctx context.Context, w *runWriter, branch int) (out []ScoreDistribution, omitted bool, err error) {
	labels := make(map[string]*ScoreDistribution)
	it := w.iterator(branch)
	defer func() { err = errors.Join(err, it.Close()) }()
	for {
		record, ok, err := it.Next(ctx)
		if err != nil {
			return nil, false, err
		}
		if !ok {
			break
		}
		key := record.LabelText()
		dist := labels[key]
		if dist == nil {
			if len(labels) >= w.limits.MaxLabels {
				return nil, true, nil
			}
			dist = &ScoreDistribution{Label: key, Bins: make([]float64, 11), Counts: make([]int, 10)}
			for i := range dist.Bins {
				dist.Bins[i] = float64(i) / 10
			}
			labels[key] = dist
		}
		index := min(9, max(0, int(record.Score*10)))
		dist.Counts[index]++
	}
	for _, dist := range labels {
		out = append(out, *dist)
	}
	slices.SortFunc(out, func(a, b ScoreDistribution) int { return strings.Compare(a.Label, b.Label) })
	return out, false, nil
}
