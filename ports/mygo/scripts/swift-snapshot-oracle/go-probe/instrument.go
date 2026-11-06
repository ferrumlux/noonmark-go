// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/canonical"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/v7"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/recurrence"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

// Counts are operation-entry witnesses, not estimates of internal allocations.
// The same wrappers serve normal captures and fixed mutation executions.
var operationCalls = map[string]int{"decode": 0, "clone": 0, "bridge": 0, "command": 0, "serializer": 0}

func resetCalls() {
	for k := range operationCalls {
		operationCalls[k] = 0
	}
}
func domainDecode(v canonical.Value) (recurrence.State, v7.Supplement, error) {
	operationCalls["decode"]++
	ingressEvent("decode")
	return v7.ToDomain(v)
}
func kernelNew(g core.TaskGraph) (*core.TaskKernel, error) {
	operationCalls["clone"]++
	ingressEvent("clone")
	return core.NewTaskKernel(g)
}
func kernelSnapshot(k *core.TaskKernel) core.TaskGraph {
	operationCalls["clone"]++
	ingressEvent("clone")
	return k.Snapshot()
}
func domainBridge(s recurrence.State, x v7.Supplement) (canonical.Value, error) {
	operationCalls["bridge"]++
	ingressEvent("bridge")
	return v7.FromDomain(s, x)
}
func codecEncode(c *v7.Codec, s canonical.Value) ([]byte, error) {
	operationCalls["serializer"]++
	ingressEvent("serializer")
	return c.Encode(s)
}
func codecDecode(c *v7.Codec, b []byte) (v7.Document, error) {
	operationCalls["decode"]++
	ingressEvent("decode")
	return c.Decode(b)
}
func createCommand(k *core.TaskKernel, in core.CreatePoolTaskInput) error {
	operationCalls["command"]++
	ingressEvent("command")
	return k.CreatePoolTask(in)
}
func scheduleCommand(k *core.TaskKernel, in core.ScheduleFromPoolInput) error {
	operationCalls["command"]++
	ingressEvent("command")
	return k.ScheduleFromPool(in)
}
