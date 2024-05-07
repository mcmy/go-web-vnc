package main

import (
	"embed"
	"fmt"
	"golang.org/x/sys/windows/registry"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
)

func getVncPort() int {
	key, exists, _ := registry.CreateKey(registry.LOCAL_MACHINE, `SOFTWARE\TightVNC\Server`, registry.ALL_ACCESS)
	defer key.Close()

	if exists {
		s, _, err := key.GetIntegerValue(`RfbPort`)
		if err == nil {
			return int(s)
		}
	}
	return 5900
}

func getLocalIPv4Address() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, addr := range addrs {
		ipNet, isIpNet := addr.(*net.IPNet)
		if isIpNet && !ipNet.IP.IsLoopback() {
			ipv4 := ipNet.IP.To4()
			if ipv4 != nil && !strings.HasPrefix(ipv4.String(), "169.254") {
				return ipv4.String()
			}
		}
	}

	return ""
}

func ExtraEmbedFs(EmbedFs embed.FS, fsDir, targetDir string) error {
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return err
	}

	err := fs.WalkDir(EmbedFs, fsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			data, err := fs.ReadFile(EmbedFs, path)
			if err != nil {
				return err
			}

			targetPath := filepath.Join(targetDir, path)
			if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
				return err
			}
			if err := os.WriteFile(targetPath, data, 0644); err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		return err
	}
	return nil
}

func initVncReg() error {
	key, _, err := registry.CreateKey(registry.LOCAL_MACHINE, `SOFTWARE\TightVNC\Server`, registry.ALL_ACCESS)
	if err != nil {
		return fmt.Errorf("Error creating/opening key: %v\n", err)
	}
	defer func(key registry.Key) {
		_ = key.Close()
	}(key)

	if err := setRegistryValues(key, settings); err != nil {
		return fmt.Errorf("Error setting registry values: %v\n", err)
	}

	return nil
}

func setRegistryValues(key registry.Key, settings interface{}) error {
	val := reflect.ValueOf(settings)
	typ := val.Type()
	var errs []string

	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		name := typ.Field(i).Name

		switch field.Kind() {
		case reflect.Uint32:
			if err := key.SetDWordValue(name, uint32(field.Uint())); err != nil {
				return err
			}
		case reflect.String:
			if err := key.SetStringValue(name, field.String()); err != nil {
				return err
			}
		default:
			errs = append(errs, fmt.Sprintf("unsupported field type %v", field.Kind()))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("unsupported field type %s", strings.Join(errs, ",unsupported field type "))
	}
	return nil
}

func findWindowsPortProcessPID(port string) string {
	cmd := exec.Command("cmd", "/C", "netstat -aon | findstr", fmt.Sprintf(":%s", port))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	lines := strings.Split(string(output), "\n")
	var pid string
	for _, line := range lines {
		if strings.Contains(line, fmt.Sprintf(":%s", port)) {
			parts := strings.Fields(line)
			pid = parts[len(parts)-1]
			break
		}
	}
	return pid
}

func killProcessUsingPort(port string, force bool) error {
	pid := findWindowsPortProcessPID(port)
	if len(pid) < 1 {
		return fmt.Errorf("not find port exe,:%s", port)
	}
	arg := []string{"/PID", pid}
	if force {
		arg = []string{"/F", "/PID", pid}
	}
	killCmd := exec.Command("taskkill", arg...)
	killCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := killCmd.Run(); err != nil {
		return err
	}
	return nil
}

func killProcess(processName string, force bool) error {
	arg := []string{"/IM", processName}
	if force {
		arg = []string{"/F", "/IM", processName}
	}
	cmd := exec.Command("taskkill", arg...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}

func checkProcessRunning(processName string) (bool, error) {
	cmd := exec.Command("tasklist", "/FI", "IMAGENAME eq "+processName, "/FO", "CSV")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	output, err := cmd.Output()
	if err != nil {
		return false, err
	}

	return strings.Contains(string(output), processName), nil
}
