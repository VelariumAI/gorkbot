package evidence

import "github.com/velariumai/gorkbot/pkg/trace"

func RecordRef(record Record) trace.Ref {
	n := record.Normalized()
	return trace.NewRef(
		"evidence_record",
		"record:"+n.ID,
		trace.StableHash(n.ID, string(n.Kind), string(n.Status), n.Subject),
		int64(len(n.EvidenceRefs)),
	)
}

func ReceiptRef(receipt Receipt) trace.Ref {
	n := receipt.Normalized()
	return trace.NewRef(
		"evidence_receipt",
		"receipt:"+n.ID,
		trace.StableHash(n.ID, string(n.Status), n.Assessment.ID),
		int64(len(n.Records)+len(n.EvidenceRefs)),
	)
}

func AssessmentRef(assessment Assessment) trace.Ref {
	n := Evaluate(assessment)
	return trace.NewRef(
		"evidence_assessment",
		"assessment:"+n.ID,
		trace.StableHash(n.ID, string(n.PolicyState), string(n.Risk), string(n.Decision), n.ReasonCode),
		int64(len(n.EvidenceRefs)),
	)
}
