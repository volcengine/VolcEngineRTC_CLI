// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package telemetry

import "sync"

var invocationStore struct {
	sync.RWMutex
	context *InvocationContext
}

func InitializeInvocation(options ResolveInvocationContextOptions) InvocationContext {
	invocationStore.Lock()
	defer invocationStore.Unlock()
	if invocationStore.context != nil {
		return *invocationStore.context
	}
	context := ResolveInvocationContext(options)
	invocationStore.context = &context
	return context
}

func GetInvocationUserAgent() (string, bool) {
	invocationStore.RLock()
	if invocationStore.context == nil {
		invocationStore.RUnlock()
		return "", false
	}
	context := *invocationStore.context
	invocationStore.RUnlock()
	return BuildInvocationUserAgent(&context)
}

func resetInvocationForTest() {
	invocationStore.Lock()
	invocationStore.context = nil
	invocationStore.Unlock()
}
