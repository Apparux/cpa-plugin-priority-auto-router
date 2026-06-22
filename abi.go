package main

/*
#include <stdint.h>
#include <stdlib.h>

typedef struct {
	void* ptr;
	size_t len;
} cliproxy_buffer;

typedef int (*cliproxy_host_call_fn)(void*, const char*, const uint8_t*, size_t, cliproxy_buffer*);
typedef void (*cliproxy_host_free_fn)(void*, size_t);

typedef struct {
	uint32_t abi_version;
	void* host_ctx;
	cliproxy_host_call_fn call;
	cliproxy_host_free_fn free_buffer;
} cliproxy_host_api;

typedef int (*cliproxy_plugin_call_fn)(char*, uint8_t*, size_t, cliproxy_buffer*);
typedef void (*cliproxy_plugin_free_fn)(void*, size_t);
typedef void (*cliproxy_plugin_shutdown_fn)(void);

typedef struct {
	uint32_t abi_version;
	cliproxy_plugin_call_fn call;
	cliproxy_plugin_free_fn free_buffer;
	cliproxy_plugin_shutdown_fn shutdown;
} cliproxy_plugin_api;

extern int PriorityAutoRouterPluginCall(char*, uint8_t*, size_t, cliproxy_buffer*);
extern void PriorityAutoRouterPluginFree(void*, size_t);
extern void PriorityAutoRouterPluginShutdown(void);

static int priority_auto_router_call_host(cliproxy_host_api* api, const char* method, const uint8_t* request, size_t request_len, cliproxy_buffer* response) {
	return api->call(api->host_ctx, method, request, request_len, response);
}

static void priority_auto_router_free_host_buffer(cliproxy_host_api* api, void* ptr, size_t len) {
	api->free_buffer(ptr, len);
}
*/
import "C"

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"unsafe"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

const maxCGoBytesLen = C.size_t(1<<31 - 1)

var priorityAutoRouterABIState = struct {
	sync.RWMutex
	host         *C.cliproxy_host_api
	plugin       *priorityAutoRouterPlugin
	shuttingDown bool
	inFlight     sync.WaitGroup
}{}

type abiEnvelope struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *abiError       `json:"error,omitempty"`
}

type abiError struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	Retryable  bool   `json:"retryable,omitempty"`
	HTTPStatus int    `json:"http_status,omitempty"`
}

type abiLifecycleRequest struct {
	ConfigYAML []byte `json:"config_yaml"`
	PluginDir  string `json:"plugin_dir,omitempty"`
}

type abiRegistration struct {
	SchemaVersion uint32             `json:"schema_version"`
	Metadata      pluginapi.Metadata `json:"metadata"`
	Capabilities  abiCapabilities    `json:"capabilities"`
}

type abiCapabilities struct {
	ModelRouter           bool                         `json:"model_router"`
	Executor              bool                         `json:"executor"`
	ExecutorModelScope    pluginapi.ExecutorModelScope `json:"executor_model_scope"`
	ExecutorInputFormats  []string                     `json:"executor_input_formats,omitempty"`
	ExecutorOutputFormats []string                     `json:"executor_output_formats,omitempty"`
}

type abiIdentifierResponse struct {
	Identifier string `json:"identifier"`
}

type abiExecutorStreamResponse struct {
	Headers http.Header                     `json:"headers,omitempty"`
	Chunks  []pluginapi.ExecutorStreamChunk `json:"chunks,omitempty"`
}

type rpcModelRouteRequest struct {
	pluginapi.ModelRouteRequest
	HostCallbackID string `json:"host_callback_id,omitempty"`
}

type rpcExecutorRequest struct {
	pluginapi.ExecutorRequest
	StreamID       string `json:"stream_id,omitempty"`
	HostCallbackID string `json:"host_callback_id,omitempty"`
}

type abiEmptyResponse struct{}

//export cliproxy_plugin_init
func cliproxy_plugin_init(host *C.cliproxy_host_api, plugin *C.cliproxy_plugin_api) C.int {
	if host == nil || plugin == nil {
		return 1
	}
	priorityAutoRouterABIState.Lock()
	priorityAutoRouterABIState.host = host
	priorityAutoRouterABIState.shuttingDown = false
	priorityAutoRouterABIState.Unlock()

	plugin.abi_version = C.uint32_t(pluginabi.ABIVersion)
	plugin.call = C.cliproxy_plugin_call_fn(C.PriorityAutoRouterPluginCall)
	plugin.free_buffer = C.cliproxy_plugin_free_fn(C.PriorityAutoRouterPluginFree)
	plugin.shutdown = C.cliproxy_plugin_shutdown_fn(C.PriorityAutoRouterPluginShutdown)
	return 0
}

