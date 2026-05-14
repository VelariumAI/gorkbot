package sense

import "testing"

func BenchmarkSENSETracerDisabled(b *testing.B) {
	tr := &SENSETracer{disabled: true}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		tr.LogToolSuccess("read_file", "{}", "ok", 1)
	}
}
