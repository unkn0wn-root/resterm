package grpcx

import (
	"context"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/restfile"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestExecuteCapturesStatusDetails(t *testing.T) {
	gr, opt := descriptorRequest(t)

	st, err := status.New(codes.InvalidArgument, "bad request").WithDetails(
		&errdetails.BadRequest{
			FieldViolations: []*errdetails.BadRequest_FieldViolation{
				{Field: "id", Description: "must not be empty"},
			},
		},
	)
	if err != nil {
		t.Fatalf("build status: %v", err)
	}

	conn := &stubConn{
		invoke: func(context.Context, string, any, any) error { return st.Err() },
	}
	resp, execErr := stubClient(conn).
		Execute(context.Background(), &restfile.Request{GRPC: gr}, opt, nil)
	if execErr == nil {
		t.Fatal("expected the rpc error to surface")
	}
	if resp == nil {
		t.Fatal("expected a response alongside the error")
	}

	if len(resp.StatusDetails) != 1 {
		t.Fatalf("StatusDetails = %v, want one entry", resp.StatusDetails)
	}
	detail := resp.StatusDetails[0]
	if !strings.Contains(detail, "must not be empty") {
		t.Fatalf("detail = %q, want the field violation", detail)
	}
	if !strings.Contains(detail, "BadRequest") {
		t.Fatalf("detail = %q, want the detail type named", detail)
	}
}

func TestStatusDetailsEmptyWithoutDetails(t *testing.T) {
	st := status.New(codes.NotFound, "missing")
	if got := statusDetails(st, codec{}); got != nil {
		t.Fatalf("statusDetails() = %v, want nil", got)
	}
	if got := statusDetails(nil, codec{}); got != nil {
		t.Fatalf("statusDetails(nil) = %v, want nil", got)
	}
}
