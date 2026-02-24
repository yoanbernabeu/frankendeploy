package deploy

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/yoanbernabeu/frankendeploy/internal/constants"
	"github.com/yoanbernabeu/frankendeploy/internal/ssh"
)

// HealthChecker performs health checks on deployed applications
type HealthChecker struct {
	client      ssh.Executor
	containerID string
	path        string
	port        string
	timeout     time.Duration
	retries     int
	interval    time.Duration
}

// NewHealthChecker creates a new health checker with sensible defaults from constants.
func NewHealthChecker(client ssh.Executor, containerID, path, port string) *HealthChecker {
	return &HealthChecker{
		client:      client,
		containerID: containerID,
		path:        path,
		port:        port,
		timeout:     constants.HealthCheckTimeout,
		retries:     constants.HealthCheckRetries,
		interval:    constants.HealthCheckInterval,
	}
}

// SetTimeout sets the overall timeout
func (h *HealthChecker) SetTimeout(timeout time.Duration) {
	h.timeout = timeout
}

// SetRetries sets the number of retries
func (h *HealthChecker) SetRetries(retries int) {
	h.retries = retries
}

// SetInterval sets the interval between retries
func (h *HealthChecker) SetInterval(interval time.Duration) {
	h.interval = interval
}

// HealthResult contains the result of a health check
type HealthResult struct {
	Healthy      bool
	StatusCode   int
	Message      string
	ResponseTime time.Duration
	Attempts     int
}

// Check performs the health check with retries
func (h *HealthChecker) Check(ctx context.Context) (*HealthResult, error) {
	result := &HealthResult{
		Healthy: false,
	}

	deadline := time.Now().Add(h.timeout)

	for attempt := 1; attempt <= h.retries; attempt++ {
		result.Attempts = attempt

		if time.Now().After(deadline) {
			result.Message = "health check timeout"
			return result, nil
		}

		// Check if container is running
		containerCheck, _ := h.client.Exec(ctx, fmt.Sprintf(
			"docker inspect %s --format '{{.State.Status}}'", h.containerID))

		containerStatus := strings.TrimSpace(containerCheck.Stdout)
		if containerStatus != "running" {
			result.Message = fmt.Sprintf("container not running (status: %s)", containerStatus)
			time.Sleep(h.interval)
			continue
		}

		// Check HTTP endpoint
		start := time.Now()
		healthCmd := fmt.Sprintf(
			"docker exec %s curl -sf -w '%%{http_code}' -o /dev/null http://localhost:%s%s",
			h.containerID, h.port, h.path)

		httpCheck, err := h.client.Exec(ctx, healthCmd)
		result.ResponseTime = time.Since(start)

		if err == nil && httpCheck.ExitCode == 0 {
			result.Healthy = true
			result.StatusCode = 200 // default if parsing fails
			if httpCheck.Stdout != "" {
				var code int
				if n, scanErr := fmt.Sscanf(strings.TrimSpace(httpCheck.Stdout), "%d", &code); n == 1 && scanErr == nil {
					result.StatusCode = code
				}
			}
			result.Message = "healthy"
			return result, nil
		}

		// Parse status code from output
		if httpCheck != nil && httpCheck.Stdout != "" {
			if n, scanErr := fmt.Sscanf(httpCheck.Stdout, "%d", &result.StatusCode); n == 0 || scanErr != nil {
				result.StatusCode = 0
			}
		}

		result.Message = fmt.Sprintf("HTTP check failed (status: %d)", result.StatusCode)
		time.Sleep(h.interval)
	}

	return result, nil
}

// CheckContainer verifies if a container is running
func (h *HealthChecker) CheckContainer(ctx context.Context) (bool, error) {
	result, err := h.client.Exec(ctx, fmt.Sprintf(
		"docker ps --filter name=%s --format '{{.Status}}'", h.containerID))

	if err != nil {
		return false, err
	}

	return result.Stdout != "", nil
}

// GetContainerLogs retrieves recent container logs
func (h *HealthChecker) GetContainerLogs(ctx context.Context, lines int) (string, error) {
	result, err := h.client.Exec(ctx, fmt.Sprintf(
		"docker logs %s --tail %d 2>&1", h.containerID, lines))

	if err != nil {
		return "", err
	}

	return result.Stdout, nil
}

// WaitForContainer waits for a container to be in running state
func (h *HealthChecker) WaitForContainer(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for container to start")
		}

		running, err := h.CheckContainer(ctx)
		if err != nil {
			return err
		}

		if running {
			return nil
		}

		time.Sleep(500 * time.Millisecond)
	}
}
