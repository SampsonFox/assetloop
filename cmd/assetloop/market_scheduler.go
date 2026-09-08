package main

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/xml"
	"flag"
	"fmt"
	"github.com/SampsonFox/assetloop/internal/config"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode/utf16"
)

func xmlText(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}
func schedulerXML(executable, configuration string, now time.Time) string {
	start := now.In(time.FixedZone("Asia/Shanghai", 8*3600)).Format("2006-01-02") + "T09:00:00+08:00"
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task"><Triggers><CalendarTrigger><StartBoundary>%s</StartBoundary><Enabled>true</Enabled><ScheduleByDay><DaysInterval>1</DaysInterval></ScheduleByDay></CalendarTrigger></Triggers><Principals><Principal id="Author"><LogonType>InteractiveToken</LogonType><RunLevel>LeastPrivilege</RunLevel></Principal></Principals><Settings><MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy><DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries><StopIfGoingOnBatteries>false</StopIfGoingOnBatteries><StartWhenAvailable>true</StartWhenAvailable><ExecutionTimeLimit>PT2H</ExecutionTimeLimit></Settings><Actions Context="Author"><Exec><Command>%s</Command><Arguments>refresh-market --config &quot;%s&quot;</Arguments><WorkingDirectory>%s</WorkingDirectory></Exec></Actions></Task>`, start, xmlText(executable), xmlText(configuration), xmlText(filepath.Dir(configuration)))
}

// Task Scheduler's XML file importer expects a Unicode document with a BOM.
func schedulerFileBytes(definition string) []byte {
	definition = strings.Replace(definition, "encoding=\"UTF-8\"", "encoding=\"UTF-16\"", 1)
	units := utf16.Encode([]rune(definition))
	data := make([]byte, 2+len(units)*2)
	data[0] = 0xff
	data[1] = 0xfe
	for i, u := range units {
		binary.LittleEndian.PutUint16(data[2+i*2:], u)
	}
	return data
}
func installMarketScheduler(args []string) error {
	flags := flag.NewFlagSet("install-scheduler", flag.ContinueOnError)
	configPath := flags.String("config", ".env", "configuration path")
	dry := flags.Bool("dry-run", false, "print task definition without installing")
	if e := flags.Parse(args); e != nil {
		return e
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected arguments")
	}
	absolute, e := filepath.Abs(*configPath)
	if e != nil {
		return e
	}
	cfg, e := config.Load(absolute)
	if e != nil {
		return e
	}
	if cfg.Market.Token == "" {
		return fmt.Errorf("market credentials are required")
	}
	executable, e := os.Executable()
	if e != nil {
		return e
	}
	if runtime.GOOS != "windows" {
		return fmt.Errorf("use the documented cron/systemd invocation of refresh-market on this platform")
	}
	definition := schedulerXML(executable, absolute, time.Now())
	if *dry {
		fmt.Fprintln(os.Stdout, definition)
		return nil
	}
	file, e := os.CreateTemp("", "assetloop-market-*.xml")
	if e != nil {
		return e
	}
	defer os.Remove(file.Name())
	if _, e = file.Write(schedulerFileBytes(definition)); e != nil {
		file.Close()
		return e
	}
	if e = file.Close(); e != nil {
		return e
	}
	sum := sha256.Sum256([]byte(absolute))
	name := fmt.Sprintf("AssetLoop-Market-%x", sum[:6])
	cmd := exec.Command("schtasks.exe", "/Create", "/TN", name, "/XML", file.Name())
	output, e := cmd.CombinedOutput()
	if e != nil {
		return fmt.Errorf("create scheduled task: %s", strings.TrimSpace(string(output)))
	}
	fmt.Fprintf(os.Stdout, "Installed %s (09:00 Asia/Shanghai)\n", name)
	return nil
}
