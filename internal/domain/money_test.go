package domain

import "testing"

func TestCurrencyCatalogDrivesParsingAndSelection(t *testing.T) {
	if CurrencyCatalogVersion != "ISO 4217 List One 2026-01-01" {
		t.Fatalf("unexpected currency catalog version %q", CurrencyCatalogVersion)
	}
	catalog := CurrencyCatalog()
	selectable := SelectableCurrencyCodes()
	if len(catalog) != 156 || len(selectable) != 156 {
		t.Fatalf("currency catalog size: catalog=%d selectable=%d", len(catalog), len(selectable))
	}
	seen := make(map[string]bool, len(catalog))
	for i, item := range catalog {
		if seen[item.Code] || !item.Selectable || selectable[i] != item.Code {
			t.Fatalf("invalid catalog entry at %d: %+v", i, item)
		}
		if i > 0 && catalog[i-1].Code >= item.Code {
			t.Fatalf("currency catalog is not sorted: %s before %s", catalog[i-1].Code, item.Code)
		}
		seen[item.Code] = true
		minor, err := ParseMajorAmount("1", item.Code)
		if err != nil || minor != pow10(item.MinorUnits).Int64() {
			t.Fatalf("catalog amount parsing for %s: minor=%d units=%d err=%v", item.Code, minor, item.MinorUnits, err)
		}
	}
	for code, units := range map[string]int{"BHD": 3, "JPY": 0, "UYW": 4, "ZWG": 2} {
		if got := currencyExponents[code]; got != units {
			t.Fatalf("minor units for %s: got %d want %d", code, got, units)
		}
	}
	if _, err := NormalizeSelectableCurrency("BGN"); err == nil {
		t.Fatal("retired BGN must not be selectable from the current catalog")
	}
	catalog[0].Code = "XXX"
	selectable[0] = "XXX"
	if CurrencyCatalog()[0].Code != "AED" || SelectableCurrencyCodes()[0] != "AED" {
		t.Fatal("currency catalog accessors must return defensive copies")
	}
}

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
