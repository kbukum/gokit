package bench

import (
	"context"
	"errors"
	"fmt"

	"github.com/kbukum/gokit/stream"
)

// CompareSamples adds exact counts and at most 100 IDs per change, without retaining datasets.
func CompareSamples(ctx context.Context, baseStore, targetStore *ResultStore, base, target *RunResult) (diff *RunDiff, err error) {
	diff = NewRunComparator().Compare(base, target)
	if !diff.Eligibility.Eligible {
		return diff, nil
	}
	a, err := baseStore.Records(ctx, base.ID, 0)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, a.Close()) }()
	b, err := targetStore.Records(ctx, target.ID, 0)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, b.Close()) }()
	for {
		aid, acorrect, aok, err := nextSample(ctx, a, base.Evaluation.Execution.Repeats)
		if err != nil {
			return nil, err
		}
		bid, bcorrect, bok, err := nextSample(ctx, b, target.Evaluation.Execution.Repeats)
		if err != nil {
			return nil, err
		}
		if aok != bok || aid != bid {
			return nil, fmt.Errorf("bench: stored sample sets differ despite matching identity")
		}
		if !aok {
			break
		}
		if !acorrect && bcorrect {
			diff.FixedCount++
			if len(diff.Fixed) < 100 {
				diff.Fixed = append(diff.Fixed, aid)
			}
		}
		if acorrect && !bcorrect {
			diff.RegressedCount++
			if len(diff.Regressed) < 100 {
				diff.Regressed = append(diff.Regressed, bid)
			}
		}
	}
	return diff, nil
}

func nextSample(ctx context.Context, it stream.Iterator[Record], repeats int) (sampleID string, allCorrect bool, available bool, err error) {
	id, correct := "", true
	for repeat := 0; repeat < repeats; repeat++ {
		record, ok, err := it.Next(ctx)
		if err != nil {
			return "", false, false, err
		}
		if !ok {
			if repeat != 0 {
				return "", false, false, fmt.Errorf("bench: truncated repeat group")
			}
			return "", false, false, nil
		}
		if repeat == 0 {
			id = record.ID
		}
		if record.ID != id || record.Repeat != repeat {
			return "", false, false, fmt.Errorf("bench: invalid repeat ordering")
		}
		correct = correct && record.Correct && record.Error == ""
	}
	return id, correct, true, nil
}
