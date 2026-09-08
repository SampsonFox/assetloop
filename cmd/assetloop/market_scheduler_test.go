package main

import (
	"encoding/xml"
	"strings"
	"testing"
	"time"
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
