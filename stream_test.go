package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func TestStartExecutorStreamRequiresStreamID(t *testing.T) {
	p := testPlugin(t, baseTestConfig)
	raw, err := p.startExecutorStream(rpcExecutorRequest{ExecutorRequest: executorRequest("claude", true)})
	if err != nil {
		t.Fatalf("startExecutorStream() error = %v", err)
	}
	var env abiEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if env.OK || env.Error == nil || env.Error.Code != "executor_error" {
		t.Fatalf("envelope = %#v", env)
	}
}

func TestRunStreamFallbackOnSetup429BeforeFirstChunk(t *testing.T) {
	p := testPlugin(t, baseTestConfig)
	execCalls := 0
	closedModelStreams := []string{}
	withHostCallback(t, func(method string, payload any) (json.RawMessage, error) {
		switch method {
		case pluginabi.MethodHostModelExecuteStream:
			execCalls++
			if execCalls == 1 {
				return rawJSON(t, pluginapi.HostModelStreamResponse{StatusCode: http.StatusTooManyRequests, StreamID: "s1"}), nil
			}
			return rawJSON(t, pluginapi.HostModelStreamResponse{StatusCode: http.StatusOK, StreamID: "s2"}), nil
		case pluginabi.MethodHostModelStreamRead:
			return rawJSON(t, pluginapi.HostModelStreamReadResponse{Done: true}), nil
		case pluginabi.MethodHostModelStreamClose:
			closedModelStreams = append(closedModelStreams, payload.(pluginapi.HostModelStreamCloseRequest).StreamID)
			return rawJSON(t, map[string]any{}), nil
		default:
			return rawJSON(t, map[string]any{}), nil
		}
	})

	if err := p.runStreamOrchestration(nil, executorRequest("claude", true), "callback", "plugin-stream"); err != nil {
		t.Fatalf("runStreamOrchestration() error = %v", err)
	}
	if execCalls != 2 || len(closedModelStreams) != 2 {
		t.Fatalf("execCalls/closed = %d/%v", execCalls, closedModelStreams)
	}
}

func TestRunStreamFallbackOnReadErrorBeforeFirstChunk(t *testing.T) {
	p := testPlugin(t, baseTestConfig)
	execCalls := 0
	readCalls := map[string]int{}
	withHostCallback(t, func(method string, payload any) (json.RawMessage, error) {
		switch method {
		case pluginabi.MethodHostModelExecuteStream:
			execCalls++
			return rawJSON(t, pluginapi.HostModelStreamResponse{StatusCode: http.StatusOK, StreamID: []string{"s1", "s2"}[execCalls-1]}), nil
		case pluginabi.MethodHostModelStreamRead:
			streamID := payload.(pluginapi.HostModelStreamReadRequest).StreamID
			readCalls[streamID]++
			if streamID == "s1" {
				return nil, errors.New("read failed with status 503")
			}
			return rawJSON(t, pluginapi.HostModelStreamReadResponse{Done: true}), nil
		default:
			return rawJSON(t, map[string]any{}), nil
		}
	})

	if err := p.runStreamOrchestration(nil, executorRequest("claude", true), "callback", "plugin-stream"); err != nil {
		t.Fatalf("runStreamOrchestration() error = %v", err)
	}
	if execCalls != 2 || readCalls["s1"] != 1 || readCalls["s2"] != 1 {
		t.Fatalf("execCalls/readCalls = %d/%v", execCalls, readCalls)
	}
}

func TestRunStreamDoesNotFallbackAfterFirstEmittedChunk(t *testing.T) {
	p := testPlugin(t, baseTestConfig)
	execCalls := 0
	readCalls := 0
	emitted := 0
	withHostCallback(t, func(method string, payload any) (json.RawMessage, error) {
		switch method {
		case pluginabi.MethodHostModelExecuteStream:
			execCalls++
			return rawJSON(t, pluginapi.HostModelStreamResponse{StatusCode: http.StatusOK, StreamID: "s1"}), nil
		case pluginabi.MethodHostModelStreamRead:
			readCalls++
			if readCalls == 1 {
				return rawJSON(t, pluginapi.HostModelStreamReadResponse{Payload: []byte("chunk")}), nil
			}
			return nil, errors.New("read failed with status 503")
		case pluginabi.MethodHostStreamEmit:
			emitted++
			return rawJSON(t, map[string]any{}), nil
		default:
			return rawJSON(t, map[string]any{}), nil
		}
	})

	err := p.runStreamOrchestration(nil, executorRequest("claude", true), "callback", "plugin-stream")
	if err == nil {
		t.Fatalf("runStreamOrchestration() error = nil, want terminal error")
	}
	if execCalls != 1 || emitted != 1 {
		t.Fatalf("execCalls/emitted = %d/%d, want 1/1", execCalls, emitted)
	}
}
