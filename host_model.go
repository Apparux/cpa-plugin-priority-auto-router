package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

type hostModelExecutionRequest struct {
	pluginapi.HostModelExecutionRequest
	HostCallbackID string `json:"host_callback_id,omitempty"`
}

type rpcStreamEmitRequest struct {
	StreamID string `json:"stream_id"`
	Payload  []byte `json:"payload,omitempty"`
	Error    string `json:"error,omitempty"`
}

type rpcStreamCloseRequest struct {
	StreamID string `json:"stream_id"`
	Error    string `json:"error,omitempty"`
}

type hostLogRequest struct {
	HostCallbackID string         `json:"host_callback_id,omitempty"`
	Level          string         `json:"level,omitempty"`
	Message        string         `json:"message,omitempty"`
	Fields         map[string]any `json:"fields,omitempty"`
}

var callHostCallback = callHost

func callHostModelExecute(req pluginapi.ExecutorRequest, candidate Candidate, hostCallbackID string) (pluginapi.HostModelExecutionResponse, error) {
	raw, err := callHostCallback(pluginabi.MethodHostModelExecute, buildHostModelExecutionRequest(req, candidate, false, hostCallbackID))
	if err != nil {
		return pluginapi.HostModelExecutionResponse{}, err
	}
	var resp pluginapi.HostModelExecutionResponse
	if errDecode := json.Unmarshal(raw, &resp); errDecode != nil {
		return pluginapi.HostModelExecutionResponse{}, fmt.Errorf("decode host.model.execute result: %w", errDecode)
	}
	return resp, nil
}

func callHostModelExecuteStream(req pluginapi.ExecutorRequest, candidate Candidate, hostCallbackID string) (pluginapi.HostModelStreamResponse, error) {
	raw, err := callHostCallback(pluginabi.MethodHostModelExecuteStream, buildHostModelExecutionRequest(req, candidate, true, hostCallbackID))
	if err != nil {
		return pluginapi.HostModelStreamResponse{}, err
	}
	var resp pluginapi.HostModelStreamResponse
	if errDecode := json.Unmarshal(raw, &resp); errDecode != nil {
		return pluginapi.HostModelStreamResponse{}, fmt.Errorf("decode host.model.execute_stream result: %w", errDecode)
	}
	return resp, nil
}

func buildHostModelExecutionRequest(req pluginapi.ExecutorRequest, candidate Candidate, stream bool, hostCallbackID string) hostModelExecutionRequest {
	protocol := normalizeProtocol(firstNonEmpty(req.SourceFormat, req.Format))
	body := req.OriginalRequest
	if len(body) == 0 {
		body = req.Payload
	}
	return hostModelExecutionRequest{
		HostModelExecutionRequest: pluginapi.HostModelExecutionRequest{
			EntryProtocol: protocol,
			ExitProtocol:  protocol,
			Model:         candidate.Model,
			Stream:        stream,
			Body:          bytes.Clone(body),
			Headers:       sanitizedHostHeaders(req.Headers),
			Query:         cloneValues(req.Query),
			Alt:           req.Alt,
		},
		HostCallbackID: strings.TrimSpace(hostCallbackID),
	}
}

func sanitizedHostHeaders(headers http.Header) http.Header {
	out := make(http.Header)
	for key, values := range headers {
		if isSensitiveHeader(key) {
			continue
		}
		out[key] = append([]string(nil), values...)
	}
	out.Set(bypassHeaderName, "1")
	return out
}

func isSensitiveHeader(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" {
		return true
	}
	switch key {
	case "authorization", "proxy-authorization", "cookie", "set-cookie", "x-api-key", "api-key":
		return true
	}
	return strings.Contains(key, "token") || strings.Contains(key, "api-key") || strings.Contains(key, "apikey") || strings.Contains(key, "authorization")
}

func readHostModelStream(streamID string) (pluginapi.HostModelStreamReadResponse, error) {
	raw, err := callHostCallback(pluginabi.MethodHostModelStreamRead, pluginapi.HostModelStreamReadRequest{StreamID: strings.TrimSpace(streamID)})
	if err != nil {
		return pluginapi.HostModelStreamReadResponse{}, err
	}
	var resp pluginapi.HostModelStreamReadResponse
	if errDecode := json.Unmarshal(raw, &resp); errDecode != nil {
		return pluginapi.HostModelStreamReadResponse{}, fmt.Errorf("decode host.model.stream_read result: %w", errDecode)
	}
	return resp, nil
}

func closeHostModelStream(streamID string) error {
	streamID = strings.TrimSpace(streamID)
	if streamID == "" {
		return nil
	}
	_, err := callHostCallback(pluginabi.MethodHostModelStreamClose, pluginapi.HostModelStreamCloseRequest{StreamID: streamID})
	return err
}

func emitPluginStreamChunk(streamID string, payload []byte) error {
	streamID = strings.TrimSpace(streamID)
	if streamID == "" {
		return fmt.Errorf("plugin stream id is required")
	}
	_, err := callHostCallback(pluginabi.MethodHostStreamEmit, rpcStreamEmitRequest{StreamID: streamID, Payload: bytes.Clone(payload)})
	return err
}

func closePluginStream(streamID, errMsg string) {
	streamID = strings.TrimSpace(streamID)
	if streamID == "" {
		return
	}
	_, _ = callHostCallback(pluginabi.MethodHostStreamClose, rpcStreamCloseRequest{StreamID: streamID, Error: strings.TrimSpace(errMsg)})
}
