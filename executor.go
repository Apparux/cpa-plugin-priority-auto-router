package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func (p *priorityAutoRouterPlugin) Identifier() string {
	return pluginProvider
}

func (p *priorityAutoRouterPlugin) Execute(ctx context.Context, req pluginapi.ExecutorRequest) (pluginapi.ExecutorResponse, error) {
	return p.execute(ctx, req, "")
}

func (p *priorityAutoRouterPlugin) ExecuteStream(_ context.Context, _ pluginapi.ExecutorRequest) (pluginapi.ExecutorStreamResponse, error) {
	return pluginapi.ExecutorStreamResponse{}, fmt.Errorf("%s streaming requires the dynamic plugin stream bridge", pluginName)
}

func (p *priorityAutoRouterPlugin) CountTokens(context.Context, pluginapi.ExecutorRequest) (pluginapi.ExecutorResponse, error) {
	return pluginapi.ExecutorResponse{Payload: []byte(`{"input_tokens":0}`)}, nil
}

func (p *priorityAutoRouterPlugin) HttpRequest(context.Context, pluginapi.ExecutorHTTPRequest) (pluginapi.ExecutorHTTPResponse, error) {
	return pluginapi.ExecutorHTTPResponse{}, fmt.Errorf("%s does not implement executor.http_request", pluginName)
}

func (p *priorityAutoRouterPlugin) execute(_ context.Context, req pluginapi.ExecutorRequest, hostCallbackID string) (pluginapi.ExecutorResponse, error) {
	plan, ok := p.routePlanForExecutor(req)
	if !ok || len(plan.Candidates) == 0 {
		return pluginapi.ExecutorResponse{}, statusError{status: http.StatusBadGateway, message: "priority auto router: no route candidates"}
	}

	var lastErr error
	lastStatus := 0
	for index, candidate := range plan.Candidates {
		resp, err := callHostModelExecute(req, candidate, hostCallbackID)
		status := extractExecutionStatus(&resp, err)
		if err == nil && successStatus(status) {
			logCandidateSuccess(hostCallbackID, plan, candidate, status)
			return pluginapi.ExecutorResponse{
				Payload:  append([]byte(nil), resp.Body...),
				Headers:  cloneHeader(resp.Headers),
				Metadata: map[string]any{"selected_candidate": candidate.Name},
			}, nil
		}
		lastErr = err
		lastStatus = status
		if !shouldFallback(status, err, p.cfg.Fallback) || index == len(plan.Candidates)-1 {
			logNoFallback(hostCallbackID, plan, candidate, status, err)
			return pluginapi.ExecutorResponse{}, statusError{status: statusOrDefault(status), message: safeErrorMessage(err, status)}
		}
		next := plan.Candidates[index+1]
		logFallback(hostCallbackID, plan, candidate, next, status, err)
	}
	return pluginapi.ExecutorResponse{}, statusError{status: statusOrDefault(lastStatus), message: safeErrorMessage(lastErr, lastStatus)}
}

func (p *priorityAutoRouterPlugin) startExecutorStream(req rpcExecutorRequest) ([]byte, error) {
	streamID := strings.TrimSpace(req.StreamID)
	if streamID == "" {
		return abiErrorEnvelope("executor_error", "stream_id is required for executor.execute_stream"), nil
	}
	priorityAutoRouterABIState.inFlight.Add(1)
	go func() {
		defer priorityAutoRouterABIState.inFlight.Done()
		defer func() {
			if recovered := recover(); recovered != nil {
				closePluginStream(streamID, fmt.Sprintf("stream orchestration panic: %v", recovered))
			}
		}()
		err := p.runStreamOrchestration(context.Background(), req.ExecutorRequest, req.HostCallbackID, streamID)
		if err != nil {
			closePluginStream(streamID, err.Error())
			return
		}
		closePluginStream(streamID, "")
	}()
	return abiOKEnvelope(abiExecutorStreamResponse{Headers: streamHeaders(req.ExecutorRequest)})
}

