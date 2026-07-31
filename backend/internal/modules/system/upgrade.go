package system

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

const (
	upgradeUnitName          = "transithub-upgrade.service"
	defaultUpgradeStatusPath = "/var/lib/transithub/upgrade-status.json"
	defaultUpgradeLogPath    = "/var/lib/transithub/upgrade.log"
	maximumUpgradeLogBytes   = 32 * 1024
)

type commandRunner func(ctx context.Context, name string, args ...string) ([]byte, error)

type systemdUpgradeExecutor struct {
	statusPath string
	logPath    string
	runCommand commandRunner
}

func newSystemdUpgradeExecutor() *systemdUpgradeExecutor {
	return &systemdUpgradeExecutor{
		statusPath: defaultUpgradeStatusPath,
		logPath:    defaultUpgradeLogPath,
		runCommand: runCombinedOutput,
	}
}

func runCombinedOutput(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

func (e *systemdUpgradeExecutor) Start(ctx context.Context) error {
	runner := e.runCommand
	if runner == nil {
		runner = runCombinedOutput
	}
	output, err := runner(ctx, "systemctl", "start", "--no-block", upgradeUnitName)
	if err == nil {
		return nil
	}
	detail := strings.TrimSpace(string(output))
	if detail == "" {
		return err
	}
	return fmt.Errorf("%w：%s", err, detail)
}

func (e *systemdUpgradeExecutor) Status(ctx context.Context) (UpgradeStatusResponse, error) {
	if err := ctx.Err(); err != nil {
		return UpgradeStatusResponse{}, err
	}
	statusPath := e.statusPath
	if statusPath == "" {
		statusPath = defaultUpgradeStatusPath
	}
	payload, err := os.ReadFile(statusPath)
	if errorsIsNotExist(err) {
		return UpgradeStatusResponse{State: UpgradeStateIdle}, nil
	}
	if err != nil {
		return UpgradeStatusResponse{}, err
	}

	var status UpgradeStatusResponse
	if err := json.Unmarshal(payload, &status); err != nil {
		return UpgradeStatusResponse{}, fmt.Errorf("解析升级状态失败：%w", err)
	}
	if !validUpgradeState(status.State) {
		return UpgradeStatusResponse{}, fmt.Errorf("未知的升级状态：%s", status.State)
	}
	if status.State == UpgradeStateFailed {
		logPath := e.logPath
		if logPath == "" {
			logPath = defaultUpgradeLogPath
		}
		output, logErr := readFileTail(logPath, maximumUpgradeLogBytes)
		if logErr != nil {
			status.Output = fmt.Sprintf("读取升级日志失败：%v", logErr)
		} else {
			status.Output = string(output)
		}
	}
	return status, nil
}

func errorsIsNotExist(err error) bool {
	return err != nil && os.IsNotExist(err)
}

func validUpgradeState(state UpgradeState) bool {
	switch state {
	case UpgradeStateStarting, UpgradeStateRunning, UpgradeStateSucceeded, UpgradeStateFailed:
		return true
	default:
		return false
	}
}

func readFileTail(path string, maximumBytes int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() > maximumBytes {
		if _, err := file.Seek(-maximumBytes, io.SeekEnd); err != nil {
			return nil, err
		}
	}
	return io.ReadAll(io.LimitReader(file, maximumBytes))
}