//export PriorityAutoRouterPluginCall
func PriorityAutoRouterPluginCall(method *C.char, request *C.uint8_t, requestLen C.size_t, response *C.cliproxy_buffer) C.int {
	if response != nil {
		response.ptr = nil
		response.len = 0
	}
	if method == nil {
		writeABIResponse(response, abiErrorEnvelope("invalid_method", "method is required"))
		return 0
	}
	var requestBytes []byte
	if request != nil && requestLen > 0 {
		if requestLen > maxCGoBytesLen {
			writeABIResponse(response, abiErrorEnvelope("request_too_large", "request payload is too large"))
			return 0
		}
		requestBytes = C.GoBytes(unsafe.Pointer(request), C.int(requestLen))
	}
	raw, errHandle := handlePriorityAutoRouterABIMethod(context.Background(), C.GoString(method), requestBytes)
	if errHandle != nil {
		writeABIResponse(response, abiErrorEnvelopeFromError("plugin_error", errHandle))
		return 0
	}
	writeABIResponse(response, raw)
	return 0
}

//export PriorityAutoRouterPluginFree
func PriorityAutoRouterPluginFree(ptr unsafe.Pointer, _ C.size_t) {
	if ptr != nil {
		C.free(ptr)
	}
}

//export PriorityAutoRouterPluginShutdown
func PriorityAutoRouterPluginShutdown() {
	priorityAutoRouterABIState.Lock()
	priorityAutoRouterABIState.shuttingDown = true
	priorityAutoRouterABIState.Unlock()

	priorityAutoRouterABIState.inFlight.Wait()

	priorityAutoRouterABIState.Lock()
	if priorityAutoRouterABIState.plugin != nil && priorityAutoRouterABIState.plugin.plans != nil {
		priorityAutoRouterABIState.plugin.plans.clear()
	}
	priorityAutoRouterABIState.plugin = nil
	priorityAutoRouterABIState.host = nil
	priorityAutoRouterABIState.Unlock()
}

func handlePriorityAutoRouterABIMethod(ctx context.Context, method string, request []byte) (out []byte, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			out = nil
			err = fmt.Errorf("panic in %s: %v", method, recovered)
		}
	}()

	switch method {
	case pluginabi.MethodPluginRegister, pluginabi.MethodPluginReconfigure:
		return handlePriorityAutoRouterRegister(request)
	}

	p, done, errPlugin := beginPriorityAutoRouterPluginCall()
	if errPlugin != nil {
		return nil, errPlugin
	}
	defer done()

	switch method {
	case pluginabi.MethodModelRoute:
		var req rpcModelRouteRequest
		if errDecode := json.Unmarshal(request, &req); errDecode != nil {
			return nil, errDecode
		}
		resp, errRoute := p.RouteModel(ctx, req.ModelRouteRequest)
		return abiOKEnvelopeWithError(resp, errRoute)
	case pluginabi.MethodExecutorIdentifier:
		return abiOKEnvelope(abiIdentifierResponse{Identifier: p.Identifier()})
	case pluginabi.MethodExecutorExecute:
		var req rpcExecutorRequest
		if errDecode := json.Unmarshal(request, &req); errDecode != nil {
			return nil, errDecode
		}
		resp, errExecute := p.execute(ctx, req.ExecutorRequest, req.HostCallbackID)
		return abiOKEnvelopeWithError(resp, errExecute)
	case pluginabi.MethodExecutorExecuteStream:
		var req rpcExecutorRequest
		if errDecode := json.Unmarshal(request, &req); errDecode != nil {
			return nil, errDecode
		}
		return p.startExecutorStream(req)
	case pluginabi.MethodExecutorCountTokens:
		resp, errCount := p.CountTokens(ctx, pluginapi.ExecutorRequest{})
		return abiOKEnvelopeWithError(resp, errCount)
	case pluginabi.MethodExecutorHTTPRequest:
		return abiErrorEnvelope("unsupported_method", "executor.http_request is not supported by priority-auto-router"), nil
	default:
		return abiErrorEnvelope("unknown_method", "unknown method: "+method), nil
	}
}

func handlePriorityAutoRouterRegister(request []byte) ([]byte, error) {
	var req abiLifecycleRequest
	if len(request) > 0 {
		if errDecode := json.Unmarshal(request, &req); errDecode != nil {
			return nil, errDecode
		}
	}
	plugin, errBuild := buildPlugin(req.ConfigYAML, req.PluginDir)
	if errBuild != nil {
		return nil, errBuild
	}
	p, ok := plugin.Capabilities.ModelRouter.(*priorityAutoRouterPlugin)
	if !ok || p == nil {
		return nil, fmt.Errorf("%s registration returned invalid plugin instance", pluginName)
	}
	priorityAutoRouterABIState.Lock()
	priorityAutoRouterABIState.plugin = p
	priorityAutoRouterABIState.shuttingDown = false
	priorityAutoRouterABIState.Unlock()
	return abiOKEnvelope(abiRegistration{
		SchemaVersion: pluginabi.SchemaVersion,
		Metadata:      plugin.Metadata,
		Capabilities: abiCapabilities{
			ModelRouter:           plugin.Capabilities.ModelRouter != nil,
			Executor:              plugin.Capabilities.Executor != nil,
			ExecutorModelScope:    plugin.Capabilities.ExecutorModelScope,
			ExecutorInputFormats:  append([]string(nil), plugin.Capabilities.ExecutorInputFormats...),
			ExecutorOutputFormats: append([]string(nil), plugin.Capabilities.ExecutorOutputFormats...),
		},
	})
}

