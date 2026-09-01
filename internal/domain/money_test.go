package domain

import "testing"

func TestMoneyParsingAndFXConversion(t *testing.T) {
	minor, err := ParseMajorAmount("1234.56", "cny")
	if err != nil || minor != 123456 {
		t.Fatalf("parse CNY: minor=%d err=%v", minor, err)
	}
	jpy, err := ParseMajorAmount("1500", "JPY")
	if err != nil || jpy != 1500 {
		t.Fatalf("parse JPY: minor=%d err=%v", jpy, err)
	}
	rate, err := ParseFXRate("7.12345678")
	if err != nil || rate != 712345678 {
		t.Fatalf("parse FX rate: rate=%d err=%v", rate, err)
	}
	converted, err := ConvertMinor(10000, "USD", "CNY", 712345678)
	if err != nil || converted != 71235 {
		t.Fatalf("convert USD to CNY: minor=%d err=%v", converted, err)
	}
	converted, err = ConvertMinor(1500, "JPY", "CNY", 4800000)
	if err != nil || converted != 7200 {
		t.Fatalf("convert JPY to CNY: minor=%d err=%v", converted, err)
	}
	if _, err := ParseMajorAmount("1.001", "CNY"); err == nil {
		t.Fatal("too many minor-unit decimals should fail")
	}
	if got := FormatMinor(-71235, "CNY"); got != "-712.35 CNY" {
		t.Fatalf("format minor amount: %q", got)
	}
}
