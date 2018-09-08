package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/dikinova/dktunnel/tunnel"
	"time"
	"bytes"
	"crypto/sha256"
	)

const (
	Version = "v1.1.5"
)

var (
	startTime = tunnel.TimeNowMs()
)

func handleExitSignal(app tunnel.APP, f *os.File) { //win10下不太有效.
	// Program that will listen to the SIGINT and SIGTERM
	// SIGINT will listen to CTRL-C.
	// SIGTERM will be caught if kill command executed.
	c := make(chan os.Signal)
	signal.Notify(c, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGHUP)

	for sig := range c {
		fmt.Fprintf(os.Stderr, "caught sig: %+v \n", sig)
		switch sig {
		case syscall.SIGHUP:
			var b bytes.Buffer
			app.Status(&b)
			tunnel.Warn("status: %s", b.String())
			tunnel.Warn("total goroutines:%d", runtime.NumGoroutine())
		default:
			time.Sleep(1 * time.Second)
			tunnel.Warn("APP END %d", uint16(startTime))
			f.Close()
			os.Exit(0)
		}
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, "usage: %s\n", os.Args[0])
	flag.PrintDefaults()
	os.Exit(1)
}

func main() {

	client := flag.Bool("c", false, "run as client")
	server := flag.Bool("s", false, "run as server")
	baddr := flag.String("backend", "1.2.3.4:5555", "backend address.")
	laddr := flag.String("listen", "127.0.0.1:3333", "listen address.")
	secret := flag.String("secret", "", "tunnel secret.")

	flag.StringVar(&tunnel.CipherName, "cipher", "dummy", "available ciphers: "+tunnel.ListCipher())
	flag.BoolVar(&tunnel.ExitOnError, "exiterror", false, "exit on error. just for test.")
	flag.BoolVar(&tunnel.VerifyCRC, "crc", true, "verify data crc.")

	tunnels := flag.Uint("tunnels", 1, "(client-only) low level tunnel count.")
	logLevel := flag.Uint("log", 1, "app log level. warn=1, info=2, etc.")

	flag.Usage = usage
	flag.Parse()

	tunnel.LogLevel = uint8(*logLevel) //必须在执行parse之后才能访问对应变量

	if *client == *server || len(*secret) == 0 {
		flag.Usage()
		return
	}

	_, _, cerr := tunnel.PickCipher(tunnel.CipherName, []byte{1})
	if cerr != nil {
		fmt.Fprintf(os.Stderr, "no cipher:%s\n", tunnel.CipherName)
		flag.Usage()
		return
	}

	if tunnel.TunnelReadTimeout < 20 || tunnel.TunnelReadTimeout > tunnel.MaxReadTimeout {
		tunnel.TunnelReadTimeout = 60
	}

	//输入参数检验完毕. do some preparing.

	filename := "gtwarn" + time.Now().Format("2006-01-02") + ".log"
	warnfile, ferr := os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if ferr != nil {
		tunnel.Error("error opening file: %v", ferr)
		return
	}
	defer warnfile.Close()
	tunnel.InitLogger(warnfile, uint16(startTime))

	tunnel.Warn("APP START %d", uint16(startTime))
	kd := sha256.Sum256([]byte(*secret))
	tunnel.Warn("protocal: %s, cipher: %s, secret-hash-hex: %x \n", Version, tunnel.CipherName, kd[:3])

	// start app now
	var app tunnel.APP
	var err error

	if *server {
		app, err = tunnel.NewServer(*laddr, *baddr, *secret)
	}

	if *client {
		tunnel.Debug("APP client mode")
		if *tunnels < 1 || *tunnels > 3 {
			*tunnels = 1
		}
		app, err = tunnel.NewClient(*laddr, *baddr, *secret, *tunnels)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "create service failed:%s\n", err.Error())
		tunnel.Warn("APP END %d", uint16(startTime))
		return
	}

	go tunnel.Report(app)

	// waiting for signal
	go handleExitSignal(app, warnfile)

	tunnel.Warn("APP END %d %v", uint16(startTime), app.Start())
}
