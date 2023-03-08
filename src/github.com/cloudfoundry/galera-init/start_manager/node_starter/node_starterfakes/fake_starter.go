// Code generated by counterfeiter. DO NOT EDIT.
package node_starterfakes

import (
	"os/exec"
	"sync"

	"github.com/cloudfoundry/galera-init/start_manager/node_starter"
)

type FakeStarter struct {
	GetMysqlCmdStub        func() *exec.Cmd
	getMysqlCmdMutex       sync.RWMutex
	getMysqlCmdArgsForCall []struct {
	}
	getMysqlCmdReturns struct {
		result1 *exec.Cmd
	}
	getMysqlCmdReturnsOnCall map[int]struct {
		result1 *exec.Cmd
	}
	StartNodeFromStateStub        func(string) (string, <-chan error, error)
	startNodeFromStateMutex       sync.RWMutex
	startNodeFromStateArgsForCall []struct {
		arg1 string
	}
	startNodeFromStateReturns struct {
		result1 string
		result2 <-chan error
		result3 error
	}
	startNodeFromStateReturnsOnCall map[int]struct {
		result1 string
		result2 <-chan error
		result3 error
	}
	invocations      map[string][][]interface{}
	invocationsMutex sync.RWMutex
}

func (fake *FakeStarter) GetMysqlCmd() *exec.Cmd {
	fake.getMysqlCmdMutex.Lock()
	ret, specificReturn := fake.getMysqlCmdReturnsOnCall[len(fake.getMysqlCmdArgsForCall)]
	fake.getMysqlCmdArgsForCall = append(fake.getMysqlCmdArgsForCall, struct {
	}{})
	stub := fake.GetMysqlCmdStub
	fakeReturns := fake.getMysqlCmdReturns
	fake.recordInvocation("GetMysqlCmd", []interface{}{})
	fake.getMysqlCmdMutex.Unlock()
	if stub != nil {
		return stub()
	}
	if specificReturn {
		return ret.result1
	}
	return fakeReturns.result1
}

func (fake *FakeStarter) GetMysqlCmdCallCount() int {
	fake.getMysqlCmdMutex.RLock()
	defer fake.getMysqlCmdMutex.RUnlock()
	return len(fake.getMysqlCmdArgsForCall)
}

func (fake *FakeStarter) GetMysqlCmdCalls(stub func() *exec.Cmd) {
	fake.getMysqlCmdMutex.Lock()
	defer fake.getMysqlCmdMutex.Unlock()
	fake.GetMysqlCmdStub = stub
}

func (fake *FakeStarter) GetMysqlCmdReturns(result1 *exec.Cmd) {
	fake.getMysqlCmdMutex.Lock()
	defer fake.getMysqlCmdMutex.Unlock()
	fake.GetMysqlCmdStub = nil
	fake.getMysqlCmdReturns = struct {
		result1 *exec.Cmd
	}{result1}
}

func (fake *FakeStarter) GetMysqlCmdReturnsOnCall(i int, result1 *exec.Cmd) {
	fake.getMysqlCmdMutex.Lock()
	defer fake.getMysqlCmdMutex.Unlock()
	fake.GetMysqlCmdStub = nil
	if fake.getMysqlCmdReturnsOnCall == nil {
		fake.getMysqlCmdReturnsOnCall = make(map[int]struct {
			result1 *exec.Cmd
		})
	}
	fake.getMysqlCmdReturnsOnCall[i] = struct {
		result1 *exec.Cmd
	}{result1}
}

func (fake *FakeStarter) StartNodeFromState(arg1 string) (string, <-chan error, error) {
	fake.startNodeFromStateMutex.Lock()
	ret, specificReturn := fake.startNodeFromStateReturnsOnCall[len(fake.startNodeFromStateArgsForCall)]
	fake.startNodeFromStateArgsForCall = append(fake.startNodeFromStateArgsForCall, struct {
		arg1 string
	}{arg1})
	stub := fake.StartNodeFromStateStub
	fakeReturns := fake.startNodeFromStateReturns
	fake.recordInvocation("StartNodeFromState", []interface{}{arg1})
	fake.startNodeFromStateMutex.Unlock()
	if stub != nil {
		return stub(arg1)
	}
	if specificReturn {
		return ret.result1, ret.result2, ret.result3
	}
	return fakeReturns.result1, fakeReturns.result2, fakeReturns.result3
}

func (fake *FakeStarter) StartNodeFromStateCallCount() int {
	fake.startNodeFromStateMutex.RLock()
	defer fake.startNodeFromStateMutex.RUnlock()
	return len(fake.startNodeFromStateArgsForCall)
}

func (fake *FakeStarter) StartNodeFromStateCalls(stub func(string) (string, <-chan error, error)) {
	fake.startNodeFromStateMutex.Lock()
	defer fake.startNodeFromStateMutex.Unlock()
	fake.StartNodeFromStateStub = stub
}

func (fake *FakeStarter) StartNodeFromStateArgsForCall(i int) string {
	fake.startNodeFromStateMutex.RLock()
	defer fake.startNodeFromStateMutex.RUnlock()
	argsForCall := fake.startNodeFromStateArgsForCall[i]
	return argsForCall.arg1
}

func (fake *FakeStarter) StartNodeFromStateReturns(result1 string, result2 <-chan error, result3 error) {
	fake.startNodeFromStateMutex.Lock()
	defer fake.startNodeFromStateMutex.Unlock()
	fake.StartNodeFromStateStub = nil
	fake.startNodeFromStateReturns = struct {
		result1 string
		result2 <-chan error
		result3 error
	}{result1, result2, result3}
}

func (fake *FakeStarter) StartNodeFromStateReturnsOnCall(i int, result1 string, result2 <-chan error, result3 error) {
	fake.startNodeFromStateMutex.Lock()
	defer fake.startNodeFromStateMutex.Unlock()
	fake.StartNodeFromStateStub = nil
	if fake.startNodeFromStateReturnsOnCall == nil {
		fake.startNodeFromStateReturnsOnCall = make(map[int]struct {
			result1 string
			result2 <-chan error
			result3 error
		})
	}
	fake.startNodeFromStateReturnsOnCall[i] = struct {
		result1 string
		result2 <-chan error
		result3 error
	}{result1, result2, result3}
}

func (fake *FakeStarter) Invocations() map[string][][]interface{} {
	fake.invocationsMutex.RLock()
	defer fake.invocationsMutex.RUnlock()
	fake.getMysqlCmdMutex.RLock()
	defer fake.getMysqlCmdMutex.RUnlock()
	fake.startNodeFromStateMutex.RLock()
	defer fake.startNodeFromStateMutex.RUnlock()
	copiedInvocations := map[string][][]interface{}{}
	for key, value := range fake.invocations {
		copiedInvocations[key] = value
	}
	return copiedInvocations
}

func (fake *FakeStarter) recordInvocation(key string, args []interface{}) {
	fake.invocationsMutex.Lock()
	defer fake.invocationsMutex.Unlock()
	if fake.invocations == nil {
		fake.invocations = map[string][][]interface{}{}
	}
	if fake.invocations[key] == nil {
		fake.invocations[key] = [][]interface{}{}
	}
	fake.invocations[key] = append(fake.invocations[key], args)
}

var _ node_starter.Starter = new(FakeStarter)
