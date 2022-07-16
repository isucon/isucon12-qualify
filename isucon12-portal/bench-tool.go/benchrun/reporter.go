package benchrun

import (
	"bufio"
	"encoding/binary"
	"errors"
	"os"
	"strconv"
	"syscall"

	"github.com/golang/protobuf/proto"
	isuxportalResources "github.com/isucon/isucon12-portal/proto.go/isuxportal/resources"
	timestamppb "google.golang.org/protobuf/types/known/timestamppb"
)

// Reporter is a client for isuxportal_supervisor. Send a BenchmarkResult message to the isuxportal via the supervisor.
type Reporter interface {
	Report(result *isuxportalResources.BenchmarkResult) error
}

// NullReporter is a Reporter not backed with anything.
type NullReporter struct{}

// Report sends a BenchmarkResult message, but this does nothing because NullReporter is not backed with anything.
func (rep *NullReporter) Report(result *isuxportalResources.BenchmarkResult) error {
	return nil
}

// BoundReporter is a Reporter backed with a fd to isuxportal_supervisor.
type BoundReporter struct {
	io *bufio.Writer
}

// NewReporter returns a Reporter with a given ISUXBENCH_REPORT_FD.
// When mustPresent is true, it will returns an error when no prerequisite environment variable and file descriptor given. Otherwise, it will return NullReporter.
func NewReporter(mustPresent bool) (Reporter, error) {
	defer os.Unsetenv("ISUXBENCH_REPORT_FD")

	fdEnv := os.Getenv("ISUXBENCH_REPORT_FD")
	if len(fdEnv) == 0 {
		if mustPresent {
			return nil, errors.New("$ISUXBENCH_REPORT_FD isn't given while it is required")
		}
		return &NullReporter{}, nil
	}

	fd, err := strconv.Atoi(fdEnv)
	if err != nil {
		return nil, err
	}

	syscall.CloseOnExec(fd)
	io := os.NewFile(uintptr(fd), "ISUXBENCH_REPORT_FD")

	bufWriter := bufio.NewWriter(io)

	rep := &BoundReporter{
		io: bufWriter,
	}

	return rep, nil
}

// Report sends a BenchmarkResult message to a client. Returns an error when invalid message is found at the client side, or failed to send.
func (rep *BoundReporter) Report(result *isuxportalResources.BenchmarkResult) error {
	if result.SurveyResponse != nil && len(result.SurveyResponse.Language) > 140 {
		return errors.New("language in a given survey response is too long (max: 140)")
	}

	if result.MarkedAt == nil {
		result.MarkedAt = timestamppb.Now()
	}

	wire, err := proto.Marshal(result)
	if err != nil {
		return err
	}
	if len(wire) > 65536 {
		return errors.New("Marshalled BenchmarkResult is too long (max: 65536)")
	}

	lenBuf := make([]byte, 2)
	binary.BigEndian.PutUint16(lenBuf, uint16(len(wire)))

	if _, err := rep.io.Write(lenBuf); err != nil {
		return err
	}
	if _, err := rep.io.Write(wire); err != nil {
		return err
	}
	if err := rep.io.Flush(); err != nil {
		return err
	}

	return nil
}
