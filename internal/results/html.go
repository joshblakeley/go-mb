package results

import (
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"strings"
)

// chartData is the JSON payload embedded in the <script> block of the HTML report.
// All slice fields are parallel arrays indexed by reporting interval.
type chartData struct {
	Labels            []float64 `json:"labels"`
	PubRateMsgPerSec  []float64 `json:"pubRateMsgPerSec"`
	ConsRateMsgPerSec []float64 `json:"consRateMsgPerSec"`
	PubRateMBPerSec   []float64 `json:"pubRateMBPerSec"`
	ConsRateMBPerSec  []float64 `json:"consRateMBPerSec"`
	PubLatencyAvg     []float64 `json:"pubLatencyAvg"`
	PubLatencyP50     []float64 `json:"pubLatencyP50"`
	PubLatencyP99     []float64 `json:"pubLatencyP99"`
	PubLatencyP999    []float64 `json:"pubLatencyP999"`
	PubLatencyMax     []float64 `json:"pubLatencyMax"`
	E2ELatencyAvg     []float64 `json:"e2eLatencyAvg"`
	E2ELatencyP50     []float64 `json:"e2eLatencyP50"`
	E2ELatencyP99     []float64 `json:"e2eLatencyP99"`
	E2ELatencyP999    []float64 `json:"e2eLatencyP999"`
	E2ELatencyMax     []float64 `json:"e2eLatencyMax"`
}

// templateVars is passed to the HTML template.
type templateVars struct {
	BrokersStr  string
	Topic       string
	Producers   int
	Consumers   int
	MessageSize int
	Duration    string
	Timestamp   string
	Summary     FinalSummary
	ChartJSON   template.JS // pre-marshalled JSON, safe to embed in <script>
}

// WriteHTML renders a self-contained HTML report to path.
// Returns an error if the file cannot be created or the template fails to execute.
func WriteHTML(run *Run, path string) error {
	// Build parallel chart data arrays from DataPoints.
	cd := chartData{}
	for _, p := range run.Points {
		cd.Labels = append(cd.Labels, p.ElapsedSecs)
		cd.PubRateMsgPerSec = append(cd.PubRateMsgPerSec, p.PubRateMsgPerSec)
		cd.ConsRateMsgPerSec = append(cd.ConsRateMsgPerSec, p.ConsRateMsgPerSec)
		cd.PubRateMBPerSec = append(cd.PubRateMBPerSec, p.PubRateMBPerSec)
		cd.ConsRateMBPerSec = append(cd.ConsRateMBPerSec, p.ConsRateMBPerSec)
		cd.PubLatencyAvg = append(cd.PubLatencyAvg, p.PubLatencyAvgMs)
		cd.PubLatencyP50 = append(cd.PubLatencyP50, p.PubLatencyP50Ms)
		cd.PubLatencyP99 = append(cd.PubLatencyP99, p.PubLatencyP99Ms)
		cd.PubLatencyP999 = append(cd.PubLatencyP999, p.PubLatencyP999Ms)
		cd.PubLatencyMax = append(cd.PubLatencyMax, p.PubLatencyMaxMs)
		cd.E2ELatencyAvg = append(cd.E2ELatencyAvg, p.E2ELatencyAvgMs)
		cd.E2ELatencyP50 = append(cd.E2ELatencyP50, p.E2ELatencyP50Ms)
		cd.E2ELatencyP99 = append(cd.E2ELatencyP99, p.E2ELatencyP99Ms)
		cd.E2ELatencyP999 = append(cd.E2ELatencyP999, p.E2ELatencyP999Ms)
		cd.E2ELatencyMax = append(cd.E2ELatencyMax, p.E2ELatencyMaxMs)
	}

	jsonBytes, err := json.Marshal(cd)
	if err != nil {
		return fmt.Errorf("marshal chart data: %w", err)
	}

	vars := templateVars{
		BrokersStr:  strings.Join(run.Meta.Brokers, ", "),
		Topic:       run.Meta.Topic,
		Producers:   run.Meta.Producers,
		Consumers:   run.Meta.Consumers,
		MessageSize: run.Meta.MessageSize,
		Duration:    run.Meta.Duration.String(),
		Timestamp:   run.Meta.Timestamp.Format("2006-01-02 15:04:05"),
		Summary:     run.Summary,
		ChartJSON:   template.JS(jsonBytes),
	}

	tmpl, err := template.New("report").Parse(reportTemplate)
	if err != nil {
		return fmt.Errorf("parse template: %w", err)
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create report file: %w", err)
	}

	if execErr := tmpl.Execute(f, vars); execErr != nil {
		f.Close()
		if rerr := os.Remove(path); rerr != nil {
			return fmt.Errorf("render template: %w; also failed to remove partial file: %v", execErr, rerr)
		}
		return fmt.Errorf("render template: %w", execErr)
	}
	return f.Close()
}

const reportTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<title>go-bench Report</title>
<script src="https://cdn.jsdelivr.net/npm/chart.js@4"></script>
<style>
* { box-sizing: border-box; }
body { font-family: system-ui, sans-serif; max-width: 1200px; margin: 0 auto; padding: 24px; color: #222; }
.meta { background: #f8f8f8; border: 1px solid #e0e0e0; border-radius: 8px; padding: 20px; margin-bottom: 28px; }
.meta h1 { margin: 0 0 12px; font-size: 1.3em; }
.meta-grid { display: grid; grid-template-columns: auto 1fr; gap: 4px 20px; }
.meta-grid dt { font-weight: 600; }
.meta-grid dd { margin: 0; }
.charts { display: grid; grid-template-columns: 1fr 1fr; gap: 24px; margin-bottom: 32px; }
.chart-box { border: 1px solid #e0e0e0; border-radius: 8px; padding: 16px; }
.chart-box h2 { margin: 0 0 12px; font-size: 0.85em; font-weight: 600; color: #555;
  text-transform: uppercase; letter-spacing: 0.05em; }
h2.section { font-size: 1.1em; color: #222; text-transform: none; letter-spacing: 0; margin-bottom: 12px; }
table { border-collapse: collapse; width: 100%; font-size: 0.9em; }
thead th { background: #f0f0f0; font-weight: 600; text-align: right; padding: 8px 12px;
  border-bottom: 2px solid #ccc; }
thead th:first-child { text-align: left; }
tbody td { padding: 8px 12px; text-align: right; border-bottom: 1px solid #eee; }
tbody td:first-child { text-align: left; font-weight: 500; }
</style>
</head>
<body>
<div class="meta">
  <h1>go-bench Report</h1>
  <dl class="meta-grid">
    <dt>Brokers</dt><dd>{{.BrokersStr}}</dd>
    <dt>Topic</dt><dd>{{.Topic}}</dd>
    <dt>Producers</dt><dd>{{.Producers}}</dd>
    <dt>Consumers</dt><dd>{{.Consumers}}</dd>
    <dt>Message Size</dt><dd>{{.MessageSize}} bytes</dd>
    <dt>Duration</dt><dd>{{.Duration}}</dd>
    <dt>Timestamp</dt><dd>{{.Timestamp}}</dd>
  </dl>
</div>
<div class="charts">
  <div class="chart-box"><h2>Publish &amp; Consume Rate (msg/s)</h2><canvas id="c1"></canvas></div>
  <div class="chart-box"><h2>Throughput (MB/s)</h2><canvas id="c2"></canvas></div>
  <div class="chart-box"><h2>Publish Latency (ms)</h2><canvas id="c3"></canvas></div>
  <div class="chart-box"><h2>E2E Latency (ms)</h2><canvas id="c4"></canvas></div>
</div>
<h2 class="section">Aggregated Latency Summary</h2>
<table>
  <thead>
    <tr>
      <th></th>
      <th>Avg</th><th>p50</th><th>p95</th><th>p99</th><th>p99.9</th><th>p99.99</th><th>Max</th>
    </tr>
  </thead>
  <tbody>
    <tr>
      <td>Publish (ms)</td>
      <td>{{printf "%.3f" .Summary.PubAvgMs}}</td>
      <td>{{printf "%.3f" .Summary.PubP50Ms}}</td>
      <td>{{printf "%.3f" .Summary.PubP95Ms}}</td>
      <td>{{printf "%.3f" .Summary.PubP99Ms}}</td>
      <td>{{printf "%.3f" .Summary.PubP999Ms}}</td>
      <td>{{printf "%.3f" .Summary.PubP9999Ms}}</td>
      <td>{{printf "%.3f" .Summary.PubMaxMs}}</td>
    </tr>
    <tr>
      <td>E2E (ms)</td>
      <td>{{printf "%.3f" .Summary.E2EAvgMs}}</td>
      <td>{{printf "%.3f" .Summary.E2EP50Ms}}</td>
      <td>{{printf "%.3f" .Summary.E2EP95Ms}}</td>
      <td>{{printf "%.3f" .Summary.E2EP99Ms}}</td>
      <td>{{printf "%.3f" .Summary.E2EP999Ms}}</td>
      <td>{{printf "%.3f" .Summary.E2EP9999Ms}}</td>
      <td>{{printf "%.3f" .Summary.E2EMaxMs}}</td>
    </tr>
  </tbody>
</table>
<script>
var d = {{.ChartJSON}};
function mkChart(id, yLabel, datasets) {
  new Chart(document.getElementById(id), {
    type: 'line',
    data: {
      labels: d.labels,
      datasets: datasets.map(function(ds) {
        return {label: ds[0], data: d[ds[1]], borderWidth: 1.5, pointRadius: 3, tension: 0.1};
      })
    },
    options: {
      responsive: true,
      plugins: {legend: {position: 'bottom'}},
      scales: {
        x: {title: {display: true, text: 'Elapsed (s)'}},
        y: {title: {display: true, text: yLabel}, beginAtZero: true}
      }
    }
  });
}
mkChart('c1', 'msg/s', [
  ['Publish Rate', 'pubRateMsgPerSec'],
  ['Consume Rate', 'consRateMsgPerSec']
]);
mkChart('c2', 'MB/s', [
  ['Publish', 'pubRateMBPerSec'],
  ['Consume', 'consRateMBPerSec']
]);
mkChart('c3', 'ms', [
  ['Avg', 'pubLatencyAvg'],
  ['p50', 'pubLatencyP50'],
  ['p99', 'pubLatencyP99'],
  ['p99.9', 'pubLatencyP999'],
  ['Max', 'pubLatencyMax']
]);
mkChart('c4', 'ms', [
  ['Avg', 'e2eLatencyAvg'],
  ['p50', 'e2eLatencyP50'],
  ['p99', 'e2eLatencyP99'],
  ['p99.9', 'e2eLatencyP999'],
  ['Max', 'e2eLatencyMax']
]);
</script>
</body>
</html>`
