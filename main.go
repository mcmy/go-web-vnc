package main

import (
	"embed"
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

func findWindowsPortPid(port string) string {
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

func killProcessUsingPort(port string) error {
	pid := findWindowsPortPid(port)
	if len(pid) < 1 {
		return fmt.Errorf("not find port exe,:%s", port)
	}
	killCmd := exec.Command("taskkill", "/F", "/PID", pid)
	err := killCmd.Run()
	if err != nil {
		return err
	}

	return nil
}

func main() {
	if err := killProcessUsingPort("5900"); err == nil {
		time.Sleep(time.Second * 3)
	}
	if err := os.RemoveAll(".tight_vnc"); err != nil {
		log.Fatalln(err)
	}
	if err := ExtraEmbedFs(public_fs.EmbedVNC, "tight_vnc", ".cache"); err != nil {
		log.Fatalln(err)
	}
	go func() {
		if err := exec.Command(".cache/tight_vnc/TightVNCServerPortable.exe").Run(); err != nil {
			log.Fatalln(err)
		}
	}()
	go func() {
		for {
			time.Sleep(time.Second * 3)
			if len(findWindowsPortPid("5900")) == 0 {
				log.Fatalln("vnc is shutdown")
			}
		}
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

	quit := make(chan os.Signal)
	signal.Notify(quit, os.Interrupt)
	<-quit

	log.Println("Shutdown Server ...")
	go func() {
		time.Sleep(time.Second * 5)
		os.Exit(1)
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("Server Shutdown:", err)
	}
	if err := killProcessUsingPort("5900"); err != nil {
		log.Fatal("Server Shutdown:", err)
	}
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