func (p *priorityAutoRouterPlugin) runStreamOrchestration(_ context.Context, req pluginapi.ExecutorRequest, hostCallbackID, pluginStreamID string) error {
	plan, ok := p.routePlanForExecutor(req)
	if !ok || len(plan.Candidates) == 0 {
		return statusError{status: http.StatusBadGateway, message: "priority auto router: no stream route candidates"}
	}

	var lastErr error
	lastStatus := 0
	for index, candidate := range plan.Candidates {
		status, err := p.forwardCandidateStream(req, hostCallbackID, pluginStreamID, candidate)
		if err == nil && successStatus(status) {
			logCandidateSuccess(hostCallbackID, plan, candidate, status)
			return nil
		}
		lastErr = err
		lastStatus = status
		var terminalStreamErr terminalStreamError
		if errors.As(err, &terminalStreamErr) || !shouldFallback(status, err, p.cfg.Fallback) || index == len(plan.Candidates)-1 {
			logNoFallback(hostCallbackID, plan, candidate, status, err)
			return statusError{status: statusOrDefault(status), message: safeErrorMessage(err, status)}
		}
		next := plan.Candidates[index+1]
		logFallback(hostCallbackID, plan, candidate, next, status, err)
	}
	return statusError{status: statusOrDefault(lastStatus), message: safeErrorMessage(lastErr, lastStatus)}
}

type terminalStreamError struct {
	status int
	err    error
}

func (e terminalStreamError) Error() string   { return e.err.Error() }
func (e terminalStreamError) Unwrap() error   { return e.err }
func (e terminalStreamError) StatusCode() int { return e.status }

func (p *priorityAutoRouterPlugin) forwardCandidateStream(req pluginapi.ExecutorRequest, hostCallbackID, pluginStreamID string, candidate Candidate) (int, error) {
	resp, err := callHostModelExecuteStream(req, candidate, hostCallbackID)
	status := extractStreamStatus(&resp, err)
	if err != nil {
		return status, err
	}
	if !successStatus(status) {
		_ = closeHostModelStream(resp.StreamID)
		return status, fmt.Errorf("host model status %d", statusOrDefault(status))
	}
	if strings.TrimSpace(resp.StreamID) == "" {
		return 0, fmt.Errorf("host model stream: empty stream_id")
	}
	defer func() { _ = closeHostModelStream(resp.StreamID) }()

	emitted := false
	for {
		chunk, errRead := readHostModelStream(resp.StreamID)
		if errRead != nil {
			status = statusFromError(errRead)
			if emitted {
				return status, terminalStreamError{status: status, err: errRead}
			}
			return status, errRead
		}
		if chunk.Error != "" {
			errChunk := fmt.Errorf("%s", chunk.Error)
			status = statusFromError(errChunk)
			if emitted {
				return status, terminalStreamError{status: status, err: errChunk}
			}
			return status, errChunk
		}
		if len(chunk.Payload) > 0 {
			if errEmit := emitPluginStreamChunk(pluginStreamID, chunk.Payload); errEmit != nil {
				return statusFromError(errEmit), errEmit
			}
			emitted = true
		}
		if chunk.Done {
			return http.StatusOK, nil
		}
	}
}

func streamHeaders(req pluginapi.ExecutorRequest) http.Header {
	contentType := "text/event-stream"
	if strings.EqualFold(normalizeProtocol(firstNonEmpty(req.SourceFormat, req.Format)), "openai") && !req.Stream {
		contentType = "application/json"
	}
	return http.Header{"Content-Type": []string{contentType}}
}

func statusOrDefault(status int) int {
	if status > 0 {
		return status
	}
	return http.StatusBadGateway
}

func safeErrorMessage(err error, status int) string {
	if status > 0 {
		return fmt.Sprintf("model execution failed with status %d", status)
	}
	if err != nil {
		return err.Error()
	}
	return "model execution failed"
}
