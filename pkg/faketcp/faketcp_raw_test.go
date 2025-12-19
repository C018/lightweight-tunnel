package faketcp

import "testing"

func TestRawRecvQueueSize(t *testing.T) {
	if rawRecvQueueSize < 2048 {
		t.Fatalf("rawRecvQueueSize too small: %d", rawRecvQueueSize)
	}
}
