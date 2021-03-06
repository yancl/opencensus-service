// Copyright 2018, OpenCensus Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package octrace

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"

	"google.golang.org/grpc"

	agenttracepb "github.com/census-instrumentation/opencensus-proto/gen-go/agent/trace/v1"
	"github.com/census-instrumentation/opencensus-service/internal"
	"github.com/census-instrumentation/opencensus-service/receiver"
	"github.com/census-instrumentation/opencensus-service/spansink"
)

// NewTraceReceiver will create a handle that runs an OCReceiver at the provided port.
func NewTraceReceiver(port int, opts ...Option) (receiver.TraceReceiver, error) {
	return &ocReceiverHandler{srvPort: port, receiverOptions: opts}, nil
}

// NewTraceReceiverOnDefaultPort will create a handle that runs an OCReceiver at the default port.
func NewTraceReceiverOnDefaultPort(opts ...Option) (receiver.TraceReceiver, error) {
	return &ocReceiverHandler{srvPort: defaultOCReceiverPort, receiverOptions: opts}, nil
}

type ocReceiverHandler struct {
	mu      sync.RWMutex
	srvPort int

	receiverOptions []Option

	ln         net.Listener
	grpcServer *grpc.Server

	stopOnce  sync.Once
	startOnce sync.Once
}

var _ receiver.TraceReceiver = (*ocReceiverHandler)(nil)

var errAlreadyStarted = errors.New("already started")

// StartTraceReception starts a gRPC server with an OpenCensus receiver running
func (ocih *ocReceiverHandler) StartTraceReception(ctx context.Context, sr spansink.Sink) error {
	var err = errAlreadyStarted
	ocih.startOnce.Do(func() {
		err = ocih.startInternal(ctx, sr)
	})

	return err
}

const defaultOCReceiverPort = 55678

func (ocih *ocReceiverHandler) startInternal(ctx context.Context, sr spansink.Sink) error {
	ocih.mu.Lock()
	defer ocih.mu.Unlock()

	oci, err := New(sr, ocih.receiverOptions...)
	if err != nil {
		return err
	}

	port := ocih.srvPort
	if port <= 0 {
		port = defaultOCReceiverPort
	}

	addr := fmt.Sprintf("localhost:%d", port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	srv := internal.GRPCServerWithObservabilityEnabled()

	agenttracepb.RegisterTraceServiceServer(srv, oci)
	go func() {
		_ = srv.Serve(ln)
	}()
	ocih.ln = ln
	ocih.grpcServer = srv

	return err
}

var errAlreadyStopped = errors.New("already stopped")

func (ocih *ocReceiverHandler) StopTraceReception(ctx context.Context) error {
	var err = errAlreadyStopped
	ocih.stopOnce.Do(func() {
		ocih.mu.Lock()
		defer ocih.mu.Unlock()

		// TODO: (@odeke-em) should we instead to a graceful stop instead of a sudden stop?
		// A graceful stop takes time to terminate.
		ocih.grpcServer.Stop()
		err = nil
	})
	return err
}
