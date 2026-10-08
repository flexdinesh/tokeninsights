package syncjob

import (
	"context"
	"errors"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collector"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
	"testing"
	"time"
)

type receiptDelivery struct {
	collector.Delivery
	receipt evidence.Receipt
	pending int64
	calls   int
}

func (d *receiptDelivery) Receipt(_ context.Context, stream, batch string) (evidence.Response, error) {
	d.calls++
	if stream != d.receipt.StreamID || batch != d.receipt.BatchID {
		return evidence.Response{}, errors.New("queried unrelated receipt")
	}
	return evidence.Response{Receipt: d.receipt, Processing: evidence.Status{Pending: d.pending}}, nil
}
func TestDebugObservesBoundReceiptsAndPreservesTimeout(t *testing.T) {
	receipt := evidence.Receipt{DatabaseID: "database", DatasetID: "alice", StreamID: "stream", BatchID: "batch", RequestHash: "hash", FromSequence: 1, ToSequence: 2, Accepted: 2}
	delivery := &receiptDelivery{receipt: receipt}
	if err := waitReceipts(t.Context(), delivery, []evidence.Receipt{receipt}, nil); err != nil || delivery.calls != 1 {
		t.Fatal(err, delivery.calls)
	}
	delivery.receipt.DatasetID = "bob"
	if err := waitReceipts(t.Context(), delivery, []evidence.Receipt{receipt}, nil); err == nil {
		t.Fatal("receipt switched dataset")
	}
	delivery.receipt = receipt
	delivery.pending = 1
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if err := waitReceipts(ctx, delivery, []evidence.Receipt{receipt}, nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("unfinished processing reported success", err)
	}
}
