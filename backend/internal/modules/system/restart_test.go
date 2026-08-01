package system

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"transithub/backend/internal/config"
)

type fakeRestartExecutor struct {
	status     RestartStatusResponse
	statusErr  error
	startErr   error
	startCalls int
}

type blockingRestartExecutor struct {
	startEntered chan struct{}
	releaseStart chan struct{}
}

func (f *blockingRestartExecutor) Start(ctx context.Context) error {
	close(f.startEntered)
	select {
	case <-f.releaseStart:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (f *blockingRestartExecutor) Status(context.Context) (RestartStatusResponse, error) {
	return RestartStatusResponse{State: RestartStateIdle}, nil
}

func (f *fakeRestartExecutor) Start(context.Context) error {
	f.startCalls++
	return f.startErr
}

func (f *fakeRestartExecutor) Status(context.Context) (RestartStatusResponse, error) {
	return f.status, f.statusErr
}

func TestStartRestartStartsFixedExecutor(t *testing.T) {
	upgradeExecutor := &fakeUpgradeExecutor{status: UpgradeStatusResponse{State: UpgradeStateIdle}}
	restartExecutor := &fakeRestartExecutor{status: RestartStatusResponse{State: RestartStateIdle}}
	service := newServiceWithExecutors(config.Config{}, upgradeExecutor, restartExecutor)

	response, err := service.StartRestart(context.Background())
	if err != nil {
		t.Fatalf("start restart: %v", err)
	}
	if restartExecutor.startCalls != 1 {
		t.Fatalf("expected one start call, got %d", restartExecutor.startCalls)
	}
	if response.State != RestartStateStarting || response.RequestedAt == "" {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestRestartStatusWaitsUntilRestartRequestIsEnqueued(t *testing.T) {
	restartExecutor := &blockingRestartExecutor{
		startEntered: make(chan struct{}),
		releaseStart: make(chan struct{}),
	}
	service := newServiceWithExecutors(
		config.Config{},
		&fakeUpgradeExecutor{status: UpgradeStatusResponse{State: UpgradeStateIdle}},
		restartExecutor,
	)

	startDone := make(chan error, 1)
	go func() {
		_, err := service.StartRestart(context.Background())
		startDone <- err
	}()
	<-restartExecutor.startEntered

	statusDone := make(chan error, 1)
	go func() {
		_, err := service.RestartStatus(context.Background())
		statusDone <- err
	}()

	select {
	case err := <-statusDone:
		t.Fatalf("status returned before restart enqueue completed: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	close(restartExecutor.releaseStart)
	select {
	case err := <-startDone:
		if err != nil {
			t.Fatalf("start restart: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("start restart did not finish")
	}
	select {
	case err := <-statusDone:
		if err != nil {
			t.Fatalf("restart status: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("restart status did not finish")
	}
}

func TestStartRestartRejectsDuplicateRequestWhileStartIsPending(t *testing.T) {
	upgradeExecutor := &fakeUpgradeExecutor{status: UpgradeStatusResponse{State: UpgradeStateIdle}}
	restartExecutor := &fakeRestartExecutor{status: RestartStatusResponse{State: RestartStateIdle}}
	service := newServiceWithExecutors(config.Config{}, upgradeExecutor, restartExecutor)

	if _, err := service.StartRestart(context.Background()); err != nil {
		t.Fatalf("first restart: %v", err)
	}
	_, err := service.StartRestart(context.Background())
	if !errors.Is(err, ErrRestartInProgress) {
		t.Fatalf("expected ErrRestartInProgress, got %v", err)
	}
	if restartExecutor.startCalls != 1 {
		t.Fatalf("executor must start once, got %d calls", restartExecutor.startCalls)
	}
}

func TestStartRestartRejectsRunningRestart(t *testing.T) {
	upgradeExecutor := &fakeUpgradeExecutor{status: UpgradeStatusResponse{State: UpgradeStateIdle}}
	restartExecutor := &fakeRestartExecutor{status: RestartStatusResponse{State: RestartStateRunning}}
	service := newServiceWithExecutors(config.Config{}, upgradeExecutor, restartExecutor)

	_, err := service.StartRestart(context.Background())
	if !errors.Is(err, ErrRestartInProgress) {
		t.Fatalf("expected ErrRestartInProgress, got %v", err)
	}
	if restartExecutor.startCalls != 0 {
		t.Fatalf("executor must not start while running, got %d calls", restartExecutor.startCalls)
	}
}

func TestStartRestartRejectsRunningUpgrade(t *testing.T) {
	upgradeExecutor := &fakeUpgradeExecutor{status: UpgradeStatusResponse{State: UpgradeStateRunning}}
	restartExecutor := &fakeRestartExecutor{status: RestartStatusResponse{State: RestartStateIdle}}
	service := newServiceWithExecutors(config.Config{}, upgradeExecutor, restartExecutor)

	_, err := service.StartRestart(context.Background())
	if !errors.Is(err, ErrUpgradeInProgress) {
		t.Fatalf("expected ErrUpgradeInProgress, got %v", err)
	}
	if restartExecutor.startCalls != 0 {
		t.Fatalf("restart must not start during upgrade, got %d calls", restartExecutor.startCalls)
	}
}

func TestSystemdRestartExecutorStartsOnlyFixedUnit(t *testing.T) {
	tempDir := t.TempDir()
	var gotName string
	var gotArgs []string
	executor := &systemdRestartExecutor{
		statusPath: filepath.Join(tempDir, "restart-status.json"),
		logPath:    filepath.Join(tempDir, "restart.log"),
		runCommand: func(_ context.Context, name string, args ...string) ([]byte, error) {
			status, err := os.ReadFile(filepath.Join(tempDir, "restart-status.json"))
			if err != nil {
				t.Fatalf("read starting status before systemctl: %v", err)
			}
			if !strings.Contains(string(status), `"state":"starting"`) {
				t.Fatalf("expected starting status before systemctl, got %s", status)
			}
			gotName = name
			gotArgs = append([]string(nil), args...)
			return nil, nil
		},
	}

	if err := executor.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	if gotName != "systemctl" {
		t.Fatalf("expected systemctl, got %q", gotName)
	}
	wantArgs := []string{"start", "--no-block", restartUnitName}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("expected args %v, got %v", wantArgs, gotArgs)
	}
}

func TestSystemdRestartExecutorRecordsStartFailure(t *testing.T) {
	tempDir := t.TempDir()
	executor := &systemdRestartExecutor{
		statusPath: filepath.Join(tempDir, "restart-status.json"),
		logPath:    filepath.Join(tempDir, "restart.log"),
		runCommand: func(context.Context, string, ...string) ([]byte, error) {
			return []byte("unit could not be started"), errors.New("exit status 1")
		},
	}

	if err := executor.Start(context.Background()); err == nil {
		t.Fatal("expected start failure")
	}
	status, err := executor.Status(context.Background())
	if err != nil {
		t.Fatalf("read failed status: %v", err)
	}
	if status.State != RestartStateFailed || !strings.Contains(status.Output, "unit could not be started") {
		t.Fatalf("expected persisted start failure, got %+v", status)
	}
}

func TestSystemdRestartExecutorReconcilesAsynchronousUnitFailure(t *testing.T) {
	tempDir := t.TempDir()
	statusPath := filepath.Join(tempDir, "restart-status.json")
	logPath := filepath.Join(tempDir, "restart.log")
	if err := os.WriteFile(statusPath, []byte(`{"state":"starting","startedAt":"2026-08-01T08:00:00Z"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	executor := &systemdRestartExecutor{
		statusPath: statusPath,
		logPath:    logPath,
		runCommand: func(_ context.Context, name string, args ...string) ([]byte, error) {
			if name != "systemctl" || !reflect.DeepEqual(args, []string{"show", restartUnitName, "--property=ActiveState", "--property=Result", "--property=ExecMainStatus", "--property=Job", "--no-pager"}) {
				t.Fatalf("unexpected unit status command: %s %v", name, args)
			}
			return []byte("ActiveState=failed\nResult=exit-code\nExecMainStatus=203\n"), nil
		},
	}

	status, err := executor.Status(context.Background())
	if err != nil {
		t.Fatalf("reconcile asynchronous failure: %v", err)
	}
	if status.State != RestartStateFailed || status.ExitCode == nil || *status.ExitCode != 203 {
		t.Fatalf("expected asynchronous unit failure, got %+v", status)
	}
	if !strings.Contains(status.Output, "Result=exit-code") {
		t.Fatalf("expected unit result in failure log, got %q", status.Output)
	}
}

func TestSystemdRestartExecutorKeepsStartingWhileUnitJobIsQueued(t *testing.T) {
	tempDir := t.TempDir()
	statusPath := filepath.Join(tempDir, "restart-status.json")
	if err := os.WriteFile(statusPath, []byte(`{"state":"starting","startedAt":"2026-08-01T08:00:00Z"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	executor := &systemdRestartExecutor{
		statusPath: statusPath,
		logPath:    filepath.Join(tempDir, "restart.log"),
		runCommand: func(context.Context, string, ...string) ([]byte, error) {
			return []byte("ActiveState=failed\nResult=exit-code\nExecMainStatus=203\nJob=/org/freedesktop/systemd1/job/42\n"), nil
		},
	}

	status, err := executor.Status(context.Background())
	if err != nil {
		t.Fatalf("read queued status: %v", err)
	}
	if status.State != RestartStateStarting {
		t.Fatalf("expected queued job to remain starting, got %+v", status)
	}
}

func TestSystemdRestartExecutorRejectsInactiveUnitWithoutFinalStatus(t *testing.T) {
	tempDir := t.TempDir()
	statusPath := filepath.Join(tempDir, "restart-status.json")
	logPath := filepath.Join(tempDir, "restart.log")
	if err := os.WriteFile(statusPath, []byte(`{"state":"starting","startedAt":"2026-08-01T08:00:00Z"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	executor := &systemdRestartExecutor{
		statusPath: statusPath,
		logPath:    logPath,
		runCommand: func(context.Context, string, ...string) ([]byte, error) {
			return []byte("ActiveState=inactive\nResult=success\nExecMainStatus=0\nJob=\n"), nil
		},
	}

	status, err := executor.Status(context.Background())
	if err != nil {
		t.Fatalf("reconcile inactive unit: %v", err)
	}
	if status.State != RestartStateFailed {
		t.Fatalf("expected missing final status to fail, got %+v", status)
	}
}

func TestSystemdRestartExecutorPreservesFinalStatusWrittenDuringReconcile(t *testing.T) {
	tempDir := t.TempDir()
	statusPath := filepath.Join(tempDir, "restart-status.json")
	if err := os.WriteFile(statusPath, []byte(`{"state":"starting","startedAt":"2026-08-01T08:00:00Z"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	executor := &systemdRestartExecutor{
		statusPath: statusPath,
		logPath:    filepath.Join(tempDir, "restart.log"),
		runCommand: func(context.Context, string, ...string) ([]byte, error) {
			payload := []byte(`{"state":"succeeded","startedAt":"2026-08-01T08:00:01Z","finishedAt":"2026-08-01T08:00:02Z","exitCode":0}`)
			if err := os.WriteFile(statusPath, payload, 0o644); err != nil {
				t.Fatal(err)
			}
			return []byte("ActiveState=inactive\nResult=success\nExecMainStatus=0\nJob=\n"), nil
		},
	}

	status, err := executor.Status(context.Background())
	if err != nil {
		t.Fatalf("reconcile completed status: %v", err)
	}
	if status.State != RestartStateSucceeded {
		t.Fatalf("expected completed status to win reconcile race, got %+v", status)
	}
}

func TestSystemdRestartExecutorPreservesNewRestartWrittenDuringReconcile(t *testing.T) {
	tempDir := t.TempDir()
	statusPath := filepath.Join(tempDir, "restart-status.json")
	if err := os.WriteFile(statusPath, []byte(`{"state":"starting","startedAt":"2026-08-01T08:00:00Z"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	executor := &systemdRestartExecutor{
		statusPath: statusPath,
		logPath:    filepath.Join(tempDir, "restart.log"),
		runCommand: func(context.Context, string, ...string) ([]byte, error) {
			payload := []byte(`{"state":"starting","startedAt":"2026-08-01T08:01:00Z"}`)
			if err := os.WriteFile(statusPath, payload, 0o644); err != nil {
				t.Fatal(err)
			}
			return []byte("ActiveState=inactive\nResult=success\nExecMainStatus=0\nJob=\n"), nil
		},
	}

	status, err := executor.Status(context.Background())
	if err != nil {
		t.Fatalf("reconcile newer restart: %v", err)
	}
	if status.State != RestartStateStarting || status.StartedAt != "2026-08-01T08:01:00Z" {
		t.Fatalf("expected newer restart status to win reconcile race, got %+v", status)
	}
}

func TestSystemdRestartExecutorReadsFailureLogTail(t *testing.T) {
	tempDir := t.TempDir()
	statusPath := filepath.Join(tempDir, "restart-status.json")
	logPath := filepath.Join(tempDir, "restart.log")
	statusJSON := `{"state":"failed","startedAt":"2026-08-01T08:00:00Z","finishedAt":"2026-08-01T08:01:00Z","exitCode":17}`
	if err := os.WriteFile(statusPath, []byte(statusJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	prefix := make([]byte, maximumRestartLogBytes)
	for index := range prefix {
		prefix[index] = 'x'
	}
	wantOutput := "\nrestart health check failed\n"
	if err := os.WriteFile(logPath, append(prefix, []byte(wantOutput)...), 0o600); err != nil {
		t.Fatal(err)
	}
	executor := &systemdRestartExecutor{statusPath: statusPath, logPath: logPath}

	status, err := executor.Status(context.Background())
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.State != RestartStateFailed || status.ExitCode == nil || *status.ExitCode != 17 {
		t.Fatalf("unexpected status: %+v", status)
	}
	if status.Output != string(append(prefix[len(wantOutput):], []byte(wantOutput)...)) {
		t.Fatalf("unexpected output length=%d suffix=%q", len(status.Output), status.Output[len(status.Output)-len(wantOutput):])
	}
}

func TestSystemdRestartExecutorReturnsIdleWhenStatusDoesNotExist(t *testing.T) {
	executor := &systemdRestartExecutor{
		statusPath: filepath.Join(t.TempDir(), "missing.json"),
		logPath:    filepath.Join(t.TempDir(), "missing.log"),
	}

	status, err := executor.Status(context.Background())
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.State != RestartStateIdle {
		t.Fatalf("expected idle, got %+v", status)
	}
}

func TestSystemdRestartExecutorRejectsCorruptStatus(t *testing.T) {
	tempDir := t.TempDir()
	statusPath := filepath.Join(tempDir, "restart-status.json")
	if err := os.WriteFile(statusPath, []byte(`{"state":`), 0o600); err != nil {
		t.Fatal(err)
	}
	executor := &systemdRestartExecutor{statusPath: statusPath}

	_, err := executor.Status(context.Background())
	if err == nil || !strings.Contains(err.Error(), "解析重启状态失败") {
		t.Fatalf("expected corrupt status error, got %v", err)
	}
}

func TestRestartHandlerStartsAndReturnsStatus(t *testing.T) {
	upgradeExecutor := &fakeUpgradeExecutor{status: UpgradeStatusResponse{State: UpgradeStateIdle}}
	restartExecutor := &fakeRestartExecutor{status: RestartStatusResponse{State: RestartStateIdle}}
	service := newServiceWithExecutors(config.Config{}, upgradeExecutor, restartExecutor)
	mux := http.NewServeMux()
	RegisterRoutes(mux, service)

	startRecorder := httptest.NewRecorder()
	mux.ServeHTTP(startRecorder, httptest.NewRequest(http.MethodPost, "/api/system/restart", nil))
	if startRecorder.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d body=%s", startRecorder.Code, startRecorder.Body.String())
	}
	var startResponse RestartStartResponse
	if err := json.Unmarshal(startRecorder.Body.Bytes(), &startResponse); err != nil {
		t.Fatalf("decode start response: %v", err)
	}
	if startResponse.State != RestartStateStarting || startResponse.RequestedAt == "" {
		t.Fatalf("unexpected start response: %+v", startResponse)
	}

	restartExecutor.status = RestartStatusResponse{State: RestartStateFailed, Output: "health check failed"}
	statusRecorder := httptest.NewRecorder()
	mux.ServeHTTP(statusRecorder, httptest.NewRequest(http.MethodGet, "/api/system/restart", nil))
	if statusRecorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", statusRecorder.Code, statusRecorder.Body.String())
	}
	var statusResponse RestartStatusResponse
	if err := json.Unmarshal(statusRecorder.Body.Bytes(), &statusResponse); err != nil {
		t.Fatalf("decode status response: %v", err)
	}
	if statusResponse.State != RestartStateFailed || statusResponse.Output != "health check failed" {
		t.Fatalf("unexpected status response: %+v", statusResponse)
	}
}

func TestRestartHandlerReturnsConflictWhileRunning(t *testing.T) {
	upgradeExecutor := &fakeUpgradeExecutor{status: UpgradeStatusResponse{State: UpgradeStateIdle}}
	restartExecutor := &fakeRestartExecutor{status: RestartStatusResponse{State: RestartStateRunning}}
	service := newServiceWithExecutors(config.Config{}, upgradeExecutor, restartExecutor)
	mux := http.NewServeMux()
	RegisterRoutes(mux, service)

	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/system/restart", nil))
	if recorder.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestRestartHandlerReturnsExecutorFailureReason(t *testing.T) {
	upgradeExecutor := &fakeUpgradeExecutor{status: UpgradeStatusResponse{State: UpgradeStateIdle}}
	restartExecutor := &fakeRestartExecutor{
		status:   RestartStatusResponse{State: RestartStateIdle},
		startErr: errors.New("access denied by systemd"),
	}
	service := newServiceWithExecutors(config.Config{}, upgradeExecutor, restartExecutor)
	mux := http.NewServeMux()
	RegisterRoutes(mux, service)

	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/system/restart", nil))
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if response.Message != "启动后台服务重启失败：access denied by systemd" {
		t.Fatalf("unexpected failure reason: %q", response.Message)
	}
}
