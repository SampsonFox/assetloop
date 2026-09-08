package main

import (
	"encoding/binary"
	"encoding/xml"
	"strings"
	"testing"
	"time"
	"unicode/utf16"
)

func TestMarketTaskShanghaiAndCatchup(t *testing.T) {
	s := schedulerXML("C:/A&B/assetloop.exe", "C:/A&B/.env", time.Date(2026, 9, 8, 16, 0, 0, 0, time.UTC))
	var task struct {
		Triggers struct {
			CalendarTrigger struct {
				StartBoundary string
				ScheduleByDay struct{ DaysInterval int }
			}
		}
		Settings struct {
			StartWhenAvailable      bool
			MultipleInstancesPolicy string
		}
		Actions struct {
			Exec struct{ Command, Arguments, WorkingDirectory string }
		}
	}
	if e := xml.Unmarshal([]byte(s), &task); e != nil {
		t.Fatal(e)
	}
	if task.Triggers.CalendarTrigger.StartBoundary != "2026-09-09T09:00:00+08:00" || task.Triggers.CalendarTrigger.ScheduleByDay.DaysInterval != 1 || !task.Settings.StartWhenAvailable || task.Settings.MultipleInstancesPolicy != "IgnoreNew" || task.Actions.Exec.Command != "C:/A&B/assetloop.exe" || !strings.Contains(task.Actions.Exec.Arguments, "refresh-market --config") {
		t.Fatalf("%+v", task)
	}
}

func TestWindowsTaskXMLFileEncodingPreservesUnicode(t *testing.T) {
	original := schedulerXML("C:/物迹/assetloop.exe", "C:/物迹/.env", time.Now())
	data := schedulerFileBytes(original)
	if len(data) < 2 || data[0] != 0xff || data[1] != 0xfe || len(data)%2 != 0 {
		t.Fatal("Task Scheduler requires UTF-16LE BOM")
	}
	units := make([]uint16, (len(data)-2)/2)
	for i := range units {
		units[i] = binary.LittleEndian.Uint16(data[2+i*2:])
	}
	decoded := string(utf16.Decode(units))
	if decoded != strings.Replace(original, "encoding=\"UTF-8\"", "encoding=\"UTF-16\"", 1) {
		t.Fatal("task file lost Unicode or encoding declaration")
	}
}
