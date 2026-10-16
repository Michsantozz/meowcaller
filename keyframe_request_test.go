package meowcaller

import "testing"

func TestRequestVideoKeyframeNeedsActiveMedia(t *testing.T) {
	eng, call := testEngineWithOutgoingCall()
	if err := call.RequestVideoKeyframe(); err == nil {
		t.Fatal("RequestVideoKeyframe before media started returned nil error")
	}

	requests := 0
	eng.mu.Lock()
	eng.calls["CID"].requestKeyframe = func() { requests++ }
	eng.mu.Unlock()
	if err := call.RequestVideoKeyframe(); err != nil {
		t.Fatalf("RequestVideoKeyframe with live media: %v", err)
	}
	if requests != 1 {
		t.Fatalf("media loop received %d keyframe requests, want 1", requests)
	}
}