func beginPriorityAutoRouterPluginCall() (*priorityAutoRouterPlugin, func(), error) {
	priorityAutoRouterABIState.Lock()
	defer priorityAutoRouterABIState.Unlock()
	if priorityAutoRouterABIState.shuttingDown {
		return nil, nil, fmt.Errorf("%s plugin is shutting down", pluginName)
	}
	if priorityAutoRouterABIState.plugin == nil {
		return nil, nil, fmt.Errorf("%s plugin is not registered", pluginName)
	}
	priorityAutoRouterABIState.inFlight.Add(1)
	return priorityAutoRouterABIState.plugin, priorityAutoRouterABIState.inFlight.Done, nil
}

func abiOKEnvelopeWithError(v any, err error) ([]byte, error) {
	if err != nil {
		return nil, err
	}
	return abiOKEnvelope(v)
}

func abiOKEnvelope(v any) ([]byte, error) {
	raw, errMarshal := json.Marshal(v)
	if errMarshal != nil {
		return nil, errMarshal
	}
	return json.Marshal(abiEnvelope{OK: true, Result: raw})
}

func abiErrorEnvelope(code, message string) []byte {
	raw, _ := json.Marshal(abiEnvelope{OK: false, Error: &abiError{Code: code, Message: message}})
	return raw
}

func abiErrorEnvelopeFromError(code string, err error) []byte {
	apiErr := &abiError{Code: code, Message: err.Error()}
	if carrier, ok := err.(interface{ StatusCode() int }); ok && carrier.StatusCode() > 0 {
		apiErr.HTTPStatus = carrier.StatusCode()
	}
	raw, _ := json.Marshal(abiEnvelope{OK: false, Error: apiErr})
	return raw
}

func writeABIResponse(response *C.cliproxy_buffer, raw []byte) {
	if response == nil || len(raw) == 0 {
		return
	}
	ptr := C.CBytes(raw)
	if ptr == nil {
		return
	}
	response.ptr = ptr
	response.len = C.size_t(len(raw))
}

func callHost(method string, payload any) (json.RawMessage, error) {
	rawPayload, errMarshal := json.Marshal(payload)
	if errMarshal != nil {
		return nil, fmt.Errorf("marshal host callback %s: %w", method, errMarshal)
	}

	priorityAutoRouterABIState.RLock()
	defer priorityAutoRouterABIState.RUnlock()
	if priorityAutoRouterABIState.host == nil {
		return nil, fmt.Errorf("host callback is unavailable")
	}

	cMethod := C.CString(method)
	defer C.free(unsafe.Pointer(cMethod))

	var cPayload unsafe.Pointer
	if len(rawPayload) > 0 {
		cPayload = C.CBytes(rawPayload)
		if cPayload == nil {
			return nil, fmt.Errorf("allocate host callback payload")
		}
		defer C.free(cPayload)
	}

	var response C.cliproxy_buffer
	rc := C.priority_auto_router_call_host(
		priorityAutoRouterABIState.host,
		cMethod,
		(*C.uint8_t)(cPayload),
		C.size_t(len(rawPayload)),
		&response,
	)
	var rawResponse []byte
	if response.ptr != nil && response.len > 0 {
		rawResponse = C.GoBytes(response.ptr, C.int(response.len))
	}
	if response.ptr != nil {
		C.priority_auto_router_free_host_buffer(priorityAutoRouterABIState.host, response.ptr, response.len)
	}
	if len(rawResponse) == 0 {
		return nil, fmt.Errorf("host callback %s returned no response, code=%d", method, int(rc))
	}
	var env abiEnvelope
	if errDecode := json.Unmarshal(rawResponse, &env); errDecode != nil {
		return nil, fmt.Errorf("decode host envelope %s: %w", method, errDecode)
	}
	if !env.OK {
		if env.Error != nil {
			return nil, statusError{status: env.Error.HTTPStatus, message: fmt.Sprintf("%s: %s", env.Error.Code, env.Error.Message)}
		}
		return nil, fmt.Errorf("host callback %s failed", method)
	}
	if rc != 0 {
		return nil, fmt.Errorf("host callback %s returned code=%d", method, int(rc))
	}
	return append(json.RawMessage(nil), env.Result...), nil
}
