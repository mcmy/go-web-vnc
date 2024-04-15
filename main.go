package main

import (
	"embed"
	"flag"
	"fmt"
	"github.com/gin-gonic/gin"
	"go-vnc-proxy/conf"
	"go-vnc-proxy/public_fs"
	"go-vnc-proxy/vnc_proxy"
	"golang.org/x/net/context"
	"golang.org/x/net/websocket"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"strings"
	"syscall"
	"time"
)

func init() {
	//filename, _ := filepath.Abs("app.yml")
	//yamlFile, err := os.ReadFile(filename)
	//if err != nil {
	//	log.Fatalln(err)
	//}
	//var c conf.AppConf
	//err = yaml.Unmarshal(yamlFile, &c)
	c := conf.AppConf{}
	c.AppInfo = conf.AppInfo{
		Port:       9091,
		TLSKey:     "",
		TLSCert:    "",
		TLSCaCerts: "",
	}
	conf.SetAppConf(c)
	//if err != nil {
	//	log.Fatalln(err)
	//}
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
	if err := killCmd.Run(); err != nil {
		return err
	}
	if err := killCmd.Wait(); err != nil {
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
	if err := cmd.Run(); err != nil {
		return err
	}
	if err := cmd.Wait(); err != nil {
		return err
	}
	return nil
}

func checkProcessRunning(processName string) (bool, error) {
	cmd := exec.Command("tasklist", "/FI", "IMAGENAME eq "+processName, "/FO", "CSV")
	output, err := cmd.Output()
	if err != nil {
		return false, err
	}

	return strings.Contains(string(output), processName), nil
}

func killVnc(force bool) {
	_ = killProcess("TightVNCServerPortable.exe", force)
	_ = killProcess("tvnserver.exe", force)
	_ = killProcessUsingPort("5900", force)
}

func exit(srv *http.Server) {
	go func() {
		time.Sleep(time.Second * 5)
		os.Exit(1)
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var errs []error
	if err := srv.Shutdown(ctx); err != nil {
		errs = append(errs, err)
	}
	killVnc(false)
	os.Exit(0)
}

func main() {
	shutdown := "false"
	flag.StringVar(&shutdown, "close", "false", "")
	flag.Parse()
	killVnc(true)
	if shutdown == "true" {
		os.Exit(0)
		return
	}
	if err := os.RemoveAll(".tight_vnc"); err != nil {
		log.Fatalln(err)
	}
	if err := ExtraEmbedFs(public_fs.EmbedVNC, "tight_vnc", ".cache"); err != nil {
		log.Fatalln(err)
	}
	go func() {
		_ = exec.Command(".cache/tight_vnc/TightVNCServerPortable.exe").Run()
	}()
	gin.SetMode(gin.ReleaseMode)
	engine := gin.Default()
	noVncHandler := http.FileServer(http.FS(public_fs.EmbedFiles))
	engine.GET("/vnc/*filepath", func(c *gin.Context) {
		c.Request.URL.Path = "novnc" + c.Param("filepath")
		noVncHandler.ServeHTTP(c.Writer, c.Request)
	})
	vncProxy := NewVNCProxy()
	engine.GET("/websockify", func(ctx *gin.Context) {
		h := websocket.Handler(vncProxy.ServeWS)
		h.ServeHTTP(ctx.Writer, ctx.Request)
	})
	log.Println("start vnc server on :5900")
	log.Println("start vnc http server on :9091")
	log.Println("vnc page :9091/vnc")

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", conf.Conf.AppInfo.Port),
		Handler: engine,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil {
			log.Fatalf("listen: %s\n", err)
		}
	}()

	go func() {
		for {
			time.Sleep(time.Second * 3)
			if len(findWindowsPortProcessPID("5900")) == 0 {
				running, _ := checkProcessRunning("TightVNCServerPortable.exe")
				if running {
					killVnc(false)
					time.Sleep(time.Second * 3)
					go func() {
						_ = exec.Command(".cache/tight_vnc/TightVNCServerPortable.exe").Run()
					}()
					continue
				}
				go exit(srv)
			}
		}
	}()

	quit := make(chan os.Signal)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	exit(srv)
}

func NewVNCProxy() *vnc_proxy.Proxy {
	return vnc_proxy.New(&vnc_proxy.Config{
		TokenHandler: func(r *http.Request) (addr string, err error) {
			defer func() {
				if p := recover(); p != nil {
					debug.PrintStack()
				}
			}()
			return ":5900", nil
		},
	})
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
