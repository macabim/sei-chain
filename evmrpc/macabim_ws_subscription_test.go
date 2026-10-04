package evmrpc

import (
	"context"
	"testing"
	"time"
)

func TestMacabimSubscriptionContext(t *testing.T) {
	type key struct{}
	request, endRequest := context.WithCancel(context.WithValue(context.Background(), key{}, "request value"))
	ended := make(chan error)
	subscription, cancel := bindSubscriptionContext(request, ended)
	defer cancel()
	endRequest()
	if subscription.Err() != nil {
		t.Fatal("RPC return cancels the subscription")
	}
	if subscription.Value(key{}) != "request value" {
		t.Fatal("request values are lost")
	}
	close(ended)
	select {
	case <-subscription.Done():
	case <-time.After(time.Second):
		t.Fatal("unsubscribe does not cancel subscription work")
	}
}

func TestMacabimSubscriptionWorkerExit(t *testing.T) {
	subscription, cancel := bindSubscriptionContext(context.Background(), make(chan error))
	cancel()
	select {
	case <-subscription.Done():
	case <-time.After(time.Second):
		t.Fatal("completed work does not release subscription context")
	}
}
