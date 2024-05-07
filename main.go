package main

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"go-web-vnc/conf"
	"go-web-vnc/public_fs"
	"go-web-vnc/vnc_proxy"
	"golang.org/x/net/context"
	"golang.org/x/net/websocket"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime/debug"
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

func main() {
	if err := os.RemoveAll(".cache"); err != nil {
		log.Fatalln(err)
	}
	if err := ExtraEmbedFs(public_fs.EmbedVNC, "tvnc", ".cache"); err != nil {
		log.Fatalln(err)
	}

	if err := initVncReg(); err != nil {
		log.Fatalln(err)
	}

	go func() {
		_ = exec.Command(".cache/tvnc/tvnserver.exe", "-install", "-silent").Run()
		time.Sleep(time.Second)
		for {
			if findWindowsPortProcessPID(fmt.Sprintf("%d1", settings.RfbPort)) == "" {
				_ = exec.Command(".cache/tvnc/tvnserver.exe", "-start", "-silent").Run()
			}
			time.Sleep(time.Second * 3)
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
	log.Printf("start vnc server on :%d\n", settings.RfbPort)
	log.Printf("start vnc http server on :%d\n", conf.Conf.AppInfo.Port)
	log.Printf("vnc page :%d/vnc\n", conf.Conf.AppInfo.Port)

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
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Println(err)
	}
	_ = exec.Command(".cache/tvnc/tvnserver.exe", "-stop", "-silent").Run()
	if err := os.RemoveAll(".cache"); err != nil {
		log.Println(err)
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
			return fmt.Sprintf("%s:%d", getLocalIPv4Address(), 5900), nil
		},
	})
}
