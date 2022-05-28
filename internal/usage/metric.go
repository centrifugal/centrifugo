package usage

import (
	"bytes"
	"crypto/md5"
	"fmt"
	"sort"
)

// metric represents stats point.
type metric struct {
	Id       string   `json:"id"`
	OrgId    int      `json:"org_id"`
	Name     string   `json:"name"`
	Metric   string   `json:"metric"`
	Interval int      `json:"interval"`
	Value    float64  `json:"value"`
	Unit     string   `json:"unit"`
	Time     int64    `json:"time"`
	Type     string   `json:"mtype"`
	Tags     []string `json:"tags"`
}

func (m *metric) SetId() {
	sort.Strings(m.Tags)

	buffer := bytes.NewBufferString(m.Metric)
	buffer.WriteByte(0)
	buffer.WriteString(m.Unit)
	buffer.WriteByte(0)
	buffer.WriteString(m.Type)
	buffer.WriteByte(0)
	_, _ = fmt.Fprintf(buffer, "%d", m.Interval)

	for _, k := range m.Tags {
		buffer.WriteByte(0)
		buffer.WriteString(k)
	}
	m.Id = fmt.Sprintf("%d.%x", m.OrgId, md5.Sum(buffer.Bytes()))
}
