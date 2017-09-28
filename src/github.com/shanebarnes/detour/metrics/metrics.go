package metrics

import (
    "fmt"
    "log"
    "time"
)

type Metrics struct {
    logger      *log.Logger
    timeStartNs  int64
    timeReportNs int64 // Last report time
    reportIntNs  int64 // Report interval in nanoseconds
    byteCount    int64
    tag          string
    buffer       []string
}

func New(logger *log.Logger, reportIntNs int64, tag string) *Metrics {
    metrics := new(Metrics)
    metrics.logger = logger
    metrics.timeStartNs = time.Now().UnixNano()
    metrics.timeReportNs = metrics.timeStartNs
    metrics.reportIntNs = reportIntNs
    metrics.tag = tag
    metrics.byteCount = 0
    metrics.Add(0)
    return metrics
}

func (m *Metrics) Add(bytes int64) {
    now := time.Now().UnixNano()
    m.byteCount = m.byteCount + bytes

    if now >= m.timeReportNs {
        avgBps := float64(m.byteCount) * 8. * 1000000000. / float64(now - m.timeStartNs)
        m.buffer = append(m.buffer, fmt.Sprintf("%.6f,%s,%d,%d,%d,%.6f", float64(now) / 1000000000., m.tag, (now - m.timeStartNs) / 1000, bytes, m.byteCount, avgBps / 1000000.))

        for m.timeReportNs <= now {
            m.timeReportNs = m.timeReportNs + m.reportIntNs
        }
    }
}

func (m *Metrics) Dump() {
    m.timeReportNs = time.Now().UnixNano()
    m.Add(0)

    for _, record := range m.buffer {
        m.logger.Println(record)
    }
}
