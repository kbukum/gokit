package metric

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/kbukum/gokit/bench"
)

func offlineIdentity(name string, config map[string]string) bench.MetricIdentity {
	return bench.MetricIdentity{Name: name, Version: "1", Config: config}
}
func (m *rSquared) Identity() bench.MetricIdentity { return offlineIdentity(m.Name(), nil) }
func (m *ndcg[L]) Identity() bench.MetricIdentity {
	return offlineIdentity(m.Name(), map[string]string{"k": strconv.Itoa(m.k)})
}

func (m *meanAveragePrecision[L]) Identity() bench.MetricIdentity {
	return offlineIdentity(m.Name(), map[string]string{"positive": fmt.Sprintf("%#v", m.positive)})
}

func (m *precisionAtK[L]) Identity() bench.MetricIdentity {
	return offlineIdentity(m.Name(), map[string]string{"positive": fmt.Sprintf("%#v", m.positive), "k": strconv.Itoa(m.k)})
}

func (m *recallAtK[L]) Identity() bench.MetricIdentity {
	return offlineIdentity(m.Name(), map[string]string{"positive": fmt.Sprintf("%#v", m.positive), "k": strconv.Itoa(m.k)})
}

func (m *binaryClassification[L]) Identity() bench.MetricIdentity {
	return offlineIdentity(m.Name(), map[string]string{"positive": fmt.Sprintf("%#v", m.positive), "threshold": formatThreshold(m.threshold)})
}

func (m *confusionMatrix[L]) Identity() bench.MetricIdentity {
	return offlineIdentity(m.Name(), map[string]string{"labels": fmt.Sprintf("%#v", m.labels)})
}

func (m *multiClassClassification[L]) Identity() bench.MetricIdentity {
	return offlineIdentity(m.Name(), map[string]string{"labels": fmt.Sprintf("%#v", m.labels)})
}

func (m *thresholdSweep[L]) Identity() bench.MetricIdentity {
	return offlineIdentity(m.Name(), map[string]string{"positive": fmt.Sprintf("%#v", m.positive), "thresholds": fmt.Sprintf("%v", m.thresholds)})
}

func (m *aucroc[L]) Identity() bench.MetricIdentity {
	return offlineIdentity(m.Name(), map[string]string{"positive": fmt.Sprintf("%#v", m.positive)})
}

func (m *brierScore[L]) Identity() bench.MetricIdentity {
	return offlineIdentity(m.Name(), map[string]string{"positive": fmt.Sprintf("%#v", m.positive)})
}

func (m *logLoss[L]) Identity() bench.MetricIdentity {
	return offlineIdentity(m.Name(), map[string]string{"positive": fmt.Sprintf("%#v", m.positive)})
}

func (m *calibration[L]) Identity() bench.MetricIdentity {
	return offlineIdentity(m.Name(), map[string]string{"positive": fmt.Sprintf("%#v", m.positive), "bins": strconv.Itoa(m.bins)})
}

func (m *tokenStats[L]) Identity() bench.MetricIdentity {
	return offlineIdentity(m.Name(), map[string]string{"counter": m.counter.Name()})
}

func (m *semanticSimilarity[L]) Identity() bench.MetricIdentity {
	return offlineIdentity(m.Name(), map[string]string{"model": modelIdentity(m.model, m.provider), "threshold": formatThreshold(m.threshold)})
}

func (m *llmJudge[L]) Identity() bench.MetricIdentity {
	return offlineIdentity(m.Name(), map[string]string{"model": m.model, "provider": m.provider.Name(), "prompt": m.prompt.fingerprint(), "threshold": formatThreshold(m.threshold)})
}

func (m *weightedMetric[L]) Identity() bench.MetricIdentity {
	config := make(map[string]string)
	for _, e := range m.entries {
		identity := e.metric.Identity()
		raw, err := json.Marshal(identity)
		if err != nil {
			return bench.MetricIdentity{Name: m.Name()}
		}
		config[identity.Name] = string(raw) + ":" + strconv.FormatFloat(e.weight, 'g', -1, 64)
	}
	return offlineIdentity(m.Name(), config)
}
